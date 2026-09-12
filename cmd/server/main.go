package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"rag_golang/internal/configs"
	"rag_golang/internal/core/service"
	"rag_golang/internal/infra/driven/clients/ollama"
	"rag_golang/internal/infra/driven/extractor"
	"rag_golang/internal/infra/driven/repositories"
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
		os.Exit(1)
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
		os.Exit(1)
	}

	cacheRepo, err := repositories.NewCacheRepository(cfg.Store.BboltPath)
	if err != nil {
		os.Exit(1)
	}
	defer cacheRepo.Close()

	bm25Repo := repositories.NewBM25Repository(cfg.Search.BM25K1, cfg.Search.BM25B)
	if cfg.Store.BM25Path != "" {
		_ = bm25Repo.LoadFromDisk(cfg.Store.BM25Path)
	}

	extractorDispatcher := extractor.NewExtractorDispatcher()

	// EnsureCollection es idempotente: si ya existe, no hace nada.
	if err := vectorRepo.EnsureCollection(ctx, cfg.Store.VectorDimension); err != nil {
		os.Exit(1)
	}

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
	)

	// ─── Driver: HTTP handlers ────────────────────────────────────────────────
	router := mux.NewRouter()

	router.Use(middlewares.Recover())

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

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		os.Exit(1)
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
	return cfg, nil
}
