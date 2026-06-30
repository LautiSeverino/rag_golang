package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"rag_golang/internal/core/domain/llm"
)

// OllamaLLM implementa out.ILLMPort usando la API REST de Ollama con streaming.
type OllamaLLM struct {
	baseURL    string
	httpClient *http.Client
}

func NewLLM(baseURL string) *OllamaLLM {
	return &OllamaLLM{
		baseURL: baseURL,
		httpClient: &http.Client{
			// Sin timeout global: el streaming puede tardar minutos.
			// El contexto del caller es el mecanismo de cancelación.
			Timeout: 0,
		},
	}
}

// ollamaChatReq es el cuerpo del request a /api/chat.
type ollamaChatReq struct {
	Model    llm.LLMModel    `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Options  ollamaOptions   `json:"options"`
}

type ollamaMessage struct {
	Role    llm.Role `json:"role"`
	Content string   `json:"content"`
}

type ollamaOptions struct {
	Temperature llm.LLMTemperature `json:"temperature"`
	NumPredict  llm.LLMNumPredict  `json:"num_predict"`
	NumCtx      llm.LLMNumCtx      `json:"num_ctx"`
}

// ollamaChatResp es cada línea NDJSON que devuelve Ollama en modo stream.
type ollamaChatResp struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	Done bool `json:"done"`
}

// Generate inicia la generación en streaming y devuelve un canal de tokens.
// El canal se cierra cuando Ollama termina o cuando el contexto se cancela.
// El caller es responsable de drenar el canal hasta que se cierre.
func (l *OllamaLLM) Generate(ctx context.Context, req llm.GenerateRequest) (<-chan llm.GenerateToken, error) {
	msgs := make([]ollamaMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = ollamaMessage{
			Role:    m.Role,
			Content: m.Content,
		}
	}

	body, err := json.Marshal(ollamaChatReq{
		Model:    req.Model,
		Messages: msgs,
		Stream:   true,
		Options: ollamaOptions{
			Temperature: req.Options.Temperature,
			NumPredict:  req.Options.NumPredict,
			NumCtx:      req.Options.NumCtx,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("ollama llm: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		l.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama llm: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Iniciamos la conexión antes de abrir el canal para poder
	// devolver errores de red sincrónicamente.
	resp, err := l.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama llm: do request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("ollama llm: status %d", resp.StatusCode)
	}

	// Creá un canal que pueda almacenar hasta 32 tokens antes de que el productor tenga que esperar.
	ch := make(chan llm.GenerateToken, 32)

	go func() {
		defer close(ch)
		defer resp.Body.Close()

		decoder := json.NewDecoder(resp.Body)
		for {
			var line ollamaChatResp
			if err := decoder.Decode(&line); err != nil {
				// EOF o error de red: terminamos silenciosamente.
				// El contexto cancelado es el caso normal de interrupción.
				return
			}

			select {
			case <-ctx.Done():
				return
			case ch <- llm.GenerateToken{Text: line.Message.Content, Done: line.Done}:
			}

			if line.Done {
				return
			}
		}
	}()

	return ch, nil
}

// WaitTimeout es el tiempo máximo para esperar la primera respuesta de Ollama
// cuando no está en modo streaming. No se usa en Generate pero sirve para
// health checks.
const WaitTimeout = 30 * time.Second
