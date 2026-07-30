package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"rag_golang/internal/core/domain/embed"
)

// OllamaEmbedder implementa out.IEmbedderPort usando la API REST de Ollama.
type OllamaEmbedder struct {
	baseURL    string
	model      string
	dim        int
	httpClient *http.Client
}

func NewEmbedder(baseURL string, model string, dim int) *OllamaEmbedder {
	return &OllamaEmbedder{
		baseURL: baseURL,
		model:   model,
		dim:     dim,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

type embedResp struct {
	Embeddings [][]float32 `json:"embeddings"`
}

type embedReq struct {
	Model    string   `json:"model"`
	Input    []string `json:"input"`
	Truncate *bool    `json:"truncate,omitempty"`
}

type errorResp struct {
	Error string `json:"error"`
}

func (e *OllamaEmbedder) Embed(ctx context.Context, texts []string) ([]embed.Vector, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	truncate := true
	body, err := json.Marshal(embedReq{
		Model:    string(e.model),
		Input:    texts,
		Truncate: &truncate,
	})
	if err != nil {
		return nil, fmt.Errorf("ollama embedder: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/api/embed", bytes.NewReader(body))
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
		var er errorResp
		raw, _ := io.ReadAll(resp.Body)
		if json.Unmarshal(raw, &er) == nil && er.Error != "" {
			return nil, fmt.Errorf("ollama embedder: status %d: %s", resp.StatusCode, er.Error)
		}
		return nil, fmt.Errorf("ollama embedder: status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var result embedResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("ollama embedder: decode response: %w", err)
	}

	if len(result.Embeddings) != len(texts) {
		return nil, fmt.Errorf("ollama embedder: got %d embeddings for %d texts", len(result.Embeddings), len(texts))
	}

	vectors := make([]embed.Vector, len(result.Embeddings))
	for i, emb := range result.Embeddings {
		vectors[i] = embed.Vector(emb)
	}
	return vectors, nil
}

func (e *OllamaEmbedder) Dimension() int    { return e.dim }
func (e *OllamaEmbedder) ModelName() string { return string(e.model) }
