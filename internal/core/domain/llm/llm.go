package llm

import "rag_golang/internal/core/domain/search"

// LLMOptions controla los parámetros de generación del LLM.
type LLMOptions struct {
	Temperature float32 `json:"temperature" yaml:"temperature"`  // 0.0 = determinístico, 1.0 = creativo
	NumPredict  int     `json:"num_predict"  yaml:"num_predict"` // max tokens a generar
	NumCtx      int     `json:"num_ctx"      yaml:"num_ctx"`     // context window size
}

// GenerateRequest es la solicitud de generación al LLM.
// Context contiene los chunks recuperados que se inyectan en el prompt.
type GenerateRequest struct {
	Model    string                `json:"model"`
	Messages []Message             `json:"messages"`
	Context  []search.SearchResult `json:"context,omitempty"` // chunks inyectados como contexto RAG
	Options  LLMOptions            `json:"options"`
	Stream   bool                  `json:"stream"`
}

// GenerateToken es un token de la respuesta del LLM en modo streaming.
// Done == true indica que la generación terminó.
type GenerateToken struct {
	Text string `json:"text"`
	Done bool   `json:"done"`
}
