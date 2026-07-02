package out

import "rag_golang/internal/core/domain/embed"

// IEmbedCacheRepository es el puerto de salida para el caché de embeddings.
// Implementación concreta: bbolt en infra/driven/repositories/bbolt.
// La clave es el sha256 del texto (Chunk.Hash).
// Es un Repository porque persiste vectores calculados para evitar re-computarlos.
type IEmbedCacheRepository interface {
	Get(hash string, model embed.EmbedModel) (embed.Vector, bool)
	Set(hash string, vec embed.Vector, model embed.EmbedModel) error
	Close() error
}
