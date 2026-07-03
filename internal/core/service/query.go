package service

import (
	"context"
	"fmt"
	"log"
	"rag_golang/internal/configs"
	"rag_golang/internal/core/domain/llm"
	"rag_golang/internal/core/domain/query"
	"rag_golang/internal/core/domain/search"
	"rag_golang/internal/core/ports/out"
	"strings"
)

type QueryService struct {
	embedder   out.IEmbedderPort
	vectorRepo out.IVectorRepository
	bm25Repo   out.IBM25Repository
	llm        out.ILLMPort
	cfg        configs.Config
}

// No existe NewQueryService
func NewQueryService(
	embedder out.IEmbedderPort,
	vectorRepo out.IVectorRepository,
	bm25Repo out.IBM25Repository,
	llm out.ILLMPort,
	cfg configs.Config,
) *QueryService {
	return &QueryService{
		embedder:   embedder,
		vectorRepo: vectorRepo,
		bm25Repo:   bm25Repo,
		llm:        llm,
		cfg:        cfg,
	}
}

type retrievalResult struct {
	fused []search.SearchResult
}

func (s *QueryService) retrieve(ctx context.Context, userQuery string) (*retrievalResult, error) {
	// Aplicar prefijo de query para nomic-embed-text
	queryToEmbed := s.cfg.Embed.QueryPrefix + userQuery

	vecs, err := s.embedder.Embed(ctx, []string{queryToEmbed})
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("no vectors returned")
	}

	// Usar candidates_k del config en lugar del 20 hardcodeado
	candidatesK := s.cfg.Search.CandidatesK

	denseResults, err := s.vectorRepo.Search(ctx, search.SearchRequest{
		Vector: vecs[0],
		TopK:   candidatesK,
	})
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}

	sparseResults, err := s.bm25Repo.Search(ctx, search.BM25SearchRequest{
		Query: userQuery,
		TopK:  candidatesK,
	})

	// RRF con pool más grande para tener margen antes de deduplicar
	rrfPoolSize := s.cfg.Search.TopK * 4 // candidatos extra para sobrevivir a la dedup
	if rrfPoolSize < s.cfg.Search.TopK {
		rrfPoolSize = s.cfg.Search.TopK
	}

	// ─── LOGS DE DIAGNÓSTICO ──────────────────────────────────────────
	log.Printf("[RETRIEVE] query=%q", userQuery)
	log.Printf("[DENSE] top-%d resultados:", len(denseResults))
	for i, r := range denseResults {
		sectionKey := strings.Join(r.Chunk.SectionPath, " > ")
		log.Printf("  [%d] score=%.4f  section=%q  page=%d",
			i+1, r.Score, sectionKey, r.Chunk.Page)
	}
	log.Printf("[BM25] top-%d resultados:", len(sparseResults))
	for i, r := range sparseResults {
		sectionKey := strings.Join(r.Chunk.SectionPath, " > ")
		log.Printf("  [%d] score=%.4f  section=%q  page=%d",
			i+1, r.Score, sectionKey, r.Chunk.Page)
	}
	// ──────────────────────────────────────────────────────────────────
	if err != nil {
		return nil, fmt.Errorf("bm25 search: %w", err)
	}

	fused := search.Rrf(denseResults, sparseResults, s.cfg.Search.RRFK, rrfPoolSize)

	// Deduplicar: no más de MaxChunksPerSection por sección
	if s.cfg.Search.MaxChunksPerSection > 0 {
		fused = limitChunksPerSection(fused, s.cfg.Search.MaxChunksPerSection)
	}

	// Recortar al top_k final
	if len(fused) > s.cfg.Search.TopK {
		fused = fused[:s.cfg.Search.TopK]
	}

	// ─── LOG DE RRF ───────────────────────────────────────────────────
	log.Printf("[RRF] top-%d fusionados:", len(fused))
	for i, r := range fused {
		sectionKey := strings.Join(r.Chunk.SectionPath, " > ")
		log.Printf("  [%d] rrf_score=%.6f  section=%q  page=%d",
			i+1, r.Score, sectionKey, r.Chunk.Page)
	}
	// ──────────────────────────────────────────────────────────────────
	return &retrievalResult{fused: fused}, nil
}

func (s *QueryService) Query(ctx context.Context, userQuery string) (*query.QueryResult, error) {
	res, err := s.retrieve(ctx, userQuery)
	if err != nil {
		return nil, fmt.Errorf("query service: retrieve: %w", err)
	}
	tokensChan, err := s.llm.Generate(ctx, llm.BuildRequest(userQuery, res.fused, s.cfg.LLM.Model, s.cfg.LLM.Options, s.cfg.LLM.MaxChunkLength))
	if err != nil {
		return nil, fmt.Errorf("query service: llm generate: %w", err)
	}
	return query.BuildQueryResult(userQuery, tokensChan, res.fused), nil
}

func (s *QueryService) QueryStream(ctx context.Context, userQuery string) (<-chan llm.GenerateToken, error) {
	res, err := s.retrieve(ctx, userQuery)
	if err != nil {
		return nil, fmt.Errorf("query service: retrieve: %w", err)
	}
	req := llm.BuildRequest(userQuery, res.fused, s.cfg.LLM.Model, s.cfg.LLM.Options, s.cfg.LLM.MaxChunkLength)
	req.Stream = true
	tokenCh, err := s.llm.Generate(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("query service: llm generate: %w", err)
	}
	return tokenCh, nil
}

func limitChunksPerSection(results []search.SearchResult, maxPerSection int) []search.SearchResult {
	sectionCounts := make(map[string]int, len(results))
	deduped := make([]search.SearchResult, 0, len(results))

	for _, r := range results {
		key := strings.Join(r.Chunk.SectionPath, "|")
		if sectionCounts[key] < maxPerSection {
			deduped = append(deduped, r)
			sectionCounts[key]++
		}
	}
	return deduped
}
