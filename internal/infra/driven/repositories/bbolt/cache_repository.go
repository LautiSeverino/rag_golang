package bbolt

import (
	"encoding/json"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"

	"rag_golang/internal/core/domain/embed"
)

var bucketName = []byte("embeddings")

// BboltCacheRepository implementa out.IEmbedCacheRepository usando bbolt.
type BboltCacheRepository struct {
	db *bolt.DB
}

func NewCacheRepository(path string) (*BboltCacheRepository, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("bbolt: open %q: %w", path, err)
	}

	if err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucketName)
		return err
	}); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("bbolt: create bucket: %w", err)
	}

	return &BboltCacheRepository{db: db}, nil
}

// Get recupera un vector del caché por hash.
// Devuelve (vector, true) si existe y el modelo coincide, (nil, false) si no.
func (r *BboltCacheRepository) Get(hash string, model embed.EmbedModel) (embed.Vector, bool) {
	var (
		entry embed.CacheEntry
		found bool
	)

	err := r.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketName)
		if b == nil {
			return nil
		}

		data := b.Get([]byte(hash))
		if data == nil {
			return nil
		}

		if err := json.Unmarshal(data, &entry); err != nil {
			return err
		}

		if entry.Model != model {
			return nil
		}

		found = true
		return nil
	})

	if err != nil || !found {
		return nil, false
	}

	return entry.Vector, true
}

// Set guarda un vector en el caché con su hash como clave.
func (r *BboltCacheRepository) Set(hash string, vec embed.Vector, model embed.EmbedModel) error {
	entry := embed.CacheEntry{
		Hash:      hash,
		Vector:    vec,
		Model:     model,
		CreatedAt: time.Now(),
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("bbolt: marshal entry: %w", err)
	}

	return r.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketName)
		if b == nil {
			return fmt.Errorf("bbolt: bucket %q not found", bucketName)
		}
		if err := b.Put([]byte(hash), data); err != nil {
			return fmt.Errorf("bbolt: put %q: %w", hash, err)
		}
		return nil
	})
}

func (r *BboltCacheRepository) Close() error {
	return r.db.Close()
}

func (r *BboltCacheRepository) Stats() (count int, err error) {
	err = r.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketName)
		if b == nil {
			return nil
		}
		count = b.Stats().KeyN
		return nil
	})
	return
}
