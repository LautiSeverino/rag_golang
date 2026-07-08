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
	"time"
)

type QueryService struct {
	embedder   out.IEmbedderPort
	vectorRepo out.IVectorRepository
	bm25Repo   out.IBM25Repository
	llm        out.ILLMPort
	cfg        configs.Config
	logger     *log.Logger
}

// No existe NewQueryService
func NewQueryService(
	embedder out.IEmbedderPort,
	vectorRepo out.IVectorRepository,
	bm25Repo out.IBM25Repository,
	llm out.ILLMPort,
	cfg configs.Config,
	logger *log.Logger,
) *QueryService {
	return &QueryService{
		embedder:   embedder,
		vectorRepo: vectorRepo,
		bm25Repo:   bm25Repo,
		llm:        llm,
		cfg:        cfg,
		logger:     logger,
	}
}

type retrievalResult struct {
	fused []search.SearchResult
}

func (s *QueryService) retrieve(ctx context.Context, userQuery string) (*retrievalResult, error) {
	startTotal := time.Now()

	if s.logger != nil {
		s.logger.Printf("================================================================================")
		s.logger.Printf("RAG RETRIEVE START")
		s.logger.Printf("QUERY: %s", userQuery)
		s.logger.Printf("================================================================================")
	}

	// 1) Embedding
	t0 := time.Now()
	queryToEmbed := s.cfg.Embed.QueryPrefix + userQuery

	vecs, err := s.embedder.Embed(ctx, []string{queryToEmbed})
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("no vectors returned")
	}
	if s.logger != nil {
		s.logger.Printf("[EMBEDDING] model=%s duration=%s", s.cfg.Embed.Model, time.Since(t0))
	}

	// 2) Dense search
	t1 := time.Now()
	candidatesK := s.cfg.Search.CandidatesK

	denseResults, err := s.vectorRepo.Search(ctx, search.SearchRequest{
		Vector: vecs[0],
		TopK:   candidatesK,
	})
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	denseResults = deduplicateDensePool(denseResults, 3)

	if s.logger != nil {
		s.logger.Printf("[DENSE] top=%d duration=%s", len(denseResults), time.Since(t1))
		for i, r := range denseResults {
			sectionKey := strings.Join(r.Chunk.SectionPath, " > ")
			s.logger.Printf("  [%d] score=%.6f page=%d section=%q", i+1, r.Score, r.Chunk.Page, sectionKey)
		}
	}

	// 3) BM25
	t2 := time.Now()
	sparseResults, err := s.bm25Repo.Search(ctx, search.BM25SearchRequest{
		Query: userQuery,
		TopK:  candidatesK,
	})
	if err != nil {
		return nil, fmt.Errorf("bm25 search: %w", err)
	}

	if s.logger != nil {
		s.logger.Printf("[BM25] top=%d duration=%s", len(sparseResults), time.Since(t2))
		for i, r := range sparseResults {
			sectionKey := strings.Join(r.Chunk.SectionPath, " > ")
			s.logger.Printf("  [%d] score=%.6f page=%d section=%q", i+1, r.Score, r.Chunk.Page, sectionKey)
		}
	}

	// 4) RRF
	t3 := time.Now()
	rrfPoolSize := s.cfg.Search.TopK * 4
	if rrfPoolSize < s.cfg.Search.TopK {
		rrfPoolSize = s.cfg.Search.TopK
	}

	fused := search.Rrf(denseResults, sparseResults, s.cfg.Search.RRFK, rrfPoolSize)

	if s.cfg.Search.MaxChunksPerSection > 0 {
		fused = limitChunksPerSection(fused, s.cfg.Search.MaxChunksPerSection)
	}

	if len(fused) > s.cfg.Search.TopK {
		fused = fused[:s.cfg.Search.TopK]
	}

	if s.logger != nil {
		s.logger.Printf("[RRF] top=%d duration=%s", len(fused), time.Since(t3))
		for i, r := range fused {
			sectionKey := strings.Join(r.Chunk.SectionPath, " > ")
			s.logger.Printf("  [%d] score=%.6f page=%d section=%q", i+1, r.Score, r.Chunk.Page, sectionKey)
		}
		s.logger.Printf("TIMINGS total=%s embedding=%s dense=%s bm25=%s rrf=%s",
			time.Since(startTotal),
			"see above",
			"see above",
			"see above",
			"see above",
		)
		s.logger.Printf("================================================================================")
	}

	return &retrievalResult{fused: fused}, nil
}

func (s *QueryService) Query(ctx context.Context, userQuery string) (*query.QueryResult, error) {
	start := time.Now()

	res, err := s.retrieve(ctx, userQuery)
	if err != nil {
		return nil, fmt.Errorf("query service: retrieve: %w", err)
	}

	req := llm.BuildRequest(userQuery, res.fused, s.cfg.LLM.Model, s.cfg.LLM.Options, s.cfg.LLM.MaxChunkLength)

	if s.logger != nil {
		s.logger.Printf("[LLM BUILD] chunks=%d model=%s max_chunk_length=%d", len(res.fused), s.cfg.LLM.Model, s.cfg.LLM.MaxChunkLength)
	}

	tokensChan, err := s.llm.Generate(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("query service: llm generate: %w", err)
	}

	result := query.BuildQueryResult(userQuery, tokensChan, res.fused)

	if s.logger != nil {
		s.logger.Printf("[QUERY END] duration=%s query=%q", time.Since(start), userQuery)
	}

	return result, nil
}

func (s *QueryService) QueryStream(ctx context.Context, userQuery string) (<-chan llm.GenerateToken, error) {
	start := time.Now()

	res, err := s.retrieve(ctx, userQuery)
	if err != nil {
		return nil, fmt.Errorf("query service: retrieve: %w", err)
	}

	req := llm.BuildRequest(userQuery, res.fused, s.cfg.LLM.Model, s.cfg.LLM.Options, s.cfg.LLM.MaxChunkLength)
	req.Stream = true

	if s.logger != nil {
		s.logger.Printf("[STREAM START] query=%q chunks=%d", userQuery, len(res.fused))
	}

	tokenCh, err := s.llm.Generate(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("query service: llm generate: %w", err)
	}

	if s.logger != nil {
		s.logger.Printf("[STREAM READY] duration=%s query=%q", time.Since(start), userQuery)
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

// En query.go, función nueva:
func deduplicateDensePool(results []search.SearchResult, maxPerSection int) []search.SearchResult {
	counts := make(map[string]int)
	deduped := make([]search.SearchResult, 0, len(results))
	for _, r := range results {
		key := strings.Join(r.Chunk.SectionPath, "|")
		if counts[key] < maxPerSection {
			deduped = append(deduped, r)
			counts[key]++
		}
	}
	return deduped
}
