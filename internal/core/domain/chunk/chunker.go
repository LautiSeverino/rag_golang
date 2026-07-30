package chunk

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"unicode/utf8"

	"rag_golang/internal/core/domain"

	"github.com/google/uuid"
)

// Chunker es un componente puro de dominio que convierte un *Document
// en una lista de `domain.Chunk` según la `domain.ChunkConfig`.
type Chunker struct{}

func NewChunker() *Chunker { return &Chunker{} }

// Chunk divide un documento en chunks respetando su jerarquía de secciones.
func (c *Chunker) Chunk(doc *domain.Document, cfg ChunkConfig) ([]Chunk, error) {
	if doc == nil {
		return nil, fmt.Errorf("document is nil")
	}

	var chunks []Chunk
	idx := 0

	// Agrupar elementos por su ruta de sección (e.g. "Intro|Métodos")
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

		// Construir el texto combinado de la sección (sin prefijo)
		rawParts := make([]string, 0, len(els))
		for _, e := range els {
			if strings.TrimSpace(e.Text) != "" {
				rawParts = append(rawParts, e.Text)
			}
		}
		rawCombined := strings.Join(rawParts, "\n")

		// Prefijo de contexto (opcional)
		prefix := ""
		if cfg.ContextPrefix && len(sectionPath) > 0 {
			prefix = strings.Join(sectionPath, " > ") + "\n\n"
		}
		combined := prefix + rawCombined

		// Si la sección entera cabe en un chunk, lo guardamos directamente
		if runeLen(combined) <= cfg.MaxSize {
			ch := Chunk{
				DocID:       doc.ID,
				Source:      doc.Metadata.Source,
				Text:        combined,
				RawText:     rawCombined,
				ElementType: domain.ElemParagraph,
				SectionPath: sectionPath,
				Page:        page,
				ChunkIndex:  idx,
			}
			if len(els) == 1 {
				ch.ElementType = els[0].Type
			}
			chunks = append(chunks, makeChunk(ch))
			idx++
			continue
		}

		// --- Sub-chunking: la sección es más larga que MaxSize ---
		var (
			bufRaw      []string
			bufPage     = page
			bufSize     = runeLen(prefix)
			lastRawText string // texto sin prefijo del chunk anterior para solapamiento
		)

		// flush guarda el buffer actual como un chunk, aplicando overlap si corresponde.
		flush := func() {
			if len(bufRaw) == 0 {
				return
			}
			rawText := strings.Join(bufRaw, "\n")
			text := prefix + rawText

			ch := Chunk{
				DocID:       doc.ID,
				Source:      doc.Metadata.Source,
				Text:        text,
				RawText:     rawText,
				ElementType: domain.ElemParagraph,
				SectionPath: sectionPath,
				Page:        bufPage,
				ChunkIndex:  idx,
			}
			chunks = append(chunks, makeChunk(ch))
			idx++

			// Guardamos el texto para el posible overlap del siguiente chunk
			lastRawText = rawText

			// Reiniciar buffer
			bufRaw = bufRaw[:0]
			bufSize = runeLen(prefix)

			// Si hay overlap, pre‑cargamos el final del chunk anterior
			if cfg.Overlap > 0 && len(lastRawText) > 0 {
				overlapLen := cfg.Overlap
				if overlapLen > len(lastRawText) {
					overlapLen = len(lastRawText)
				}
				// Tomamos los últimos 'overlapLen' runas
				suffix := lastRunas(lastRawText, overlapLen)
				if suffix != "" {
					bufRaw = append(bufRaw, suffix)
					bufSize += runeLen(suffix) + 1 // +1 por el '\n' que añadiremos después
				}
			}
		}

		for _, e := range els {
			// Si el elemento actual cabe en el buffer, lo añadimos
			if bufSize+runeLen(e.Text)+1 <= cfg.MaxSize {
				if len(bufRaw) == 0 {
					bufPage = e.Page
				}
				bufRaw = append(bufRaw, e.Text)
				bufSize += runeLen(e.Text) + 1
				continue
			}

			// No cabe entero: vaciamos el buffer actual y luego tratamos el elemento
			flush()

			// Si aun con buffer vacío el elemento es más grande que el espacio disponible
			if runeLen(e.Text) > cfg.MaxSize-runeLen(prefix) {
				// Troceamos el texto del elemento en sub‑fragmentos que respeten MaxSize
				subTexts := splitText(e.Text, cfg.MaxSize-runeLen(prefix))
				for _, sub := range subTexts {
					ch := Chunk{
						DocID:       doc.ID,
						Source:      doc.Metadata.Source,
						Text:        prefix + sub,
						RawText:     sub,
						ElementType: e.Type,
						SectionPath: sectionPath,
						Page:        e.Page,
						ChunkIndex:  idx,
					}
					chunks = append(chunks, makeChunk(ch))
					idx++

					// Para el overlap en el siguiente chunk (si lo hubiera) guardamos el último sub‑texto
					lastRawText = sub
				}
				// Reiniciamos buffer por si hay más elementos, con overlap del último sub‑chunk
				bufRaw = bufRaw[:0]
				bufSize = runeLen(prefix)
				if cfg.Overlap > 0 && len(lastRawText) > 0 {
					overlapLen := cfg.Overlap
					if overlapLen > len(lastRawText) {
						overlapLen = len(lastRawText)
					}
					suffix := lastRunas(lastRawText, overlapLen)
					if suffix != "" {
						bufRaw = append(bufRaw, suffix)
						bufSize += runeLen(suffix) + 1
					}
				}
			} else {
				// Cabe en un buffer nuevo, lo añadimos (la página será la del elemento)
				bufPage = e.Page
				bufRaw = append(bufRaw, e.Text)
				bufSize += runeLen(e.Text) + 1
			}
		}
		flush() // vaciar el último buffer de la sección
	}

	return chunks, nil
}

