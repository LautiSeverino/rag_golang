package service

import (
	"context"
	"fmt"
	"rag_golang/internal/configs"
	"rag_golang/internal/core/domain/llm"
	"rag_golang/internal/core/domain/query"
	"rag_golang/internal/core/domain/search"
	"rag_golang/internal/core/ports/out"
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
	vecs, err := s.embedder.Embed(ctx, []string{userQuery})
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
	if err != nil {
		return nil, fmt.Errorf("bm25 search: %w", err)
	}

	fused := search.Rrf(denseResults, sparseResults, s.cfg.Search.RRFK, s.cfg.Search.TopK)
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
