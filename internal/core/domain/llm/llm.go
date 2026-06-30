package llm

import "rag_golang/internal/core/domain/search"

// LLMModel identifica el modelo de lenguaje usado para generación.
type LLMModel string

const (
	LLMQwen25_3B          LLMModel = "qwen2.5:3b"
	LLMQwen25_7B_Instruct LLMModel = "qwen2.5:7b-instruct"
	LLMLlama32_3B         LLMModel = "llama3.2:3b"
	LLMPhi3Mini           LLMModel = "phi3:mini"
	// Modelo por defecto del sistema
	LLMDefaultModel = LLMQwen25_3B
)

type LLMTemperature float32

const (
	Deterministic LLMTemperature = 0.0
	Factual       LLMTemperature = 0.3 // Ideal para RAG (respuestas basadas en hechos)
	Balanced      LLMTemperature = 0.7 // Respuestas generales, un poco más fluidas
	Creative      LLMTemperature = 1.0
)

type LLMNumPredict int

const (
	PredictShort  LLMNumPredict = 256  // Respuestas rápidas o resúmenes ejecutivos
	PredictMedium LLMNumPredict = 512  // Estándar para la mayoría de respuestas RAG
	PredictLong   LLMNumPredict = 1024 // Para respuestas extensas o reportes
)

type LLMNumCtx int

const (
	CtxSmall  LLMNumCtx = 2048  // Para modelos ligeros o prompts cortos
	CtxMedium LLMNumCtx = 4096  // Estándar balanceado para la mayoría de LLMs locales
	CtxLarge  LLMNumCtx = 8192  // Para manejar muchos chunks de contexto
	CtxXLarge LLMNumCtx = 16384 // Para ventanas de contexto masivas
)

// LLMOptions controla los parámetros de generación del LLM.
type LLMOptions struct {
	Temperature LLMTemperature `json:"temperature" yaml:"temperature"`  // 0.0 = determinístico, 1.0 = creativo
	NumPredict  LLMNumPredict  `json:"num_predict"  yaml:"num_predict"` // max tokens a generar
	NumCtx      LLMNumCtx      `json:"num_ctx"      yaml:"num_ctx"`     // context window size
}

// GenerateRequest es la solicitud de generación al LLM.
// Context contiene los chunks recuperados que se inyectan en el prompt.
type GenerateRequest struct {
	Model    LLMModel              `json:"model"`
	Messages []Message             `json:"messages"`
	Context  []search.SearchResult `json:"context,omitempty"` // chunks inyectados como contexto RAG
	Options  LLMOptions            `json:"options"`
	Stream   bool                  `json:"stream"`
}

// GenerateToken es un token de la respuesta del LLM en modo streaming.
// Done == true indica que la generación terminó.
type GenerateToken struct {
	Text string `json:"text"`
	Done bool   `json:"done"`
}
