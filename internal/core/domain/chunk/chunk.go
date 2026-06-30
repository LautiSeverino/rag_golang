package chunk

import (
	"rag_golang/internal/core/domain"

	"github.com/google/uuid"
)

// ChunkStrategy determina cómo se divide un Document en Chunks.
type ChunkStrategy string

const (
	// ChunkSection agrupa todo el contenido bajo un heading como un chunk.
	// Secciones largas se subdividen por Element. Estrategia recomendada.
	ChunkSection ChunkStrategy = "section"

	// ChunkElement produce exactamente un chunk por Element.
	// Bueno para documentos muy estructurados donde cada párrafo es autónomo.
	ChunkElement ChunkStrategy = "element"

	// ChunkSliding aplica ventana deslizante sobre el texto plano.
	// Fallback para documentos sin estructura detectada.
	ChunkSliding ChunkStrategy = "sliding"
)

type ChunkMaxSize int

// Límites de tamaño semánticos para el Chunker (en caracteres)
const (
	ChunkMaxSizeShort  ChunkMaxSize = 500  // Ideal para chunks atómicos (frases o elementos cortos)
	ChunkMaxSizeMedium ChunkMaxSize = 1000 // Tamaño estándar balanceado (párrafos típicos)
	ChunkMaxSizeLong   ChunkMaxSize = 2000 // Para secciones extensas o documentos densos
	ChunkMaxSizeXLong  ChunkMaxSize = 4000 // Límite máximo antes de saturar contextos pequeños

	DefaultOverlap int = 200 // Overlap estándar para estrategias deslizantes
)

// Chunk es la unidad de texto que se embebe e indexa.
//
// Text contiene el SectionPath como prefijo si ContextPrefix estaba activo.
// RawText contiene solo el contenido del Element, sin prefijo.
// El embedding se calcula sobre Text; RawText se muestra al usuario en citas.
// Hash es sha256 de Text y se usa como clave en el EmbedCache (bbolt).
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
	Strategy      ChunkStrategy `json:"strategy"       yaml:"strategy"`
	MaxSize       ChunkMaxSize  `json:"max_size"       yaml:"max_size"`       // en caracteres
	Overlap       int           `json:"overlap"        yaml:"overlap"`        // solo para ChunkSliding
	ContextPrefix bool          `json:"context_prefix" yaml:"context_prefix"` // prepend SectionPath al texto
}

// DefaultConfig devuelve una configuración segura y probada para producción.
func DefaultConfig() ChunkConfig {
	return ChunkConfig{
		Strategy:      ChunkSection,
		MaxSize:       ChunkMaxSizeMedium,
		Overlap:       DefaultOverlap,
		ContextPrefix: true,
	}
}
