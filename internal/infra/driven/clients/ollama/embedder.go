package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"rag_golang/internal/core/domain/embed"
)

// OllamaEmbedder implementa out.IEmbedderPort usando la API REST de Ollama.
type OllamaEmbedder struct {
	baseURL    string
	model      embed.EmbedModel
	dim        int
	httpClient *http.Client
}

func NewEmbedder(baseURL string, model embed.EmbedModel, dim int) *OllamaEmbedder {
	return &OllamaEmbedder{
		baseURL: baseURL,
		model:   model,
		dim:     dim,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

type embedReq struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embedResp struct {
	Embeddings [][]float32 `json:"embeddings"`
}

// Embed genera embeddings para un batch de textos.
// Ollama procesa todos los textos en una sola llamada HTTP.
func (e *OllamaEmbedder) Embed(ctx context.Context, texts []string) ([]embed.Vector, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	body, err := json.Marshal(embedReq{
		Model: string(e.model),
		Input: texts,
	})
	if err != nil {
		return nil, fmt.Errorf("ollama embedder: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		e.baseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama embedder: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama embedder: do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama embedder: status %d", resp.StatusCode)
	}

	var result embedResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("ollama embedder: decode response: %w", err)
	}

	if len(result.Embeddings) != len(texts) {
		return nil, fmt.Errorf("ollama embedder: got %d embeddings for %d texts",
			len(result.Embeddings), len(texts))
	}

	vectors := make([]embed.Vector, len(result.Embeddings))
	for i, emb := range result.Embeddings {
		vectors[i] = embed.Vector(emb)
	}
	return vectors, nil
}

func (e *OllamaEmbedder) Dimension() int              { return e.dim }
func (e *OllamaEmbedder) ModelName() embed.EmbedModel { return e.model }
