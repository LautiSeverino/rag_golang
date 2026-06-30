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
	// stack[i] guarda el título del heading de nivel i+1
	stack := make([]string, 6)
	depth := 0

	for i, el := range elements {
		if el.Type != domain.ElemHeading {
			if depth > 0 {
				elements[i].SectionPath = nonEmpty(stack[:depth])
			}
			continue
		}

		lvl := el.Level
		if lvl < 1 {
			lvl = 1
		}
		if lvl > 6 {
			lvl = 6
		}

		// Actualizar la pila: limpiar niveles más profundos
		stack[lvl-1] = el.Text
		for j := lvl; j < 6; j++ {
			stack[j] = ""
		}
		depth = lvl

		elements[i].SectionPath = nonEmpty(stack[:depth])
	}

	return elements
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
