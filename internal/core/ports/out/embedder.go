package out

import (
	"context"
	"rag_golang/internal/core/domain/embed"
)

// IEmbedderPort es el puerto de salida para generación de embeddings.
// Implementación concreta: cliente Ollama HTTP en infra/driven/clients/ollama.
//
// Es un puerto de CÓMPUTO, no un repository: genera vectores pero no los persiste.
// La persistencia del vector es responsabilidad de IEmbedCacheRepository y IVectorRepository.
type IEmbedderPort interface {
	Embed(ctx context.Context, texts []string) ([]embed.Vector, error)
	Dimension() int
	ModelName() embed.EmbedModel
}
