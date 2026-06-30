package search

import "rag_golang/internal/core/domain/chunk"

// BM25SearchRequest parametriza una búsqueda esparsa (BM25).
type BM25SearchRequest struct {
	Query          string        `json:"query"`
	TopK           int           `json:"top_k"`
	Filter         *SearchFilter `json:"filter,omitempty"`
	CollectionName string        `json:"collection_name"`
}

// BM25Result es un resultado de búsqueda BM25 con su score TF-IDF.
type BM25Result struct {
	Chunk chunk.Chunk `json:"chunk"`
	Score float64     `json:"score"`
}
