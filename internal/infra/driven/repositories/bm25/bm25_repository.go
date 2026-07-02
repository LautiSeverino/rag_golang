package bm25

import (
	"context"
	"math"
	"sort"
	"strings"
	"sync"
	"unicode"

	"rag_golang/internal/core/domain/chunk"
	"rag_golang/internal/core/domain/search"

	"github.com/google/uuid"
)

const (
	defaultK1 = 1.2  // controla la saturación de TF
	defaultB  = 0.75 // controla la normalización por longitud de documento
)

// invertedEntry es una entrada en el índice invertido para un término.
type invertedEntry struct {
	chunkIdx int     // índice en r.chunks
	tf       float64 // frecuencia del término en el chunk
}

// BM25Repository implementa out.IBM25Repository con un índice invertido en memoria.
// El índice se reconstruye en memoria al arrancar; no persiste en disco en esta versión.
// Para producción con miles de documentos, considerar serialización a disco.
type BM25Repository struct {
	mu        sync.RWMutex
	chunks    []chunk.Chunk
	index     map[string][]invertedEntry // término → entradas
	docLens   []int                      // longitud en tokens de cada chunk
	avgDocLen float64
	k1        float64
	b         float64
}

func NewRepository() *BM25Repository {
	return &BM25Repository{
		index: make(map[string][]invertedEntry),
		k1:    defaultK1,
		b:     defaultB,
	}
}

// Index agrega los chunks al índice invertido.
// Se puede llamar múltiples veces (indexación incremental).
func (r *BM25Repository) Index(_ context.Context, chunks []chunk.Chunk) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	startIdx := len(r.chunks)
	r.chunks = append(r.chunks, chunks...)

	for i, ch := range chunks {
		idx := startIdx + i
		terms := tokenize(ch.Text)
		r.docLens = append(r.docLens, len(terms))

		// Contar frecuencia de cada término en el chunk
		tf := make(map[string]int, len(terms))
		for _, t := range terms {
			tf[t]++
		}

		for term, count := range tf {
			r.index[term] = append(r.index[term], invertedEntry{
				chunkIdx: idx,
				tf:       float64(count),
			})
		}
	}

	r.avgDocLen = computeAvgDocLen(r.docLens)
	return nil
}

// Search ejecuta la búsqueda BM25 y devuelve los top-K resultados ordenados por score.
func (r *BM25Repository) Search(_ context.Context, req search.BM25SearchRequest) ([]search.BM25Result, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.chunks) == 0 {
		return nil, nil
	}

	queryTerms := tokenize(req.Query)
	if len(queryTerms) == 0 {
		return nil, nil
	}

	N := float64(len(r.chunks))
	scores := make(map[int]float64, len(r.chunks))

	for _, term := range queryTerms {
		entries, ok := r.index[term]
		if !ok {
			continue
		}

		// IDF con suavizado para evitar log(0)
		// Fórmula: log((N - df + 0.5) / (df + 0.5) + 1)
		df := float64(len(entries))
		idf := math.Log((N-df+0.5)/(df+0.5) + 1)

		for _, e := range entries {
			docLen := float64(r.docLens[e.chunkIdx])
			// TF normalizado con saturación
			tfNorm := (e.tf * (r.k1 + 1)) /
				(e.tf + r.k1*(1-r.b+r.b*(docLen/r.avgDocLen)))

			scores[e.chunkIdx] += idf * tfNorm
		}
	}

	// Aplicar filtros de metadata
	results := make([]search.BM25Result, 0, len(scores))
	for idx, score := range scores {
		ch := r.chunks[idx]
		if req.Filter != nil && !matchesFilter(ch, req.Filter) {
			continue
		}
		results = append(results, search.BM25Result{Chunk: ch, Score: score})
	}

	// Ordenar por score descendente
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	topK := req.TopK
	if topK <= 0 || topK > len(results) {
		topK = len(results)
	}
	return results[:topK], nil
}

// DeleteByDocID elimina todos los chunks de un documento del índice.
// Reconstruye el índice después de la eliminación.
func (r *BM25Repository) DeleteByDocID(_ context.Context, docID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Filtrar los chunks que NO son del documento
	kept := r.chunks[:0]
	for _, ch := range r.chunks {
		if ch.DocID != docID {
			kept = append(kept, ch)
		}
	}
	r.chunks = kept

	// Reconstruir el índice completo
	r.index = make(map[string][]invertedEntry)
	r.docLens = r.docLens[:0]

	for i, ch := range r.chunks {
		terms := tokenize(ch.Text)
		r.docLens = append(r.docLens, len(terms))
		tf := make(map[string]int)
		for _, t := range terms {
			tf[t]++
		}
		for term, count := range tf {
			r.index[term] = append(r.index[term], invertedEntry{
				chunkIdx: i,
				tf:       float64(count),
			})
		}
	}
	r.avgDocLen = computeAvgDocLen(r.docLens)
	return nil
}

// ─── helpers ──────────────────────────────────────────────────────────────────

// tokenize convierte texto a términos normalizados.
// Lowercase + split por no-letra/no-dígito.
// No hace stemming: es simple y suficiente para español/inglés.
func tokenize(text string) []string {
	lower := strings.ToLower(text)
	terms := strings.FieldsFunc(lower, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	// Filtrar términos de un solo carácter (artículos, preposiciones cortas)
	result := terms[:0]
	for _, t := range terms {
		if len([]rune(t)) > 1 {
			result = append(result, t)
		}
	}
	return result
}

func computeAvgDocLen(lens []int) float64 {
	if len(lens) == 0 {
		return 0
	}
	sum := 0
	for _, l := range lens {
		sum += l
	}
	return float64(sum) / float64(len(lens))
}

func matchesFilter(ch chunk.Chunk, f *search.SearchFilter) bool {
	if f.ElementType != nil && ch.ElementType != *f.ElementType {
		return false
	}
	if f.Source != nil && ch.Source != *f.Source {
		return false
	}
	if f.MinPage != nil && ch.Page < *f.MinPage {
		return false
	}
	if f.MaxPage != nil && ch.Page > *f.MaxPage {
		return false
	}
	return true
}
