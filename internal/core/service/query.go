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

func (s *QueryService) Query(ctx context.Context, userQuery string) (*query.QueryResult, error) {
	// 1. Embed la query
	vecs, err := s.embedder.Embed(ctx, []string{userQuery})
	if err != nil {
		return nil, fmt.Errorf("query service: embed: %w", err)
	}

	if len(vecs) == 0 {
		return nil, fmt.Errorf("query service: no vectors returned")
	}

	// 2. Búsqueda densa (Qdrant)
	denseResults, err := s.vectorRepo.Search(ctx, search.SearchRequest{
		Vector: vecs[0], TopK: 20,
	})
	if err != nil {
		return nil, fmt.Errorf("query service: vector repo, search: %w", err)
	}

	// 3. Búsqueda esparsa (BM25)
	sparseResults, err := s.bm25Repo.Search(ctx, search.BM25SearchRequest{
		Query: userQuery, TopK: 20,
	})
	if err != nil {
		return nil, fmt.Errorf("query service: bm25 repo, search: %w", err)
	}

	// 4. RRF fusion
	fused := search.Rrf(denseResults, sparseResults, s.cfg.Search.RRFK, s.cfg.Search.TopK)

	// 5. Generar respuesta (el LLM devuelve un channel de tokens)
	// un channel es un canal de comunicación en Go.
	// En este caso el LLM va enviando tokens uno por uno,
	// y el servicio los consume para armar la respuesta final.
	tokensChan, err := s.llm.Generate(ctx, llm.BuildRequest(userQuery, fused, s.cfg.LLM.Model, s.cfg.LLM.Options))
	if err != nil {
		return nil, fmt.Errorf("query service: llm generate: %w", err)
	}

	// 6. Acumular y devolver
	return query.BuildQueryResult(userQuery, tokensChan, fused), nil
}

func (s *QueryService) QueryStream(ctx context.Context, userQuery string) (<-chan llm.GenerateToken, error) {
	// 1. Embed la query
	vecs, err := s.embedder.Embed(ctx, []string{userQuery})
	if err != nil {
		return nil, fmt.Errorf("query service: embed: %w", err)
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("query service: no vectors returned")
	}

	// 2. Búsqueda densa (Qdrant)
	denseResults, err := s.vectorRepo.Search(ctx, search.SearchRequest{
		Vector: vecs[0], TopK: 20,
	})
	if err != nil {
		return nil, fmt.Errorf("query service: vector repo search: %w", err)
	}

	// 3. Búsqueda esparsa (BM25)
	sparseResults, err := s.bm25Repo.Search(ctx, search.BM25SearchRequest{
		Query: userQuery, TopK: 20,
	})
	if err != nil {
		return nil, fmt.Errorf("query service: bm25 repo search: %w", err)
	}

	// 4. RRF fusion
	fused := search.Rrf(denseResults, sparseResults, s.cfg.Search.RRFK, s.cfg.Search.TopK)

	// 5. Construir el request con Stream=true y devolver el canal sin drenarlo
	req := llm.BuildRequest(userQuery, fused, s.cfg.LLM.Model, s.cfg.LLM.Options)
	req.Stream = true

	tokenCh, err := s.llm.Generate(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("query service: llm generate: %w", err)
	}

	return tokenCh, nil
}
