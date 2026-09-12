package service

import (
	"context"
	"fmt"
	"rag_golang/internal/configs"
	"rag_golang/internal/core/domain/llm"
	"rag_golang/internal/core/domain/query"
	"rag_golang/internal/core/domain/search"
	"rag_golang/internal/core/ports/out"
	"sort"
	"strings"
)

type QueryService struct {
	embedder   out.IEmbedderPort
	vectorRepo out.IVectorRepository
	bm25Repo   out.IBM25Repository
	llm        out.ILLMPort
	cfg        configs.Config
}

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
	// 1) Embedding
	queryToEmbed := s.cfg.Embed.QueryPrefix + userQuery

	vecs, err := s.embedder.Embed(ctx, []string{queryToEmbed})
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("no vectors returned")
	}
	// 2) Dense search
	candidatesK := s.cfg.Search.CandidatesK

	denseResults, err := s.vectorRepo.Search(ctx, search.SearchRequest{
		Vector:         vecs[0],
		TopK:           candidatesK,
		ScoreThreshold: scoreThresholdPtr(s.cfg.Search.DenseScoreThreshold),
	})
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	denseResults = deduplicateDensePool(denseResults, s.cfg.Search.MaxDensePerSection)

	// 3) BM25
	sparseResults, err := s.bm25Repo.Search(ctx, search.BM25SearchRequest{
		Query: userQuery,
		TopK:  candidatesK,
	})
	if err != nil {
		return nil, fmt.Errorf("bm25 search: %w", err)
	}

	// 4) RRF
	rrfPoolSize := s.cfg.Search.TopK * 4
	if rrfPoolSize < s.cfg.Search.TopK {
		rrfPoolSize = s.cfg.Search.TopK
	}

	fused := search.Rrf(denseResults, sparseResults, s.cfg.Search.RRFK, rrfPoolSize)

	if s.cfg.Search.MaxChunksPerSection > 0 {
		fused = limitChunksPerSection(fused, s.cfg.Search.MaxChunksPerSection)
	}

	fused = groupSectionChunksTogether(fused)

	if len(fused) > s.cfg.Search.TopK {
		fused = fused[:s.cfg.Search.TopK]
	}

	return &retrievalResult{fused: fused}, nil
}

func scoreThresholdPtr(f float32) *float32 {
	return &f
}

func (s *QueryService) Query(ctx context.Context, userQuery string) (*query.QueryResult, error) {
	res, err := s.retrieve(ctx, userQuery)
	if err != nil {
		return nil, fmt.Errorf("query service: retrieve: %w", err)
	}

	req := llm.BuildRequest(userQuery, res.fused, s.cfg.LLM.Model, s.cfg.LLM.Options, s.cfg.LLM.MaxChunkLength)

	tokensChan, err := s.llm.Generate(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("query service: llm generate: %w", err)
	}

	result := query.BuildQueryResult(userQuery, tokensChan, res.fused)

	return result, nil
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

func groupSectionChunksTogether(results []search.SearchResult) []search.SearchResult {
	type sectionGroup struct {
		chunks []search.SearchResult
	}
	seen := make(map[string]*sectionGroup)
	order := make([]string, 0)

	for _, r := range results {
		key := strings.Join(r.Chunk.SectionPath, "|")
		if _, ok := seen[key]; !ok {
			seen[key] = &sectionGroup{}
			order = append(order, key)
		}
		seen[key].chunks = append(seen[key].chunks, r)
	}

	// Dentro de cada sección, ordenar por posición en el documento
	for _, g := range seen {
		sort.Slice(g.chunks, func(i, j int) bool {
			return g.chunks[i].Chunk.ChunkIndex < g.chunks[j].Chunk.ChunkIndex
		})
	}

	out := make([]search.SearchResult, 0, len(results))
	for _, key := range order {
		out = append(out, seen[key].chunks...)
	}
	return out
}
