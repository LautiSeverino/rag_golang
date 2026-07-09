package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"rag_golang/internal/configs"
	"rag_golang/internal/core/domain/llm"
	"rag_golang/internal/core/service"
	"rag_golang/internal/infra/driven/clients/ollama"
	"rag_golang/internal/infra/driven/extractor"
	bboltrepo "rag_golang/internal/infra/driven/repositories/bbolt"
	bm25repo "rag_golang/internal/infra/driven/repositories/bm25"
	qdrantrepo "rag_golang/internal/infra/driven/repositories/qdrant"
	"rag_golang/internal/infra/driver/http/handler"
	"rag_golang/internal/infra/driver/http/middlewares"
	"time"

	"github.com/gorilla/mux"
	"gopkg.in/yaml.v2"
)

func main() {
	// ─── Config ───────────────────────────────────────────────────────────────
	cfg, err := loadConfig("internal/configs/config.yaml")
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx := context.Background()

	// ─── Infra: driven adapters ───────────────────────────────────────────────

	embedder := ollama.NewEmbedder(
		cfg.Embed.OllamaURL,
		cfg.Embed.Model,
		cfg.Store.VectorDimension,
	)

	llmClient := ollama.NewLLM(cfg.LLM.OllamaURL)

	vectorRepo, err := qdrantrepo.NewVectorRepository(
		cfg.Store.QdrantHost,
		cfg.Store.QdrantPort,
		cfg.Store.CollectionName,
	)
	if err != nil {
		log.Fatalf("qdrant: %v", err)
	}

	cacheRepo, err := bboltrepo.NewCacheRepository(cfg.Store.BboltPath)
	if err != nil {
		log.Fatalf("bbolt: %v", err)
	}
	defer cacheRepo.Close()

	bm25Repo := bm25repo.NewRepository(cfg.Search.BM25K1, cfg.Search.BM25B)
	if cfg.Store.BM25Path != "" {
		if err := bm25Repo.LoadFromDisk(cfg.Store.BM25Path); err != nil {
			log.Printf("warning: no se pudo cargar BM25 desde disco: %v", err)
		} else {
			log.Printf("BM25 cargado desde %s", cfg.Store.BM25Path)
		}
	}

	extractorDispatcher := extractor.NewExtractorDispatcher(cfg.Extract)

	// EnsureCollection es idempotente: si ya existe, no hace nada.
	if err := vectorRepo.EnsureCollection(ctx, cfg.Store.VectorDimension); err != nil {
		log.Fatalf("qdrant ensure collection: %v", err)
	}

	// ─── Infra: driver adapters ───────────────────────────────────────────────

	if err := os.MkdirAll("logs", 0755); err != nil {
		log.Fatalf("crear carpeta logs: %v", err)
	}

	logFile, err := os.OpenFile(
		cfg.Log.FilePath,
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0644,
	)
	if err != nil {
		log.Fatalf("abrir log file: %v", err)
	}
	defer logFile.Close()

	httpLogger := log.New(logFile, "", log.LstdFlags|log.Lmicroseconds)
	ragLogger := log.New(logFile, "", log.LstdFlags|log.Lmicroseconds)

	// ─── Core: services ───────────────────────────────────────────────────────
	indexSvc := service.NewIndexService(
		extractorDispatcher,
		embedder,
		vectorRepo,
		cacheRepo,
		bm25Repo,
		cfg.Chunk,
		cfg.Embed,
		cfg.Store.CollectionName,
		cfg.Store.BM25Path,
	)

	querySvc := service.NewQueryService(
		embedder,
		vectorRepo,
		bm25Repo,
		llmClient,
		cfg,
		ragLogger,
	)

	// ─── Driver: HTTP handlers ────────────────────────────────────────────────
	router := mux.NewRouter()

	router.Use(middlewares.Logging(
		httpLogger,
		cfg.Log.LogRequests,
		cfg.Log.LogResponses,
		cfg.Log.MaxBodyBytes,
	))
	router.Use(middlewares.Recover(httpLogger))

	handler.NewIndexHandler(indexSvc).RegisterPublicRoutes(router)
	handler.NewQueryHandler(querySvc).RegisterPublicRoutes(router)

	router.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	}).Methods(http.MethodGet)

	// ─── Servidor ─────────────────────────────────────────────────────────────
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 10 * time.Minute,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("servidor RAG escuchando en %s", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("servidor: %v", err)
	}
}

func loadConfig(path string) (configs.Config, error) {
	var cfg configs.Config
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("leer config %q: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parsear config: %w", err)
	}
	setDefaults(&cfg)
	return cfg, nil
}

// cmd/server/main.go

func setDefaults(cfg *configs.Config) {
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Search.RRFK == 0 {
		cfg.Search.RRFK = 60
	}
	if cfg.Search.TopK == 0 {
		cfg.Search.TopK = 8
	}
	// NUEVO: default para candidates_k
	if cfg.Search.CandidatesK == 0 {
		cfg.Search.CandidatesK = 40
	}
	// NUEVO: parámetros BM25
	if cfg.Search.BM25K1 == 0 {
		cfg.Search.BM25K1 = 1.2
	}
	if cfg.Search.BM25B == 0 {
		cfg.Search.BM25B = 0.75
	}
	// NUEVO: límite de chunk en prompt
	if cfg.LLM.MaxChunkLength == 0 {
		cfg.LLM.MaxChunkLength = 1000
	}
	if cfg.Chunk.MaxSize == 0 {
		cfg.Chunk.MaxSize = 1000
	}
	if cfg.Extract.ProcessedDir == "" {
		cfg.Extract.ProcessedDir = "data/processed"
	}
	if cfg.Extract.CacheDir == "" {
		cfg.Extract.CacheDir = "data/cache"
	}
	if cfg.LLM.Model == "" {
		cfg.LLM.Model = llm.LLMDefaultModel
	}
	if cfg.LLM.Options.Temperature == 0 {
		cfg.LLM.Options.Temperature = llm.Factual
	}
	if cfg.LLM.Options.NumPredict == 0 {
		cfg.LLM.Options.NumPredict = llm.PredictMedium
	}
	if cfg.LLM.Options.NumCtx == 0 {
		cfg.LLM.Options.NumCtx = llm.CtxSmall
	}
	if cfg.Embed.QueryPrefix == "" && cfg.Embed.Model == "nomic-embed-text" {
		cfg.Embed.QueryPrefix = "search_query: "
	}
	if cfg.Embed.DocumentPrefix == "" && cfg.Embed.Model == "nomic-embed-text" {
		cfg.Embed.DocumentPrefix = "search_document: "
	}
	if cfg.Search.MaxChunksPerSection == 0 {
		cfg.Search.MaxChunksPerSection = 2
	}
	if cfg.Search.MaxDensePerSection == 0 {
		cfg.Search.MaxDensePerSection = 5
	}
}
