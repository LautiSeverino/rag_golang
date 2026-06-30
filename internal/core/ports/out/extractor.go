package out

import "rag_golang/internal/core/domain"

// IExtractorPort es el puerto de salida para extracción de documentos.
// Implementación concreta: go-fitz (PDF/DOCX), x/net/html (HTML), stdlib (MD).
//
// Es un puerto de CÓMPUTO, no un repository: convierte un archivo en un Document
// pero no persiste nada. El resultado se cachea fuera de este port (en disco como JSON).
//
// CanHandle permite al dispatcher elegir el extractor correcto por extensión.
type IExtractorPort interface {
	Extract(path string) (*domain.Document, error)
	CanHandle(path string) bool
}
