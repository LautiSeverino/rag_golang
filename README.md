# rag_golang

> Sistema RAG (Retrieval-Augmented Generation) local construido en Go con arquitectura hexagonal, búsqueda híbrida densa + esparsa con fusión RRF, y soporte para documentos Markdown y HTML.

---

## Descripción

rag_golang es un sistema RAG local que permite indexar documentos Markdown y HTML para consultarlos en lenguaje natural. Usa embeddings semánticos para búsqueda densa, BM25 para búsqueda esparsa, y fusiona ambos resultados con Reciprocal Rank Fusion (RRF) antes de pasarlos como contexto a un LLM que genera la respuesta final.

Todo el stack corre localmente —sin dependencias de APIs externas de pago— usando Ollama para embeddings y generación de texto, y Qdrant como base de datos vectorial.

La arquitectura es hexagonal (ports and adapters): el dominio no conoce la infraestructura, los servicios solo conocen interfaces, y cada adaptador de infraestructura puede reemplazarse sin tocar el núcleo del sistema.

---

## Features

- **Búsqueda híbrida**: combina similitud vectorial (Qdrant) con BM25 esparso mediante Reciprocal Rank Fusion
- **Chunking estructural**: agrupa elementos por sección (respetando la jerarquía de headings) y subdivide secciones largas por elemento, preservando la coherencia semántica
- **Caché de embeddings**: los vectores se cachean en bbolt indexados por sha256 del texto, eliminando llamadas redundantes a Ollama en re-indexaciones
- **Formatos soportados**: Markdown via parser propio, HTML via `x/net/html`
- **Filtro de TOC**: entradas de índice en Markdown (headings con dot leaders y número de página) se descartan automáticamente en la extracción
- **API REST**: endpoints de indexación y consulta con soporte de streaming SSE para respuesta en tiempo real
- **BM25 persistente**: el índice esparso se serializa a disco en formato gob y se carga al iniciar
- **Arquitectura hexagonal**: domain, ports, service e infra claramente separados con dependencias unidireccionales
- **100% local**: Ollama + Qdrant, sin APIs externas de pago, sin envío de datos a terceros

---

## Stack técnico

| Capa | Tecnología |
|---|---|
| Lenguaje | Go 1.25+ |
| LLM | Ollama (`qwen2.5:3b`) |
| Embeddings | Ollama (`mxbai-embed-large`, 1024 dimensiones) |
| Base de datos vectorial | Qdrant (gRPC) |
| Caché de embeddings | bbolt (BoltDB embedded) |
| Búsqueda esparsa | BM25 in-memory con persistencia gob (implementación propia) |
| Extracción HTML | `golang.org/x/net/html` |
| HTTP router | `gorilla/mux` |

---

## Arquitectura

El sistema sigue arquitectura hexagonal (ports and adapters):

```
┌─────────────────────────────────────────────────────────┐
│                      CORE DOMAIN                         │
│                                                          │
│  ┌───────────┐   ┌─────────────┐   ┌─────────────────┐  │
│  │ ports/in  │   │   service   │   │   ports/out     │  │
│  │           │   │             │   │                 │  │
│  │ IIndexPort│──▶│ IndexService│──▶│ IExtractorPort  │  │
│  │ IQueryPort│──▶│ QueryService│──▶│ IEmbedderPort   │  │
│  │           │   │             │   │ IVectorRepository│  │
│  │           │   │  Chunker    │   │ IEmbedCacheRepo │  │
│  │           │   │  RRF fusion │   │ IBM25Repository │  │
│  │           │   │             │   │ ILLMPort        │  │
│  └───────────┘   └─────────────┘   └─────────────────┘  │
└──────────┬──────────────────────────────────┬────────────┘
           │                                  │
   ┌───────▼──────┐                  ┌────────▼──────────┐
   │    DRIVER    │                  │      DRIVEN       │
   │  (inbound)   │                  │    (outbound)     │
   │              │                  │                   │
   │ HTTP Handler │                  │ Qdrant repo       │
   │ Middlewares  │                  │ bbolt cache       │
   │              │                  │ BM25 repo         │
   └──────────────┘                  │ Ollama embedder   │
                                     │ Ollama LLM        │
                                     │ MD extractor      │
                                     │ HTML extractor    │
                                     └───────────────────┘
```

