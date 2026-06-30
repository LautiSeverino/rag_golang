package qdrantrepo

import (
	"strings"

	"github.com/google/uuid"
	qdrant "github.com/qdrant/go-client/qdrant"

	"rag_golang/internal/core/domain"
	"rag_golang/internal/core/domain/chunk"
	"rag_golang/internal/core/domain/index"
	"rag_golang/internal/core/domain/search"
)

func chunkToPoint(req index.IndexRequest) *qdrant.PointStruct {
	ch := req.Chunk
	sectionStr := strings.Join(ch.SectionPath, " > ")

	return &qdrant.PointStruct{
		Id: qdrant.NewID(ch.ID.String()),
		Vectors: qdrant.NewVectors(
			req.Vector...,
		),
		Payload: qdrant.NewValueMap(map[string]any{
			"doc_id":       ch.DocID.String(),
			"text":         ch.RawText,
			"element_type": string(ch.ElementType),
			"section_path": sectionStr,
			"page":         int64(ch.Page),
			"chunk_index":  int64(ch.ChunkIndex),
			"source":       ch.Source,
			"hash":         ch.Hash,
		}),
	}
}

func pointToChunk(hit *qdrant.ScoredPoint) chunk.Chunk {
	p := hit.Payload

	docID, _ := uuid.Parse(getString(p, "doc_id"))

	return chunk.Chunk{
		ID:          getUUID(hit.Id),
		DocID:       docID,
		RawText:     getString(p, "text"),
		Text:        getString(p, "text"),
		ElementType: domain.ElementType(getString(p, "element_type")),
		SectionPath: splitSection(getString(p, "section_path")),
		Page:        int(getInt(p, "page")),
		ChunkIndex:  int(getInt(p, "chunk_index")),
		Source:      getString(p, "source"),
		Hash:        getString(p, "hash"),
	}
}

func buildFilter(f *search.SearchFilter) *qdrant.Filter {
	var conditions []*qdrant.Condition

	if f.ElementType != nil {
		conditions = append(conditions, qdrant.NewMatch("element_type", string(*f.ElementType)))
	}
	if f.Source != nil {
		conditions = append(conditions, qdrant.NewMatch("source", *f.Source))
	}

	if len(conditions) == 0 {
		return nil
	}

	return &qdrant.Filter{
		Must: conditions,
	}
}

func getString(p map[string]*qdrant.Value, key string) string {
	if v, ok := p[key]; ok {
		return v.GetStringValue()
	}
	return ""
}

func getInt(p map[string]*qdrant.Value, key string) int64 {
	if v, ok := p[key]; ok {
		return v.GetIntegerValue()
	}
	return 0
}

func getUUID(id *qdrant.PointId) uuid.UUID {
	if id == nil {
		return uuid.Nil
	}
	if s := id.GetUuid(); s != "" {
		u, err := uuid.Parse(s)
		if err == nil {
			return u
		}
	}
	return uuid.Nil
}

func splitSection(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, " > ")
}
