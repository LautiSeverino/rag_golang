package extractor

import (
	"rag_golang/internal/core/domain"
	"strings"
)

// attachSectionPath — usada por md.go y html.go
func attachSectionPath(elements []domain.Element) []domain.Element {
	stack := make([]string, 6)
	depth := 0
	for i, el := range elements {
		if el.Type != domain.ElemHeading {
			if depth > 0 {
				elements[i].SectionPath = nonEmpty(stack[:depth])
			}
			continue
		}
		if isPageNumberHeading(el) {
			elements[i].SectionPath = nonEmpty(stack[:depth])
			continue
		}
		if isStructuralHeading(el) {
			elements[i].SectionPath = nonEmpty(stack[:depth])
			continue
		}
		lvl := min(max(el.Level, 1), 6)
		stack[lvl-1] = el.Text
		for j := lvl; j < 6; j++ {
			stack[j] = ""
		}
		depth = lvl
		elements[i].SectionPath = nonEmpty(stack[:depth])
	}
	return elements
}

func isPageNumberHeading(el domain.Element) bool {
	if el.Type != domain.ElemHeading {
		return false
	}
	text := strings.TrimSpace(el.Text)
	for _, r := range text {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(text) > 0 && len(text) <= 3
}

var structuralHeadings = map[string]bool{
	"ÍNDICE": true, "CONTENTS": true, "TABLE OF CONTENTS": true,
	"ÍNDICE GENERAL": true, "INDICE": true, "CONTENIDO": true,
}

func isStructuralHeading(el domain.Element) bool {
	if el.Type != domain.ElemHeading {
		return false
	}
	return structuralHeadings[strings.ToUpper(strings.TrimSpace(el.Text))]
}
