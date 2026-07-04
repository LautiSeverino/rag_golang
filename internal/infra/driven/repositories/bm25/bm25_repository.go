package bm25

import (
	"context"
	"encoding/gob"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode"

	"rag_golang/internal/core/domain/chunk"
	"rag_golang/internal/core/domain/search"

	"github.com/google/uuid"
)

// invertedEntry es una entrada en el índice invertido para un término.
type invertedEntry struct {
	ChunkIdx int     // índice en r.chunks
	TF       float64 // frecuencia del término en el chunk
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

func NewRepository(k1, b float64) *BM25Repository {
	if k1 == 0 {
		k1 = 1.2
	}
	if b == 0 {
		b = 0.75
	}
	return &BM25Repository{
		index: make(map[string][]invertedEntry),
		k1:    k1,
		b:     b,
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
				ChunkIdx: idx,
				TF:       float64(count),
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
			docLen := float64(r.docLens[e.ChunkIdx])
			// TF normalizado con saturación
			tfNorm := (e.TF * (r.k1 + 1)) /
				(e.TF + r.k1*(1-r.b+r.b*(docLen/r.avgDocLen)))

			scores[e.ChunkIdx] += idf * tfNorm
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
				ChunkIdx: i,
				TF:       float64(count),
			})
		}
	}
	r.avgDocLen = computeAvgDocLen(r.docLens)
	return nil
}

type persistedState struct {
	Chunks    []chunk.Chunk
	Index     map[string][]invertedEntry
	DocLens   []int
	AvgDocLen float64
}

func (r *BM25Repository) SaveToDisk(path string) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("bm25: create file %q: %w", path, err)
	}
	defer f.Close()

	state := persistedState{
		Chunks:    r.chunks,
		Index:     r.index,
		DocLens:   r.docLens,
		AvgDocLen: r.avgDocLen,
	}
	return gob.NewEncoder(f).Encode(state)
}

func (r *BM25Repository) LoadFromDisk(path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // primera vez, no hay nada que cargar
		}
		return fmt.Errorf("bm25: open file %q: %w", path, err)
	}
	defer f.Close()

	var state persistedState
	if err := gob.NewDecoder(f).Decode(&state); err != nil {
		return fmt.Errorf("bm25: decode: %w", err)
	}

	r.chunks = state.Chunks
	r.index = state.Index
	r.docLens = state.DocLens
	r.avgDocLen = state.AvgDocLen
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