// makeChunk completa los metadatos del chunk y genera su ID e integridad.
func makeChunk(chunk Chunk) Chunk {
	sum := sha256.Sum256([]byte(chunk.Text))
	hash := fmt.Sprintf("%x", sum)
	seed := fmt.Sprintf("%s-%d", chunk.DocID.String(), chunk.ChunkIndex)

	chunk.ID = uuid.NewSHA1(uuid.NameSpaceURL, []byte(seed))
	chunk.Hash = hash
	return chunk
}

// splitText divide un texto en partes que no excedan maxSize runas.
// Intenta partir por palabras, y si una palabra sola es demasiado larga, por caracteres.
func splitText(text string, maxSize int) []string {
	if maxSize <= 0 {
		return []string{text}
	}
	if runeLen(text) <= maxSize {
		return []string{text}
	}

	// Separar por palabras (espacios)
	words := strings.Fields(text)
	var chunks []string
	var current []string
	currentSize := 0

	for _, w := range words {
		wLen := runeLen(w)
		// Si la palabra sola ya excede el máximo, la troceamos carácter a carácter
		if wLen > maxSize {
			// Primero vaciamos lo acumulado
			if len(current) > 0 {
				chunks = append(chunks, strings.Join(current, " "))
				current = nil
				currentSize = 0
			}
			// Dividimos la palabra larga en fragmentos de maxSize
			runes := []rune(w)
			for i := 0; i < len(runes); i += maxSize {
				end := i + maxSize
				if end > len(runes) {
					end = len(runes)
				}
				chunks = append(chunks, string(runes[i:end]))
			}
			continue
		}

		// Comprobar si añadiendo la palabra (más espacio si no es el primero) excedemos maxSize
		needed := wLen
		if len(current) > 0 {
			needed += 1 // espacio separador
		}
		if currentSize+needed > maxSize {
			// Guardamos el chunk actual y empezamos uno nuevo con esta palabra
			chunks = append(chunks, strings.Join(current, " "))
			current = []string{w}
			currentSize = wLen
		} else {
			current = append(current, w)
			currentSize += needed
		}
	}
	if len(current) > 0 {
		chunks = append(chunks, strings.Join(current, " "))
	}

	return chunks
}

// lastRunas devuelve las últimas n runas de s. Si n > len(s) devuelve s completa.
func lastRunas(s string, n int) string {
	runes := []rune(s)
	if n >= len(runes) {
		return s
	}
	return string(runes[len(runes)-n:])
}

// runeLen cuenta el número de runas (caracteres Unicode) de una cadena.
func runeLen(s string) int {
	return utf8.RuneCountInString(s)
}
