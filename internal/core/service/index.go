package service

import (
	"context"
	"rag_golang/internal/core/domain/chunk"
	"rag_golang/internal/core/domain/index"
	"rag_golang/internal/core/ports/out"
)

// IndexService implementa el puerto de entrada IIndexPort.
// Orquesta todos los puertos de salida para indexar un documento.
type IndexService struct {
	extractor  out.IExtractorPort
	embedder   out.IEmbedderPort
	vectorRepo out.IVectorRepository
	cacheRepo  out.IEmbedCacheRepository
	bm25Repo   out.IBM25Repository
	chunker    *chunk.Chunker
	collection string
	cfg        chunk.ChunkConfig
}

func NewIndexService(
	extractor out.IExtractorPort,
	embedder out.IEmbedderPort,
	vectorRepo out.IVectorRepository,
	cacheRepo out.IEmbedCacheRepository,
	bm25Repo out.IBM25Repository,
	cfg chunk.ChunkConfig,
	collection string,
) *IndexService {
	return &IndexService{
		extractor:  extractor,
		embedder:   embedder,
		vectorRepo: vectorRepo,
		cacheRepo:  cacheRepo,
		bm25Repo:   bm25Repo,
		chunker:    chunk.NewChunker(),
		collection: collection,
		cfg:        cfg,
	}
}

// index es la función principal del servicio: recibe un path, extrae el documento,
// lo chunkifica, obtiene los embeddings (con cache) y luego indexa en Qdrant y BM25.
func (s *IndexService) Index(ctx context.Context, sourcePath string) (*index.IndexResult, error) {
	// 1. Extrae el Document (go-fitz o JSON pre-procesado por Python)
	doc, err := s.extractor.Extract(sourcePath)
	if err != nil {
		return nil, err
	}

	// 2. Chunking — llamada directa, no a través de un port
	chunks, err := s.chunker.Chunk(doc, s.cfg)
	if err != nil {
		return nil, err
	}

	// 3. Embeddings con cache
	var reqs []index.IndexRequest
	cacheHits := 0
	for _, chunk := range chunks {
		if vec, ok := s.cacheRepo.Get(chunk.Hash); ok {
			reqs = append(reqs, index.IndexRequest{Chunk: chunk, Vector: vec, CollectionName: s.collection})
			cacheHits++
			continue
		}
		vecs, err := s.embedder.Embed(ctx, []string{chunk.Text})
		if err != nil {
			return nil, err
		}
		_ = s.cacheRepo.Set(chunk.Hash, vecs[0], s.embedder.ModelName())
		reqs = append(reqs, index.IndexRequest{Chunk: chunk, Vector: vecs[0], CollectionName: s.collection})
	}

	// 4. Upsert en Qdrant
	if err := s.vectorRepo.Upsert(ctx, reqs); err != nil {
		return nil, err
	}

	// 5. Indexar en BM25
	if err := s.bm25Repo.Index(ctx, chunks); err != nil {
		return nil, err
	}

	return &index.IndexResult{
		DocID:      doc.ID,
		Source:     sourcePath,
		ChunkCount: len(chunks),
		CacheHits:  cacheHits,
	}, nil
}
