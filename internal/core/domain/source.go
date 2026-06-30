package domain

// Source es una fuente citada en una respuesta, lista para mostrar al usuario.
type Source struct {
	File        string      `json:"file"`
	Page        int         `json:"page"`
	SectionPath []string    `json:"section_path,omitempty"`
	ElementType ElementType `json:"element_type"`
	Score       float32     `json:"score"`
	Excerpt     string      `json:"excerpt,omitempty"` // fragmento breve para UI
}