Las dependencias siempre apuntan hacia el dominio. El dominio no importa nada de infra.

---

## Pipeline de indexación

```
Archivo (.md / .html)
        │
        ▼
  ExtractorDispatcher
  ├── MarkdownExtractor  → parser propio → headings / párrafos / tablas / listas
  │                        filtra entradas de TOC (dot leaders + página)
  └── HTMLExtractor      → x/net/html  → elementos semánticos
        │
        ▼
  Document{ []Element{ Type · Level · Text · Cells · Page · SectionPath } }
        │
        ▼
  Chunker (estrategia: section)
  ├── Tablas: chunk atómico (nunca se parten)
  ├── Headings: límite de sección
  └── Secciones largas: subdivisión por elemento
        │
        ▼
  Embedder (Ollama / mxbai-embed-large)
  Prefix de documento: "search_document: <texto>"
  └── Cache bbolt: sha256(texto) → vector, evita re-embeder en reindexaciones
        │
        ▼
  ┌─────────────┐
  │   Qdrant    │  ← búsqueda densa (similitud coseno, 1024 dims)
  │  (vectores) │
  └─────────────┘
  ┌─────────────┐
  │    BM25     │  ← búsqueda esparsa (term matching)
  │  (in-mem)   │
  └─────────────┘
```

## Pipeline de consulta

```
Query del usuario
        │
        ▼
  Embed query (Ollama)
  Prefix: "search_query: <query>"
        │
        ├──────────────────────────┐
        ▼                          ▼
  Qdrant Search (TopK=50)    BM25 Search (TopK=50)
  score_threshold=0.70       sparse results
  dense results
  dedup por sección (max 5)
        │                          │
        └──────────┬───────────────┘
                   ▼
         RRF Fusion (k=60)
         score = Σ 1/(k + rank)
                   │
                   ▼
         limitChunksPerSection (max 3 por sección)
         groupSectionChunksTogether (agrupa por sección)
                   │
                   ▼
         Top 8 chunks fusionados
                   │
                   ▼
         LLM (Ollama / qwen2.5:3b)
         con contexto RAG inyectado
                   │
                   ▼
         Respuesta con citas de fuentes
```

---

## Estructura del proyecto

```
rag-go/
├── cmd/
│   └── server/
│       └── main.go               # Wiring: instancia todos los componentes y levanta el servidor
│
├── internal/
│   ├── configs/
│   │   ├── config.go             # Structs de configuración (Config, SearchConfig, etc.)
│   │   └── config.yaml           # Configuración activa del sistema
│   │
│   └── core/
│       ├── domain/               # Tipos del dominio y lógica pura
│       │   ├── chunk/            # Chunk, ChunkConfig, Chunker
│       │   ├── embed/            # Vector, CacheEntry
│       │   ├── index/            # IndexRequest, IndexResult
│       │   ├── search/           # SearchRequest, SearchResult, BM25*, HybridSearchRequest, RRF
│       │   ├── llm/              # GenerateRequest, GenerateToken, BuildRequest
│       │   └── query/            # QueryResult, BuildQueryResult
│       │   └── element.go        # ElementType, Element
│       │   └── source.go         # Source (para citas en la respuesta)
│       │   └── document.go       # Document, DocumentMetadata, DocType
│       │
│       ├── ports/
│       │   ├── in/
│       │   │   ├── index.go      # IIndexPort
│       │   │   └── query.go      # IQueryPort
│       │   └── out/
│       │       ├── vector.go     # IVectorRepository
│       │       ├── cache.go      # IEmbedCacheRepository
│       │       ├── bm25.go       # IBM25Repository
│       │       ├── extractor.go  # IExtractorPort
│       │       ├── embedder.go   # IEmbedderPort
│       │       └── llm.go        # ILLMPort
│       │
│       └── service/
│           ├── index.go          # IndexService (implementa IIndexPort)
│           └── query.go          # QueryService (implementa IQueryPort)
│
├── internal/infra/
│   ├── driven/
│   │   ├── clients/
│   │   │   └── ollama/
│   │   │       ├── embedder.go   # OllamaEmbedder → IEmbedderPort
│   │   │       └── llm.go        # OllamaLLM → ILLMPort (streaming NDJSON)
│   │   ├── repositories/
│   │   │   ├── qdrant/
│   │   │   │   ├── qdrant.go     # QdrantRepository → IVectorRepository
│   │   │   │   └── mapping.go    # Conversión chunk ↔ payload Qdrant
│   │   │   ├── cache.go          # BboltCacheRepository → IEmbedCacheRepository
│   │   │   └── bm25.go           # BM25Repository → IBM25Repository
│   │   └── extractor/
│   │       ├── dispatcher.go     # ExtractorDispatcher (elige extractor por extensión)
│   │       ├── md.go             # MarkdownExtractor (parser propio, filtra TOC)
│   │       ├── html.go           # HTMLExtractor (x/net/html)
│   │       ├── postprocess.go    # attachSectionPath, isPageNumberHeading, isStructuralHeading
│   │       └── helper.go         # generateID, fileChecksum, nonEmpty
│   │
│   └── driver/
│       └── http/
│           ├── handler/
│           │   ├── index.go      # POST /api/v1/index
│           │   └── query.go      # POST /api/v1/query · GET /api/v1/query/stream
│           └── middlewares/
│               └── middlewares.go  # Recover (panic → 500)
│
├── data/
│   ├── embeddings.db   # Caché de vectores (bbolt)
│   └── bm25.gob        # Índice BM25 serializado (se carga al iniciar)
│
├── docs/               # Documentos de prueba (.md)
├── tests/              # Archivos .http para probar los endpoints
└── go.mod
└── go.sum
└── README.md
```

