package index

import (
	"time"

	"github.com/google/uuid"
)

// IndexResult es el resultado de indexar un documento.
type IndexResult struct {
	DocID      uuid.UUID     `json:"doc_id"`
	Source     string        `json:"source"`
	ChunkCount int           `json:"chunk_count"`
	CacheHits  int           `json:"cache_hits"` // cuántos chunks ya estaban en bbolt
	Duration   time.Duration `json:"duration_ns"`
}
