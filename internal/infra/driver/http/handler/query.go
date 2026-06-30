package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"

	"rag_golang/internal/core/ports/in"
)

type QueryHandler struct {
	service in.IQueryPort
}

func NewQueryHandler(service in.IQueryPort) *QueryHandler {
	return &QueryHandler{service: service}
}

// queryRequest es el body esperado en POST /api/v1/query.
type queryRequest struct {
	Query string `json:"query"`
}

// Query procesa una consulta al sistema RAG.
// POST /api/v1/query
// Body:    {"query": "¿cuáles son los requisitos del sistema?"}
// Success: 200 + QueryResult como JSON
// Errors:  400 si falta la query, 500 si falla el pipeline
func (h *QueryHandler) Query() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req queryRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, "body inválido: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.Query == "" {
			writeError(w, "el campo 'query' es obligatorio", http.StatusBadRequest)
			return
		}

		result, err := h.service.Query(r.Context(), req.Query)
		if err != nil {
			writeError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSON(w, result, http.StatusOK)
	}
}

// QueryStream procesa una consulta con respuesta en streaming (SSE).
// GET /api/v1/query/stream?q=...
// El cliente recibe tokens del LLM en tiempo real via Server-Sent Events.
func (h *QueryHandler) QueryStream() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		if query == "" {
			writeError(w, "el parámetro 'q' es obligatorio", http.StatusBadRequest)
			return
		}

		// Configurar SSE
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		flusher, ok := w.(http.Flusher)
		if !ok {
			writeError(w, "streaming no soportado", http.StatusInternalServerError)
			return
		}

		tokenCh, err := h.service.QueryStream(r.Context(), query)
		if err != nil {
			writeError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		for token := range tokenCh {
			data, _ := json.Marshal(token)
			// Formato SSE: "data: <payload>\n\n"
			w.Write([]byte("data: " + string(data) + "\n\n"))
			flusher.Flush()

			if token.Done {
				break
			}
		}
	}
}

func (h *QueryHandler) RegisterPublicRoutes(router *mux.Router) {
	router.HandleFunc("/api/v1/query", h.Query()).Methods(http.MethodPost)
	router.HandleFunc("/api/v1/query/stream", h.QueryStream()).Methods(http.MethodGet)
}
