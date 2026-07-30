package embed

import (
	"time"
)

// Vector es un vector de embeddings en float32.
// float32 en lugar de float64 porque Qdrant usa float32 y
// reducir a la mitad el tamaño en bbolt es significativo.
type Vector []float32

// CacheEntry es lo que se almacena en bbolt para cada embedding cacheado.
// La clave en bbolt es Chunk.Hash (sha256 del texto).
type CacheEntry struct {
	Hash      string    `json:"hash"`
	Vector    Vector    `json:"vector"`
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
}
