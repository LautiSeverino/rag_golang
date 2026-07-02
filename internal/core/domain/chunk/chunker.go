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
				// Sub-chunking: agrupar elementos hasta MaxSize, no 1 por 1
				// Sección demasiado larga: agrupar elementos en sub-chunks de hasta MaxSize.
				// Mejor que 1 elemento por chunk porque preserva coherencia semántica local.
				var bufRaw []string
				bufPage := els[0].Page
				bufSize := runeLen(prefix) // el prefix ocupa espacio en cada sub-chunk

				flush := func() {
					if len(bufRaw) == 0 {
						return
					}
					rawText := strings.Join(bufRaw, "\n")
					text := prefix + rawText
					ch := makeChunk(
						doc.ID, doc.Metadata.Source,
						text, rawText,
						domain.ElemParagraph, sectionPath, bufPage, idx,
					)
					chunks = append(chunks, ch)
					idx++
					bufRaw = bufRaw[:0]
					bufSize = runeLen(prefix)
				}

				for _, e := range els {
					eLen := runeLen(e.Text) + 1 // +1 por el "\n" separador
					if bufSize+eLen > int(cfg.MaxSize) && len(bufRaw) > 0 {
						flush()
					}
					if len(bufRaw) == 0 {
						bufPage = e.Page // la página del primer elemento de este sub-chunk
					}
					bufRaw = append(bufRaw, e.Text)
					bufSize += eLen
				}
				flush() // vaciar lo que quedó en el buffer
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
		// error explícito para que el operador sepa que hay un problema de config
		return nil, fmt.Errorf("chunker: estrategia desconocida %q. Valores válidos: section, element, sliding", cfg.Strategy)
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
