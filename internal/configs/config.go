package configs

import (
	"rag_golang/internal/core/domain/chunk"
	"rag_golang/internal/core/domain/embed"
	"rag_golang/internal/core/domain/llm"
)

// Config es la configuración raíz del sistema.
type Config struct {
	Server  ServerConfig      `yaml:"server"`
	Extract ExtractConfig     `yaml:"extract"`
	Chunk   chunk.ChunkConfig `yaml:"chunk"`
	Embed   EmbedConfig       `yaml:"embed"`
	Store   StoreConfig       `yaml:"store"`
	LLM     LLMConfig         `yaml:"llm"`
	Search  SearchConfig      `yaml:"search"`
}

// ExtractConfig configura el comportamiento del Extractor.
type ExtractConfig struct {
	// ProcessedDir es donde el sistema busca JSON pre-procesados.
	// Si data/processed/<name>.json existe, se usa en lugar de re-extraer.
	// Esto permite insertar el output del script Python para PDFs con tablas.
	ProcessedDir string `yaml:"processed_dir"`

	// CacheDir es donde se guardan los Document{} serializados como JSON
	// para evitar re-extraer documentos ya procesados por go-fitz.
	CacheDir string `yaml:"cache_dir"`
}

// EmbedConfig configura el Embedder (Ollama).
type EmbedConfig struct {
	Model          embed.EmbedModel `yaml:"model"`
	OllamaURL      string           `yaml:"ollama_url"`
	BatchSize      int              `yaml:"batch_size"` // chunks por request a Ollama
	QueryPrefix    string           `yaml:"query_prefix"`
	DocumentPrefix string           `yaml:"document_prefix"`
}

// StoreConfig configura el VectorStore (Qdrant) y el EmbedCache (bbolt).
type StoreConfig struct {
	QdrantHost      string `yaml:"qdrant_host"`
	QdrantPort      int    `yaml:"qdrant_port"`
	CollectionName  string `yaml:"collection_name"`
	VectorDimension int    `yaml:"vector_dimension"` // 768 para nomic-embed-text
	BboltPath       string `yaml:"bbolt_path"`
	BM25Path        string `yaml:"bm25_path"` // path del índice BM25
}

// LLMConfig configura el LLM (Ollama).
type LLMConfig struct {
	Model          llm.LLMModel   `yaml:"model"`
	OllamaURL      string         `yaml:"ollama_url"`
	Options        llm.LLMOptions `yaml:"options"`
	MaxChunkLength int            `yaml:"max_chunk_length"`
}

// SearchConfig parametriza la búsqueda híbrida en QueryService.
type SearchConfig struct {
	RRFK                int     `yaml:"rrf_k"`                  // constante RRF fusion, default 60
	TopK                int     `yaml:"top_k"`                  // resultados finales que va al contexto del LLM
	CandidatesK         int     `yaml:"candidates_k"`           // candidatos pre-RRF por store
	BM25K1              float64 `yaml:"bm25_k1"`                // parámetro saturación TF
	BM25B               float64 `yaml:"bm25_b"`                 // parámetro normalización longitud
	MaxChunksPerSection int     `yaml:"max_chunks_per_section"` // máximo de chunks por sección en el contexto final

}

// ServerConfig
type ServerConfig struct {
	Port int `yaml:"port"`
}
