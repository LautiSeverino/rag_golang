package query

import (
	"fmt"
	"rag_golang/internal/core/domain"
	"rag_golang/internal/core/domain/llm"
	"rag_golang/internal/core/domain/search"
	"sort"
	"strings"
	"time"
)

// QueryResult es el resultado completo de una consulta al sistema RAG.
type QueryResult struct {
	Query    string          `json:"query"`
	Answer   string          `json:"answer"`
	Sources  []domain.Source `json:"sources,omitempty"`
	Duration time.Duration   `json:"duration_ns"`
}

// BuildQueryResult construye el QueryResult final a partir de la respuesta del LLM
// y los chunks utilizados.
//
// Params:
// - query: pregunta original
// - tokensChan: channel de tokens del LLM (streaming)
// - fusedResults: chunks utilizados para generar la respuesta
//
// Retorna: QueryResult completo con respuesta y fuentes
func BuildQueryResult(
	query string,
	tokensChan <-chan llm.GenerateToken,
	fusedResults []search.SearchResult,
) *QueryResult {
	// Leer todos los tokens del channel y concatenarlos en la respuesta
	var answer strings.Builder
	for token := range tokensChan {
		answer.WriteString(token.Text)
	}

	// Extraer fuentes únicas de los chunks utilizados
	sources := extractSources(fusedResults)

	return &QueryResult{
		Query:    query,
		Answer:   answer.String(),
		Sources:  sources,
		Duration: 0, // La duración se calcularía en el nivel de transporte/handler
	}
}

// extractSources convierte los SearchResult en Source entries para el resultado.
// Deduplica por archivo y sección, manteniendo el score más alto.
func extractSources(results []search.SearchResult) []domain.Source {
	// Map para deduplicación: (archivo + sección) -> Source
	sourceMap := make(map[string]domain.Source)

	for _, result := range results {
		chk := result.Chunk

		// Crear clave única por archivo + sección
		key := fmt.Sprintf("%s:%s:%d", chk.Source, strings.Join(chk.SectionPath, "|"), chk.Page)

		// Si ya existe y tiene mejor score, mantener
		if existing, exists := sourceMap[key]; exists && existing.Score >= result.Score {
			continue
		}

		sourceMap[key] = domain.Source{
			File:        chk.Source,
			Page:        chk.Page,
			SectionPath: chk.SectionPath,
			ElementType: chk.ElementType,
			Score:       result.Score,
			Excerpt:     truncateExcerpt(chk.RawText, 200),
		}
	}

	// Convertir map a slice
	sources := make([]domain.Source, 0, len(sourceMap))
	for _, source := range sourceMap {
		sources = append(sources, source)
	}

	// Ordenar por score descendente
	sort.Slice(sources, func(i, j int) bool {
		return sources[i].Score > sources[j].Score
	})

	return sources
}

// truncateExcerpt limita un string a N caracteres, respetando límites de palabra.
func truncateExcerpt(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}

	// Truncar y buscar el último espacio para no cortar palabra
	truncated := text[:maxLen]
	lastSpace := strings.LastIndex(truncated, " ")
	if lastSpace > 0 && lastSpace > maxLen-50 {
		truncated = truncated[:lastSpace] + "…"
	} else {
		truncated = truncated + "…"
	}

	return truncated
}
