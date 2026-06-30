package out

import (
	"context"
	"rag_golang/internal/core/domain/llm"
)

// ILLMPort es el puerto de salida para generación de texto con el LLM.
// Implementación concreta: cliente Ollama HTTP con streaming en infra/driven/clients/ollama.
//
// Es un puerto de CÓMPUTO: genera texto dado un prompt, sin estado persistente propio.
type ILLMPort interface {
	Generate(ctx context.Context, req llm.GenerateRequest) (<-chan llm.GenerateToken, error)
}
