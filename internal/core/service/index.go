package service

import (
	"context"
	"fmt"
	"rag_golang/internal/configs"
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
	chunkCfg   chunk.ChunkConfig
	embedCfg   configs.EmbedConfig
}

func NewIndexService(
	extractor out.IExtractorPort,
	embedder out.IEmbedderPort,
	vectorRepo out.IVectorRepository,
	cacheRepo out.IEmbedCacheRepository,
	bm25Repo out.IBM25Repository,
	chunkCfg chunk.ChunkConfig,
	embedCfg configs.EmbedConfig,
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
		chunkCfg:   chunkCfg,
		embedCfg:   embedCfg,
	}
}

// index es la función principal del servicio: recibe un path, extrae el documento,
// lo chunkifica, obtiene los embeddings (con cache) y luego indexa en Qdrant y BM25.
func (s *IndexService) Index(ctx context.Context, sourcePath string) (*index.IndexResult, error) {
	doc, err := s.extractor.Extract(sourcePath)
	if err != nil {
		return nil, err
	}

	// Borrar versión anterior del documento en ambos stores antes de re-insertar.
	// Qdrant: Delete filtra por doc_id en el payload — es idempotente si no existe.
	// BM25: DeleteByDocID reconstruye el índice invertido sin ese doc.
	if err := s.vectorRepo.Delete(ctx, doc.ID.String()); err != nil {
		// Loguear pero no fallar: si el doc no existía aún, el error es esperado
		// y no queremos abortar la indexación por eso.
		fmt.Printf("warning: vectorRepo.Delete doc_id=%s: %v\n", doc.ID, err)
	}
	if err := s.bm25Repo.DeleteByDocID(ctx, doc.ID); err != nil {
		return nil, fmt.Errorf("bm25: delete before reindex: %w", err)
	}

	chunks, err := s.chunker.Chunk(doc, s.chunkCfg)
	if err != nil {
		return nil, err
	}

	// 1. Separar chunks con y sin caché
	type pendingChunk struct {
		chunk chunk.Chunk
		idx   int
	}

	reqs := make([]index.IndexRequest, len(chunks))
	var pending []pendingChunk
	cacheHits := 0

	for i, ch := range chunks {
		if vec, ok := s.cacheRepo.Get(ch.Hash, s.embedder.ModelName()); ok {
			reqs[i] = index.IndexRequest{Chunk: ch, Vector: vec, CollectionName: s.collection}
			cacheHits++
		} else {
			pending = append(pending, pendingChunk{chunk: ch, idx: i})
		}
	}

	// 2. Embed en batches
	batchSize := s.embedCfg.BatchSize
	for start := 0; start < len(pending); start += batchSize {
		end := start + batchSize
		if end > len(pending) {
			end = len(pending)
		}
		batch := pending[start:end]

		texts := make([]string, len(batch))
		for j, p := range batch {
			texts[j] = p.chunk.Text
		}

		vecs, err := s.embedder.Embed(ctx, texts)
		if err != nil {
			return nil, fmt.Errorf("embed batch [%d:%d]: %w", start, end, err)
		}

		for j, p := range batch {
			if err := s.cacheRepo.Set(p.chunk.Hash, vecs[j], s.embedder.ModelName()); err != nil {
				return nil, fmt.Errorf("cache set: %w", err)
			}
			reqs[p.idx] = index.IndexRequest{Chunk: p.chunk, Vector: vecs[j], CollectionName: s.collection}
		}
	}

	if err := s.vectorRepo.Upsert(ctx, reqs); err != nil {
		return nil, err
	}
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
