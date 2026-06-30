package out

import (
	"context"
	"rag_golang/internal/core/domain/index"
	"rag_golang/internal/core/domain/search"
)

// IVectorRepository es el puerto de salida para almacenamiento de vectores.
// Implementación concreta: Qdrant vía gRPC en infra/driven/repositories/qdrant.
// Es un Repository porque persiste y recupera datos del dominio (chunks + vectores).
type IVectorRepository interface {
	EnsureCollection(ctx context.Context, dimension int) error
	Upsert(ctx context.Context, reqs []index.IndexRequest) error
	Search(ctx context.Context, req search.SearchRequest) ([]search.SearchResult, error)
	Delete(ctx context.Context, docID string) error
}