---

## Prerrequisitos

### Sistema
- Go 1.25+
- Docker (para Qdrant)
- Ollama instalado y corriendo

### Modelos Ollama

```bash
ollama pull mxbai-embed-large   # embeddings (1024 dimensiones)
ollama pull qwen2.5:3b          # LLM generativo
```

### Qdrant

```bash
docker run -d --name qdrant \
  -p 6333:6333 \
  -p 6334:6334 \
  -v $(pwd)/qdrant_storage:/qdrant/storage \
  qdrant/qdrant
```

Puerto 6333: API REST de Qdrant. Puerto 6334: gRPC (el que usa este proyecto).

---

## Instalación

```bash
git clone https://github.com/LautiSeverino/rag-go
cd rag-go
go mod download
```

---

## Configuración

El archivo `internal/configs/config.yaml` controla todos los parámetros del sistema:

```yaml
server:
  port: 8080

chunk:
  strategy: "section"        # agrupa elementos por sección de heading
  max_size: 1000             # caracteres máximos por chunk
  overlap: 80                # solo aplica con strategy: sliding
  context_prefix: true       # antepone SectionPath al texto embebible

embed:
  model: "mxbai-embed-large:latest"
  ollama_url: "http://localhost:11434"
  batch_size: 8
  query_prefix: "search_query: "
  document_prefix: "search_document: "

store:
  qdrant_host: "localhost"
  qdrant_port: 6334          # puerto gRPC
  collection_name: "rag_docs"
  vector_dimension: 1024     # mxbai-embed-large produce vectores de 1024
  bbolt_path: "data/embeddings.db"
  bm25_path: "data/bm25.gob"

llm:
  model: "qwen2.5:3b"
  ollama_url: "http://localhost:11434"
  max_chunk_length: 600
  options:
    temperature: 0.1
    num_predict: 200
    num_ctx: 3072

search:
  rrf_k: 60                      # constante RRF (paper original: 60)
  top_k: 8                       # chunks que llegan al LLM como contexto
  candidates_k: 50               # candidatos pre-RRF por store
  bm25_k1: 1.2
  bm25_b: 0.75
  max_chunks_per_section: 3      # máximo de chunks por sección en el contexto final
  max_dense_per_section: 5       # dedup pre-RRF en el pool denso
  dense_score_threshold: 0.70    # score coseno mínimo para considerar un chunk denso
```

El sistema crea automáticamente el directorio `data/` al iniciar.

---

## Uso

### Iniciar el servidor

```bash
go run cmd/server/main.go
```

### Indexar un documento

```bash
curl -X POST http://localhost:8080/api/v1/index \
  -H "Content-Type: application/json" \
  -d '{"path": "docs/manual.md"}'
```

Respuesta:

```json
{
  "doc_id": "b5db8234-6f4b-...",
  "source": "docs/manual.md",
  "chunk_count": 24,
  "cache_hits": 0
}
```

