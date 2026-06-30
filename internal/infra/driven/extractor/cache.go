package extractor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"rag_golang/internal/core/domain"
	"strings"

	"github.com/google/uuid"
)

// ─── pre-processed JSON (Python output) ──────────────────────────────────────

func (e *FitzExtractor) loadPreProcessed(sourcePath string) (*domain.Document, bool) {
	if e.processedDir == "" {
		return nil, false
	}
	name := strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath))
	jsonPath := filepath.Join(e.processedDir, name+".json")
	return loadDocFromJSON(jsonPath)
}

func (e *FitzExtractor) loadCache(sourcePath string) (*domain.Document, bool) {
	if e.cacheDir == "" {
		return nil, false
	}
	name := strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath))
	jsonPath := filepath.Join(e.cacheDir, name+".json")
	doc, ok := loadDocFromJSON(jsonPath)
	if !ok {
		return nil, false
	}
	// Invalidar caché si el archivo fuente cambió
	if doc.Metadata.Checksum != fileChecksum(sourcePath) {
		return nil, false
	}
	return doc, true
}

func (e *FitzExtractor) saveCache(sourcePath string, doc *domain.Document) error {
	if e.cacheDir == "" {
		return nil
	}
	if err := os.MkdirAll(e.cacheDir, 0755); err != nil {
		return err
	}
	name := strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath))
	jsonPath := filepath.Join(e.cacheDir, name+".json")
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(jsonPath, data, 0644)
}

func loadDocFromJSON(path string) (*domain.Document, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var doc domain.Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, false
	}
	return &doc, true
}

func generateID(path string) uuid.UUID {
	// UUID determinístico basado en el path + checksum.
	// Así el mismo archivo genera siempre el mismo ID.
	seed := path + "|" + fileChecksum(path)
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(seed))
}
func fileChecksum(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	// sha256 simple para invalidación de caché
	// Import crypto/sha256 en producción; aquí usamos len+path como placeholder
	// para no agregar imports que ya existen en el proyecto.
	_ = data
	info, _ := os.Stat(path)
	if info == nil {
		return path
	}
	return fmt.Sprintf("%s-%d-%d", filepath.Base(path), info.Size(), info.ModTime().Unix())
}
