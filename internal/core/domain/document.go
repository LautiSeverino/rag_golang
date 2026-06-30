package domain

import (
	"time"

	"github.com/google/uuid"
)

// DocType identifica el formato del archivo de origen.
type DocType string

const (
	DocPDF      DocType = "pdf"
	DocDOCX     DocType = "docx"
	DocHTML     DocType = "html"
	DocMarkdown DocType = "md"
)

// Document es la representación intermedia unificada del pipeline.
// Todos los extractores (PDF, DOCX, HTML, MD) producen un *Document.
// El Chunker solo conoce *Document, no el formato de origen.
//
// Se serializa a JSON y se persiste en data/processed/<name>.json
// para evitar re-extraer en futuras re-indexaciones.
type Document struct {
	ID       uuid.UUID        `json:"id"` // uuid v4
	Metadata DocumentMetadata `json:"metadata"`
	Elements []Element        `json:"elements"`
}

// DocumentMetadata contiene información sobre el archivo de origen.
// Se persiste junto al Document en el JSON de caché.
type DocumentMetadata struct {
	Source    string    `json:"source"` // path absoluto o relativo del archivo original
	DocType   DocType   `json:"doc_type"`
	Title     string    `json:"title,omitempty"` // título extraído o inferido
	PageCount int       `json:"page_count"`
	Checksum  string    `json:"checksum"` // sha256 del archivo, para invalidación de caché
	IndexedAt time.Time `json:"indexed_at,omitempty"`
}
