package qdrantrepo

import (
	"context"
	"fmt"
	"strings"

	qdrant "github.com/qdrant/go-client/qdrant"

	"rag_golang/internal/core/domain/index"
	"rag_golang/internal/core/domain/search"
	"rag_golang/internal/core/ports/out"
)

// QdrantRepository implementa out.IVectorRepository usando el cliente alto de Qdrant.
type QdrantRepository struct {
	client     *qdrant.Client
	collection string
}

func NewVectorRepository(host string, port int, collection string) (*QdrantRepository, error) {
	client, err := qdrant.NewClient(&qdrant.Config{
		Host: host,
		Port: port,
	})
	if err != nil {
		return nil, fmt.Errorf("qdrant: connect to %s:%d: %w", host, port, err)
	}

	return &QdrantRepository{
		client:     client,
		collection: collection,
	}, nil
}

// EnsureCollection crea la colección si no existe.
func (r *QdrantRepository) EnsureCollection(ctx context.Context, dimension int) error {
	err := r.client.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: r.collection,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     uint64(dimension),
			Distance: qdrant.Distance_Cosine,
		}),
	})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "already exists") {
			return nil
		}
		return fmt.Errorf("qdrant: ensure collection %q: %w", r.collection, err)
	}
	return nil
}

// Upsert inserta o actualiza puntos en batch.
func (r *QdrantRepository) Upsert(ctx context.Context, reqs []index.IndexRequest) error {
	if len(reqs) == 0 {
		return nil
	}

	points := make([]*qdrant.PointStruct, 0, len(reqs))
	for _, req := range reqs {
		points = append(points, chunkToPoint(req))
	}

	_, err := r.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: r.collection,
		Points:         points,
	})
	if err != nil {
		return fmt.Errorf("qdrant: upsert %d points: %w", len(points), err)
	}

	return nil
}

// Search ejecuta la búsqueda semántica.
func (r *QdrantRepository) Search(ctx context.Context, req search.SearchRequest) ([]search.SearchResult, error) {
	limit := uint64(req.TopK)

	queryReq := &qdrant.QueryPoints{
		CollectionName: r.collection,
		Query:          qdrant.NewQuery(req.Vector...),
		Limit:          &limit,
		WithPayload:    qdrant.NewWithPayload(true),
	}

	if req.Filter != nil {
		queryReq.Filter = buildFilter(req.Filter)
	}

	if req.ScoreThreshold != nil {
		queryReq.ScoreThreshold = req.ScoreThreshold
	}

	resp, err := r.client.Query(ctx, queryReq)
	if err != nil {
		return nil, fmt.Errorf("qdrant: query: %w", err)
	}

	results := make([]search.SearchResult, 0, len(resp))
	for _, hit := range resp {
		results = append(results, search.SearchResult{
			Chunk: pointToChunk(hit),
			Score: hit.Score,
		})
	}
	return results, nil
}

// Delete elimina todos los puntos de un documento por doc_id.
func (r *QdrantRepository) Delete(ctx context.Context, docID string) error {
	_, err := r.client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: r.collection,
		Points: qdrant.NewPointsSelectorFilter(&qdrant.Filter{
			Must: []*qdrant.Condition{
				qdrant.NewMatch("doc_id", docID),
			},
		}),
	})
	if err != nil {
		return fmt.Errorf("qdrant: delete doc %q: %w", docID, err)
	}
	return nil
}

// Ensure interface compatibility at compile time.
var _ out.IVectorRepository = (*QdrantRepository)(nil)

// No-op helper if you still want a close-like hook at call site.
// The high-level client wrapper exposed in the README does not require a manual Close here.
func (r *QdrantRepository) Client() *qdrant.Client {
	return r.client
}
