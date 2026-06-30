package in

import (
	"context"
	"rag_golang/internal/core/domain/index"
)

// IIndexPort es el puerto de entrada para indexar documentos.
// Lo implementa IndexService. Lo llaman el HTTP handler y el CLI.
type IIndexPort interface {
	Index(ctx context.Context, sourcePath string) (*index.IndexResult, error)
}
