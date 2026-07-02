package chunk

import (
	"rag_golang/internal/core/domain"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChunkSectionCombinesElements(t *testing.T) {
	doc := &domain.Document{
		ID:       uuid.New(),
		Metadata: domain.DocumentMetadata{Source: "test.pdf"},
		Elements: []domain.Element{
			{Type: domain.ElemHeading, Level: 2, Text: "2. ELEMENTOS DE JUEGO",
				Page: 2, SectionPath: []string{"2. ELEMENTOS DE JUEGO"}},
			{Type: domain.ElemListItem, Text: "MAPA",
				Page: 2, SectionPath: []string{"2. ELEMENTOS DE JUEGO"}},
			{Type: domain.ElemParagraph, Text: "Un planisferio dividido en 50 países.",
				Page: 2, SectionPath: []string{"2. ELEMENTOS DE JUEGO"}},
			{Type: domain.ElemListItem, Text: "FICHAS",
				Page: 2, SectionPath: []string{"2. ELEMENTOS DE JUEGO"}},
			{Type: domain.ElemParagraph, Text: "Cada ficha representa 1 ejército.",
				Page: 2, SectionPath: []string{"2. ELEMENTOS DE JUEGO"}},
		},
	}

	cfg := ChunkConfig{
		Strategy:      ChunkSection,
		MaxSize:       1000,
		ContextPrefix: true,
	}

	chunks, err := NewChunker().Chunk(doc, cfg)
	require.NoError(t, err)

	// Todos los elementos de la sección deben producir UN SOLO chunk.
	require.Len(t, chunks, 1, "ChunkSection debe producir 1 chunk para una sección que entra en MaxSize")

	c := chunks[0]
	assert.Contains(t, c.RawText, "MAPA")
	assert.Contains(t, c.RawText, "FICHAS")
	assert.Contains(t, c.RawText, "50 países")
	assert.Contains(t, c.Text, "2. ELEMENTOS DE JUEGO") // prefix
	assert.NotEmpty(t, c.RawText)
}

func TestChunkSectionLongSectionSubchunks(t *testing.T) {
	// Sección que excede MaxSize=100 (usamos MaxSize pequeño para el test)
	elements := make([]domain.Element, 20)
	for i := range elements {
		elements[i] = domain.Element{
			Type:        domain.ElemParagraph,
			Text:        "Párrafo de contenido suficientemente largo para el test número " + strconv.Itoa(i),
			Page:        1,
			SectionPath: []string{"Sección larga"},
		}
	}

	doc := &domain.Document{
		ID:       uuid.New(),
		Metadata: domain.DocumentMetadata{Source: "test.pdf"},
		Elements: elements,
	}

	cfg := ChunkConfig{
		Strategy:      ChunkSection,
		MaxSize:       100, // pequeño para forzar el else
		ContextPrefix: true,
	}

	chunks, err := NewChunker().Chunk(doc, cfg)
	require.NoError(t, err)

	// Con 20 elementos de ~70 chars y MaxSize=100, debe haber varios sub-chunks
	// pero MENOS que 20 (ningún sub-chunk debe ser de 1 solo elemento si hay espacio)
	assert.Greater(t, len(chunks), 1, "debe haber más de 1 sub-chunk")
	assert.Less(t, len(chunks), 20, "no debe haber un chunk por elemento")

	for _, c := range chunks {
		assert.NotEmpty(t, c.RawText, "ningún sub-chunk debe estar vacío")
		assert.Contains(t, c.Text, "Sección larga", "todos deben llevar el prefix")
		assert.LessOrEqual(t, runeLen(c.Text), 200, // MaxSize + prefix margin
			"ningún sub-chunk debe exceder groseramente MaxSize")
	}
}

// func TestChunkSectionNoEmptyChunks(t *testing.T) {
// 	// Verifica que el bug de chunks vacíos no regrese.
// 	// ...
// 	for _, c := range chunks {
// 		assert.NotEmpty(t, c.RawText, "ningún chunk debe tener RawText vacío")
// 	}
// }
