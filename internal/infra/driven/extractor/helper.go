package extractor

import (
	"path/filepath"
	"rag_golang/internal/core/domain"
	"strings"
)

func nonEmpty(ss []string) []string {
	// CRÍTICO: usar make() en lugar de ss[:0]. Con ss[:0] el slice resultante
	// comparte el mismo array subyacente que `stack` en attachSectionPath().
	// Como `stack` se sobrescribe en cada heading nuevo, todos los SectionPath
	// asignados previamente quedarían apuntando al mismo array y terminarían
	// reflejando el ÚLTIMO estado de `stack`, no el estado en el momento de
	// la asignación. Con make(), cada llamada produce un array independiente.
	result := make([]string, 0, len(ss))
	for _, s := range ss {
		if s != "" {
			result = append(result, s)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
func inferDocType(path string) domain.DocType {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".pdf":
		return domain.DocPDF
	case ".docx":
		return domain.DocDOCX
	default:
		return domain.DocPDF
	}
}
