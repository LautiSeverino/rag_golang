package extractor

import (
	"rag_golang/internal/core/domain"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAttachSectionPath_IgnoresStructuralHeadings(t *testing.T) {
	elements := []domain.Element{
		{Type: domain.ElemHeading, Level: 2, Text: "2. ELEMENTOS DE JUEGO"},
		{Type: domain.ElemParagraph, Text: "Contenido de elementos"},
		{Type: domain.ElemHeading, Level: 1, Text: "ÍNDICE"},
		// Lista del índice — no debe convertirse en hija de ÍNDICE
		{Type: domain.ElemListItem, Text: "Elementos de juego . . . 2"},
		{Type: domain.ElemHeading, Level: 2, Text: "3. REPARTO DE PAÍSES"},
		{Type: domain.ElemParagraph, Text: "Contenido de reparto"},
	}

	result := attachSectionPath(elements)

	// "3. REPARTO DE PAÍSES" NO debe estar bajo "ÍNDICE"
	for _, el := range result {
		if el.Text == "Contenido de reparto" {
			assert.NotContains(t, el.SectionPath, "ÍNDICE",
				"las secciones de contenido no deben anidarse bajo el heading ÍNDICE")
			assert.Contains(t, el.SectionPath, "3. REPARTO DE PAÍSES")
		}
	}
}

func TestFilterTOCElements_RemovesTOCEntries(t *testing.T) {
	elements := []domain.Element{
		{Type: domain.ElemListItem, Text: "ELEMENTOS DE JUEGO . . . . . . . . . 2"},
		{Type: domain.ElemListItem, Text: "REPARTO DE PAÍSES . . . . . . . . . 3"},
		{Type: domain.ElemParagraph, Text: "Un planisferio dividido en 50 países."},
	}

	result := filterTOCElements(elements)

	require.Len(t, result, 1, "solo debe quedar el párrafo de contenido real")
	assert.Equal(t, "Un planisferio dividido en 50 países.", result[0].Text)
}
