package extractor

import (
	"math"
	"rag_golang/internal/core/domain"
	"regexp"
	"strings"
)

// classifyBlocks infiere el ElementType de cada bloque usando el font-size
// relativo a la moda (tamaño del cuerpo de texto).
func classifyBlocks(blocks []rawBlock) []domain.Element {
	if len(blocks) == 0 {
		return nil
	}

	bodySize := bodyFontSize(blocks)
	elements := make([]domain.Element, 0, len(blocks))

	var listBuffer []string
	var listPage int
	var listSection []string

	flushList := func() {
		if len(listBuffer) == 0 {
			return
		}
		elements = append(elements, domain.Element{
			Type: domain.ElemListItem,
			Text: strings.Join(listBuffer, "\n"),
			Page: listPage,
		})
		listBuffer = nil
		listSection = nil
	}

	for _, b := range blocks {
		ratio := b.FontSize / bodySize
		text := strings.TrimSpace(b.Text)
		if text == "" {
			continue
		}

		// IMPORTANTE: clasificar por font-ratio ANTES de chequear list-prefix.
		// Títulos de sección numerados como "1. IDEA GENERAL" o
		// "3. REPARTO DE PAÍSES" tienen el mismo prefijo "N. " que un ítem
		// de lista numerada. Si isListPrefix() se evalúa primero, esos
		// headings se clasifican incorrectamente como list_item y nunca
		// actualizan el SectionPath del documento. El font-size es la señal
		// confiable; el prefijo numérico solo importa para texto de cuerpo.
		elType, level := classifyByRatio(ratio, b.IsBold, text)
		if elType == domain.ElemHeading {
			flushList()
			elements = append(elements, domain.Element{
				Type:  elType,
				Level: level,
				Text:  text,
				Page:  b.Page,
			})
			continue
		}

		// Detectar ítems de lista por prefijo (solo para no-headings)
		if isListPrefix(text) {
			if len(listBuffer) == 0 {
				listPage = b.Page
				listSection = nil
			}
			listBuffer = append(listBuffer, stripListPrefix(text))
			continue
		}
		flushList()

		elements = append(elements, domain.Element{
			Type:  elType,
			Level: level,
			Text:  text,
			Page:  b.Page,
		})
		_ = listSection
	}
	flushList()

	return elements
}

func classifyByRatio(ratio float64, bold bool, text string) (domain.ElementType, int) {
	short := len([]rune(text)) < 150
	endsWithPeriod := strings.HasSuffix(strings.TrimSpace(text), ".")

	switch {
	case ratio >= 1.5:
		return domain.ElemHeading, 1
	case ratio >= 1.25:
		return domain.ElemHeading, 2
	case ratio >= 1.1 && bold && short:
		return domain.ElemHeading, 3
	case bold && short && !endsWithPeriod:
		// Bold + corto + no termina en punto → heading implícito (mismo size que body)
		return domain.ElemHeading, 3
	default:
		return domain.ElemParagraph, 0
	}
}

// attachSectionPath recorre los elementos en orden y asigna el SectionPath
// acumulado hasta ese punto. Los headings actualizan la pila de sección.
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

		// headings estructurales (ÍNDICE, CONTENTS) no modifican el stack.
		// Se les asigna el SectionPath actual pero no se convierten en ancestros.
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

// isPageNumberHeading detecta headings que son solo dígitos (números de página).
// go-fitz los clasifica como headings por su posición y tamaño relativo.
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
	return len(text) > 0 && len(text) <= 3 // "2", "10", "120" → página
}

// filterPageHeaders descarta bloques que aparecen repetidos en posición fija
// (números de página, headers y footers de página).
func filterPageHeaders(elements []domain.Element) []domain.Element {
	// Contar cuántas veces aparece cada texto en el documento
	textCount := make(map[string]int, len(elements))
	for _, el := range elements {
		textCount[el.Text]++
	}

	// Descartar textos que aparecen en más de 3 páginas distintas
	// y son cortos (típico de headers/footers)
	result := elements[:0]
	for _, el := range elements {
		count := textCount[el.Text]
		short := len([]rune(el.Text)) < 60
		if count >= 3 && short {
			continue // probable header/footer de página
		}
		result = append(result, el)
	}
	return result
}

// ─── helpers ──────────────────────────────────────────────────────────────────

var (
	bulletRe = regexp.MustCompile(`^[\s]*[•·◦▪▸▶\-\*]\s+`)
	numRe    = regexp.MustCompile(`^[\s]*\d+[\.\)]\s+`)
	alphaRe  = regexp.MustCompile(`^[\s]*[a-zA-Z][\.\)]\s+`)
)

