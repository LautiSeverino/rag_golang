package chunk

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"rag_golang/internal/core/domain"

	"github.com/google/uuid"
)

// Chunker es un componente puro de dominio que convierte un *Document
// en una lista de `domain.Chunk` según la `domain.ChunkConfig`.
type Chunker struct{}

func NewChunker() *Chunker { return &Chunker{} }

// Chunk produce los chunks del documento según la estrategia del cfg.
// No usa interfaces ni puertos porque es lógica pura y determinista.
func (c *Chunker) Chunk(doc *domain.Document, cfg ChunkConfig) ([]Chunk, error) {
	if doc == nil {
		return nil, fmt.Errorf("document is nil")
	}
	if cfg.MaxSize <= 0 {
		// fallback razonable
		cfg.MaxSize = ChunkMaxSizeMedium
	}

	var chunks []Chunk
	idx := 0

	switch cfg.Strategy {
	case ChunkElement:
		for _, el := range doc.Elements {
			text := el.Text
			if cfg.ContextPrefix && len(el.SectionPath) > 0 {
				prefix := strings.Join(el.SectionPath, " > ")
				text = prefix + "\n\n" + text
			}
			ch := makeChunk(doc.ID, doc.Metadata.Source, text, el.Text, el.Type, el.SectionPath, el.Page, idx)
			chunks = append(chunks, ch)
			idx++
		}

	case ChunkSection:
		groups := make(map[string][]domain.Element)
		order := make([]string, 0)
		for _, el := range doc.Elements {
			key := strings.Join(el.SectionPath, "|")
			if _, ok := groups[key]; !ok {
				order = append(order, key)
			}
			groups[key] = append(groups[key], el)
		}

		for _, key := range order {
			els := groups[key]
			if len(els) == 0 {
				continue
			}

			page := els[0].Page
			sectionPath := els[0].SectionPath

			// Construir el texto raw combinando TODOS los elementos.
			// Incluimos los headings: su texto es el título de sección, que
			// aporta señal léxica directa en BM25 y en el embedding.
			rawParts := make([]string, 0, len(els))
			for _, e := range els {
				if strings.TrimSpace(e.Text) != "" {
					rawParts = append(rawParts, e.Text)
				}
			}
			rawCombined := strings.Join(rawParts, "\n")

			prefix := ""
			if cfg.ContextPrefix && len(sectionPath) > 0 {
				prefix = strings.Join(sectionPath, " > ") + "\n\n"
			}
			combined := prefix + rawCombined

			if runeLen(combined) <= int(cfg.MaxSize) {
				// Sección entra en un solo chunk.
				elemType := domain.ElemParagraph
				if len(els) == 1 {
					elemType = els[0].Type
				}
				ch := makeChunk(
					doc.ID, doc.Metadata.Source,
					combined, rawCombined,
					elemType, sectionPath, page, idx,
				)
				chunks = append(chunks, ch)
				idx++
			} else {
				// Sección demasiado larga: un chunk por elemento con prefix.
				// Las tablas siempre llevan prefix independientemente del flag.
				for _, e := range els {
					text := e.Text
					if e.Type == domain.ElemTable || cfg.ContextPrefix {
						text = prefix + e.Text
					}
					ch := makeChunk(
						doc.ID, doc.Metadata.Source,
						text, e.Text,
						e.Type, e.SectionPath, e.Page, idx,
					)
					chunks = append(chunks, ch)
					idx++
				}
			}
		}
	case ChunkSliding:
		// Construir texto plano y mantener mapeo de offsets a elementos
		texts := make([]string, 0, len(doc.Elements))
		acc := 0
		for _, e := range doc.Elements {
			texts = append(texts, e.Text)
			acc += runeLen(e.Text) + 2 // accounting for separator
		}
		full := strings.Join(texts, "\n\n")
		runes := []rune(full)
		step := int(cfg.MaxSize) - cfg.Overlap
		if step <= 0 {
			step = int(cfg.MaxSize)
		}
		for start := 0; start < len(runes); start += step {
			end := start + int(cfg.MaxSize)
			if end > len(runes) {
				end = len(runes)
			}
			window := string(runes[start:end])
			// inferir página y sectionPath a partir del primer elemento que contribuye
			page, sectionPath := findMetadataForOffset(doc.Elements, start)
			ch := makeChunk(doc.ID, doc.Metadata.Source, window, window, domain.ElemParagraph, sectionPath, page, idx)
			chunks = append(chunks, ch)
			idx++
			if end == len(runes) {
				break
			}
		}

	default:
		// usar default config:
		// Strategy:      ChunkSection,
		// MaxSize:       SizeMedium,
		// Overlap:       DefaultOverlap,
		// ContextPrefix: true,
		config := DefaultConfig()
		return c.Chunk(doc, config)
	}

	return chunks, nil
}

func makeChunk(docID uuid.UUID, source, text, raw string, et domain.ElementType, section []string, page, idx int) Chunk {
	sum := sha256.Sum256([]byte(text))
	hash := fmt.Sprintf("%x", sum)
	seed := fmt.Sprintf("%s-%d", docID.String(), idx)

	return Chunk{
		ID:          uuid.NewSHA1(uuid.NameSpaceURL, []byte(seed)),
		DocID:       docID,
		Text:        text,
		RawText:     raw,
		ElementType: et,
		SectionPath: section,
		Page:        page,
		ChunkIndex:  idx,
		Source:      source,
		Hash:        hash,
	}
}

func runeLen(s string) int { return len([]rune(s)) }

// findMetadataForOffset devuelve la página y sectionPath del elemento
// que contiene el offset en runes aproximado. Si no se encuentra, devuelve 0, nil.
func findMetadataForOffset(elements []domain.Element, offset int) (int, []string) {
	acc := 0
	for _, e := range elements {
		l := runeLen(e.Text) + 2
		if offset >= acc && offset < acc+l {
			return e.Page, e.SectionPath
		}
		acc += l
	}
	if len(elements) > 0 {
		return elements[0].Page, elements[0].SectionPath
	}
	return 0, nil
}
