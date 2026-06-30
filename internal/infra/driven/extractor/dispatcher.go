package extractor

import (
	"fmt"
	"path/filepath"
	"rag_golang/internal/configs"
	"rag_golang/internal/core/domain"
	"rag_golang/internal/core/ports/out"
)

// ExtractorDispatcher elige el extractor correcto según la extensión del archivo.
// Es el único punto de entrada que el IndexService necesita conocer.
type ExtractorDispatcher struct {
	extractors []interface {
		CanHandle(string) bool
		Extract(string) (*domain.Document, error)
	}
}

func NewExtractorDispatcher(cfg configs.ExtractConfig) *ExtractorDispatcher {
	return &ExtractorDispatcher{
		extractors: []interface {
			CanHandle(string) bool
			Extract(string) (*domain.Document, error)
		}{
			NewFitzExtractor(cfg),
			NewMarkdownExtractor(),
			NewHTMLExtractor(),
		},
	}
}

// Extract delega al extractor que CanHandle el archivo.
// Implementa out.IExtractorPort.
func (d *ExtractorDispatcher) Extract(path string) (*domain.Document, error) {
	for _, ex := range d.extractors {
		if ex.CanHandle(path) {
			return ex.Extract(path)
		}
	}
	return nil, fmt.Errorf("extractor: no handler for %q", filepath.Ext(path))
}

func (d *ExtractorDispatcher) CanHandle(path string) bool {
	for _, ex := range d.extractors {
		if ex.CanHandle(path) {
			return true
		}
	}
	return false
}

var _ out.IExtractorPort = (*ExtractorDispatcher)(nil)