func isListPrefix(text string) bool {
	return bulletRe.MatchString(text) || numRe.MatchString(text) || alphaRe.MatchString(text)
}

func stripListPrefix(text string) string {
	text = bulletRe.ReplaceAllString(text, "")
	text = numRe.ReplaceAllString(text, "")
	text = alphaRe.ReplaceAllString(text, "")
	return strings.TrimSpace(text)
}

// bodyFontSize devuelve la moda de los font-sizes: el tamaño más frecuente
// es el del cuerpo de texto. Más robusto que el promedio ante documentos
// con muchos títulos grandes.
func bodyFontSize(blocks []rawBlock) float64 {
	counts := make(map[float64]int, 20)
	for _, b := range blocks {
		if b.FontSize <= 0 {
			continue
		}
		// Bucket a 0.5pt para tolerar variaciones mínimas de renderizado
		rounded := math.Round(b.FontSize*2) / 2
		counts[rounded]++
	}

	var bodySize float64
	maxCount := 0
	for size, count := range counts {
		if count > maxCount {
			maxCount = count
			bodySize = size
		}
	}

	if bodySize <= 0 {
		return 11 // fallback razonable
	}
	return bodySize
}

// mergeSplitParagraphs fusiona líneas de párrafo que go-fitz parte
// por salto de línea dentro de la misma página y sección visual.
// Heurística: si el fragmento anterior es ElemParagraph, está en la
// misma página, y NO termina en puntuación de cierre de oración,
// es una continuación de la misma oración.
func mergeSplitParagraphs(elements []domain.Element) []domain.Element {
	if len(elements) == 0 {
		return elements
	}

	endsClause := func(s string) bool {
		s = strings.TrimSpace(s)
		if s == "" {
			return false
		}
		last := rune(s[len(s)-1])
		// Punto, punto y coma, interrogación, exclamación, dos puntos
		return strings.ContainsRune(".;?!:", last)
	}

	merged := make([]domain.Element, 0, len(elements))
	for _, el := range elements {
		if el.Type != domain.ElemParagraph ||
			len(merged) == 0 ||
			merged[len(merged)-1].Type != domain.ElemParagraph ||
			merged[len(merged)-1].Page != el.Page {
			merged = append(merged, el)
			continue
		}

		prev := &merged[len(merged)-1]
		if !endsClause(prev.Text) {
			// Fusionar: el fragmento anterior no cierra oración.
			// Usar espacio como separador (era salto de línea del PDF).
			prev.Text = strings.TrimSpace(prev.Text) + " " + strings.TrimSpace(el.Text)
		} else {
			merged = append(merged, el)
		}
	}
	return merged
}

// tocLineRe detecta líneas típicas de tabla de contenidos: texto seguido de puntos y número de página
var tocLineRe = regexp.MustCompile(`\.{3,}|\.\s{0,2}\.\s{0,2}\.`)

// structuralHeadings son headings que no representan contenido sino
// estructura del documento. No deben convertirse en nodos raíz del SectionPath.
var structuralHeadings = map[string]bool{
	"ÍNDICE":            true,
	"CONTENTS":          true,
	"TABLE OF CONTENTS": true,
	"ÍNDICE GENERAL":    true,
	"INDICE":            true,
	"CONTENIDO":         true,
}

// filterTOCElements elimina elementos que son entradas de tabla de contenidos.
// Una entrada TOC típica contiene una serie de puntos suspensivos seguidos de un número.
func filterTOCElements(elements []domain.Element) []domain.Element {
	result := make([]domain.Element, 0, len(elements))
	for _, el := range elements {
		// Headings estructurales (ÍNDICE) se preservan como headings
		// pero se marcarán para que attachSectionPath no los use como raíz.
		// Los list_item o paragraph que contengan el patrón TOC se descartan.
		if el.Type == domain.ElemListItem || el.Type == domain.ElemParagraph {
			if tocLineRe.MatchString(el.Text) {
				continue // es una entrada de TOC, descartar
			}
		}
		result = append(result, el)
	}
	return result
}

// isStructuralHeading retorna true si el heading es estructural (TOC, índice)
// y no debe usarse como nodo raíz en el SectionPath.
func isStructuralHeading(el domain.Element) bool {
	if el.Type != domain.ElemHeading {
		return false
	}
	normalized := strings.ToUpper(strings.TrimSpace(el.Text))
	return structuralHeadings[normalized]
}
