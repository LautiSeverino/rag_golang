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

	bm25Repo := bm25repo.NewRepository()

	extractorDispatcher := extractor.NewExtractorDispatcher(cfg.Extract)

	// EnsureCollection es idempotente: si ya existe, no hace nada.
	if err := vectorRepo.EnsureCollection(ctx, cfg.Store.VectorDimension); err != nil {
		log.Fatalf("qdrant ensure collection: %v", err)
	}

	// ─── Core: services ───────────────────────────────────────────────────────
	indexSvc := service.NewIndexService(
		extractorDispatcher,
		embedder,
		vectorRepo,
		cacheRepo,
		bm25Repo,
		cfg.Chunk,
		cfg.Store.CollectionName,
	)

	querySvc := service.NewQueryService(
		embedder,
		vectorRepo,
		bm25Repo,
		llmClient,
		cfg,
	)

	// ─── Driver: HTTP handlers ────────────────────────────────────────────────
	router := mux.NewRouter()

	router.Use(middlewares.Logging)
	router.Use(middlewares.Recover)

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
		WriteTimeout: 120 * time.Second,
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

func setDefaults(cfg *configs.Config) {
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Search.RRFK == 0 {
		cfg.Search.RRFK = 60
	}
	if cfg.Search.TopK == 0 {
		cfg.Search.TopK = 5
	}
	if cfg.Chunk.MaxSize == 0 {
		cfg.Chunk.MaxSize = 1000
	}
	if cfg.Store.VectorDimension == 0 {
		cfg.Store.VectorDimension = 768
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
}
