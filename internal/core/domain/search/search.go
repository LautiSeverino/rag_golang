package search

import (
	"rag_golang/internal/core/domain"
	"rag_golang/internal/core/domain/chunk"
	"rag_golang/internal/core/domain/embed"
)

// SearchFilter permite filtrar resultados de búsqueda por metadatos.
// Todos los campos son punteros para distinguir "no filtrar" de "filtrar por zero value".
type SearchFilter struct {
	ElementType *domain.ElementType `json:"element_type,omitempty"` // filtrar por tipo de elemento
	Source      *string             `json:"source,omitempty"`       // filtrar por archivo de origen
	MinPage     *int                `json:"min_page,omitempty"`     // página mínima inclusive
	MaxPage     *int                `json:"max_page,omitempty"`     // página máxima inclusive
}

// SearchRequest parametriza una búsqueda densa en Qdrant.
type SearchRequest struct {
	Vector         embed.Vector  `json:"vector"`
	TopK           int           `json:"top_k"`
	ScoreThreshold *float32      `json:"score_threshold,omitempty"` // descartar resultados debajo de este score
	Filter         *SearchFilter `json:"filter,omitempty"`
}

// SearchResult es un resultado de búsqueda con su score de similitud.
type SearchResult struct {
	Chunk chunk.Chunk `json:"chunk"`
	Score float32     `json:"score"` // [0, 1], mayor = más similar
}