En una segunda indexación del mismo archivo, `cache_hits` será igual a `chunk_count` — los vectores se reutilizan de bbolt sin llamar a Ollama.

### Consultar

```bash
curl -X POST http://localhost:8080/api/v1/query \
  -H "Content-Type: application/json" \
  -d '{"query": "¿Cuáles son los elementos de juego?"}'
```

Respuesta:

```json
{
  "query": "¿Cuáles son los elementos de juego?",
  "answer": "Los elementos de juego son: un mapa dividido en 50 países, fichas (100 por color), 6 dados, 50 tarjetas de países, 15 tarjetas de objetivos secretos y un reglamento [Chunk #1].",
  "sources": [
    {
      "file": "docs/teg.md",
      "page": 1,
      "section_path": ["2. ELEMENTOS DE JUEGO"],
      "element_type": "paragraph",
      "score": 0.0481,
      "excerpt": "Un planisferio dividido en 50 países agrupados en 6 continentes..."
    }
  ]
}
```

### Consulta con streaming (SSE)

```bash
curl -N "http://localhost:8080/api/v1/query/stream?q=¿cuáles+son+los+elementos+de+juego?"
```

El servidor emite Server-Sent Events mientras el LLM genera la respuesta, token a token.

### Health check

```bash
curl http://localhost:8080/health
# {"status":"ok"}
```

---

## Formatos de documento soportados

| Formato | Extractor | Headings | Tablas | Listas | Filtro TOC |
|---|---|---|---|---|---|
| `.md` | parser propio | ✓ `##` headings | ✓ pipes `\|` | ✓ `- / 1.` | ✓ dot leaders |
| `.html` / `.htm` | `x/net/html` | ✓ `<h1>...<h6>` | ✓ `<table>` | ✓ `<ul>/<ol>` | — |

---

## Estrategia de chunking

**`section`** (única estrategia activa): agrupa todos los elementos bajo el mismo heading como un chunk. Las secciones que superan `max_size` se subdividen por elemento. Las tablas son siempre atómicas (nunca se parten). Es la estrategia que mejor preserva la coherencia semántica para retrieval.

---

## Notas de diseño

**Prefijos de embedding (mxbai-embed-large)**: el modelo `mxbai-embed-large` está entrenado para usar prefijos diferenciados según el rol del texto. Los documentos se indexan con `"search_document: "` y las queries se embeben con `"search_query: "`. Omitir estos prefijos degrada la calidad de recuperación.

**Por qué BM25 in-memory con persistencia**: para un RAG local con cientos de documentos, el índice BM25 cabe perfectamente en RAM. Se persiste a disco en formato gob (`data/bm25.gob`) y se recarga al arrancar el servidor, evitando re-indexar. Para producción con miles de documentos, el repositorio BM25 puede reemplazarse sin cambiar una línea del servicio (el port `IBM25Repository` abstrae la implementación).

**Por qué RRF puro (sin ponderar por score)**: RRF combina rankings por posición, no por score. El score de BM25 y el score de similitud coseno de Qdrant no son comparables en magnitud ni escala, así que ponderarlos directamente introduciría bias. RRF solo usa el rank (posición en el ranking) que sí es comparable. El parámetro `k=60` estabiliza los scores cuando los dos rankings divergen mucho.

**Dense score threshold**: el parámetro `dense_score_threshold: 0.70` actúa como gate antes del RRF, descartando del pool denso chunks con similitud coseno demasiado baja. Esto reduce ruido en el lado vectorial sin afectar el pool BM25.

**Filtro de TOC en Markdown**: los documentos con índice al inicio (entradas tipo `## 2. ELEMENTOS . . . . . . 2`) se detectan via regex y se descartan en la extracción. De lo contrario contaminarían el índice con chunks de baja calidad que parecen secciones reales.

---

## Dependencias principales

```
github.com/gorilla/mux          # HTTP router
github.com/qdrant/go-client     # cliente Qdrant gRPC
github.com/google/uuid          # UUIDs determinísticos por sha1
go.etcd.io/bbolt                # caché de embeddings (BoltDB)
golang.org/x/net/html           # parser HTML semántico
gopkg.in/yaml.v2                # lectura de config.yaml
```

---

## Licencia

MIT
