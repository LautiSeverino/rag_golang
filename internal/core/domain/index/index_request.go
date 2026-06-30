package index

import (
	"rag_golang/internal/core/domain/chunk"
	"rag_golang/internal/core/domain/embed"
)

// IndexRequest es un Chunk con su Vector listo para ser insertado en Qdrant.
// El payload de Qdrant se construye desde Chunk.
type IndexRequest struct {
	Chunk          chunk.Chunk  `json:"chunk"`
	Vector         embed.Vector `json:"vector"`
	CollectionName string       `json:"collection_name"`
}
