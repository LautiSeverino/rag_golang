package in

import (
	"context"
	"rag_golang/internal/core/domain/llm"
	"rag_golang/internal/core/domain/query"
)

// IQueryPort es el puerto de entrada para consultas al RAG.
// Lo implementa QueryService. Lo llaman el HTTP handler y el CLI.
type IQueryPort interface {
	Query(ctx context.Context, query string) (*query.QueryResult, error)
	QueryStream(ctx context.Context, query string) (<-chan llm.GenerateToken, error)
}
