package extractor

import (
	"crypto/sha256"
	"fmt"
	"os"

	"github.com/google/uuid"
)

// nonEmpty — devuelve un slice de strings sin elementos vacíos.
func nonEmpty(ss []string) []string {
	result := make([]string, 0, len(ss))
	for _, s := range ss {
		if s != "" {
			result = append(result, s)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// generateID — genera un UUID determinístico basado en el path del archivo y su checksum.
func generateID(path string) uuid.UUID {
	// UUID determinístico basado en el path + checksum.
	// Así el mismo archivo genera siempre el mismo ID.
	seed := path + "|" + fileChecksum(path)
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(seed))
}

// fileChecksum — calcula el checksum SHA256 del archivo en la ruta dada.
func fileChecksum(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		// Fallback: si no se puede leer, no cachear
		return ""
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum)
}
