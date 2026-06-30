package search

import (
	"rag_golang/internal/core/domain/chunk"
	"sort"

	"github.com/google/uuid"
)

// Rrf implementa Reciprocal Rank Fusion puro para combinar búsqueda densa y esparsa.
// Fusiona los rankings basándose exclusivamente en la posición de los elementos.
//
// Params:
// - denseResults: resultados de búsqueda vectorial (Qdrant)
// - sparseResults: resultados de búsqueda BM25
// - rrfK: constante de suavizado (típicamente 60)
// - topK: número máximo de resultados a devolver
//
// Retorna: lista de SearchResult ordenada por score RRF descendente, deduplicated
func Rrf(
	denseResults []SearchResult,
	sparseResults []BM25Result,
	rrfK int,
	topK int,
) []SearchResult {
	// 1. Mapas para acumular scores RRF y guardar la metadata del Chunk
	rrfScores := make(map[uuid.UUID]float32)
	chunkMap := make(map[uuid.UUID]chunk.Chunk)

	// 2. Procesar resultados de búsqueda densa (Qdrant)
	for rank, result := range denseResults {
		id := result.Chunk.ID
		rrfScore := float32(1.0 / float64(rrfK+rank+1))
		rrfScores[id] += rrfScore
		chunkMap[id] = result.Chunk
	}

	// 3. Procesar resultados de búsqueda esparsa (BM25)
	// ¡RRF PURO!: No multiplicamos por el score de BM25. Solo usamos la posición (rank).
	for rank, result := range sparseResults {
		id := result.Chunk.ID
		rrfScore := float32(1.0 / float64(rrfK+rank+1))
		rrfScores[id] += rrfScore

		// Si el chunk no estaba en los densos, lo agregamos al mapa de metadatos
		if _, exists := chunkMap[id]; !exists {
			chunkMap[id] = result.Chunk
		}
	}

	// 4. Convertir el mapa unificado a un slice para ordenar
	scored := make([]SearchResult, 0, len(rrfScores))
	for id, rrfScore := range rrfScores {
		scored = append(scored, SearchResult{
			Chunk: chunkMap[id],
			Score: rrfScore, // El score final de este SearchResult es el score RRF
		})
	}

	// 5. Ordenar por score RRF descendente
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	// 6. Recortar al topK solicitado
	if len(scored) > topK {
		return scored[:topK]
	}

	return scored
}
