package out

import (
	"context"
	"rag_golang/internal/core/domain/chunk"
	"rag_golang/internal/core/domain/search"

	"github.com/google/uuid"
)

// IBM25Repository es el puerto de salida para el índice de búsqueda esparsa.
// Implementación concreta: índice BM25 personalizado en infra/driven/repositories/bm25.
// Es un Repository porque el índice invertido se persiste entre sesiones.
type IBM25Repository interface {
	Index(ctx context.Context, chunks []chunk.Chunk) error
	Search(ctx context.Context, req search.BM25SearchRequest) ([]search.BM25Result, error)
	DeleteByDocID(ctx context.Context, docID uuid.UUID) error
	LoadFromDisk(path string) error
	SaveToDisk(path string) error
}
