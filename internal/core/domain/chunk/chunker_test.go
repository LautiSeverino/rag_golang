package chunk

import (
	"rag_golang/internal/core/domain"
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

// func TestChunkSectionNoEmptyChunks(t *testing.T) {
// 	// Verifica que el bug de chunks vacíos no regrese.
// 	// ...
// 	for _, c := range chunks {
// 		assert.NotEmpty(t, c.RawText, "ningún chunk debe tener RawText vacío")
// 	}
// }
