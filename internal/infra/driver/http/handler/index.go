package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"

	"rag_golang/internal/core/ports/in"
)

type IndexHandler struct {
	service in.IIndexPort
}

func NewIndexHandler(service in.IIndexPort) *IndexHandler {
	return &IndexHandler{service: service}
}

// indexRequest es el body esperado en POST /api/v1/index.
type indexRequest struct {
	Path string `json:"path"`
}

// errorResponse es la estructura de error uniforme de la API.
type errorResponse struct {
	Error string `json:"error"`
}

// Index procesa la solicitud de indexación de un documento.
// POST /api/v1/index
// Body:    {"path": "data/pdfs/manual.pdf"}
// Success: 200 + IndexResult como JSON
// Errors:  400 si falta el path, 500 si falla el pipeline
func (h *IndexHandler) Index() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req indexRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, "body inválido: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.Path == "" {
			writeError(w, "el campo 'path' es obligatorio", http.StatusBadRequest)
			return
		}

		result, err := h.service.Index(r.Context(), req.Path)
		if err != nil {
			writeError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSON(w, result, http.StatusOK)
	}
}

func (h *IndexHandler) RegisterPublicRoutes(router *mux.Router) {
	router.HandleFunc("/api/v1/index", h.Index()).Methods(http.MethodPost)
}

// ─── helpers compartidos ──────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, v any, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, msg string, status int) {
	writeJSON(w, errorResponse{Error: msg}, status)
}
