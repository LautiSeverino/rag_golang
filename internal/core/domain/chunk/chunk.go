package chunk

import (
	"rag_golang/internal/core/domain"

	"github.com/google/uuid"
)

// Chunk es la unidad de texto que se embebe e indexa.
type Chunk struct {
	ID          uuid.UUID          `json:"id"`
	DocID       uuid.UUID          `json:"doc_id"`
	Text        string             `json:"text"`     // texto embebible (puede incluir prefix)
	RawText     string             `json:"raw_text"` // texto sin prefix para display
	ElementType domain.ElementType `json:"element_type"`
	SectionPath []string           `json:"section_path,omitempty"`
	Page        int                `json:"page"`
	ChunkIndex  int                `json:"chunk_index"` // posición dentro del documento
	Source      string             `json:"source"`      // path del archivo original
	Hash        string             `json:"hash"`        // sha256(Text), clave de caché
}

// ChunkConfig parametriza el comportamiento del Chunker.
type ChunkConfig struct {
	MaxSize       int  `json:"max_size"       yaml:"max_size"`
	Overlap       int  `json:"overlap"        yaml:"overlap"`        // solo para ChunkSliding
	ContextPrefix bool `json:"context_prefix" yaml:"context_prefix"` // prepend SectionPath al texto
}
