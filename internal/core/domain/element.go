package domain

// ElementType clasifica el tipo de contenido de cada bloque del documento.
// String-based para que el JSON entre Go y Python sea legible y compatible.
type ElementType string

const (
	ElemHeading   ElementType = "heading"   // bloque de título (level 1-6)
	ElemParagraph ElementType = "paragraph" // párrafo de cuerpo
	ElemTable     ElementType = "table"     // tabla con estructura de celdas
	ElemListItem  ElementType = "list_item" // ítem de lista (bullet o numerada)
	ElemCaption   ElementType = "caption"   // caption de figura o tabla
	ElemCode      ElementType = "code"      // bloque de código
)

// Element es un bloque atómico de contenido dentro de un documento.
// Esta es la unidad que el Chunker agrupa y corta para producir Chunks.
//
// Invariante: si Type == ElemTable, el campo Cells estará poblado.
// Invariante: si Type == ElemHeading, Level estará entre 1 y 6.
type Element struct {
	Type        ElementType `json:"type"`
	Level       int         `json:"level,omitempty"`        // solo headings: 1-6
	Text        string      `json:"text"`                   // texto plano (tablas: Markdown serializado)
	Cells       [][]string  `json:"cells,omitempty"`        // solo tablas: [fila][columna]
	Page        int         `json:"page"`                   // número de página 1-based
	SectionPath []string    `json:"section_path,omitempty"` // ["Cap 3", "3.1 Instalación"]
}
