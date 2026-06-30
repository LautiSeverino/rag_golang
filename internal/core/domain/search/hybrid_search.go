package search

import "rag_golang/internal/core/domain/embed"

// HybridSearchRequest combina búsqueda densa y esparsa para RRF fusion.
// RRFK es la constante k de Reciprocal Rank Fusion (recomendado: 60).
type HybridSearchRequest struct {
	QueryText      string        `json:"query_text"`
	QueryVector    embed.Vector  `json:"query_vector"`
	TopK           int           `json:"top_k"`
	RRFK           int           `json:"rrf_k"` // default 60, estabiliza scores cuando los rankings divergen
	Filter         *SearchFilter `json:"filter,omitempty"`
	CollectionName string        `json:"collection_name"`
}
