# rag_golang

> Sistema RAG (Retrieval-Augmented Generation) local construido en Go con arquitectura hexagonal, búsqueda híbrida densa + esparsa con fusión RRF, y soporte para múltiples formatos de documento.

---

## Descripción

rag_golang es un sistema RAG de producción local que permite indexar documentos (PDF, DOCX, HTML, Markdown) y consultarlos en lenguaje natural. Usa embeddings semánticos para búsqueda densa, BM25 para búsqueda esparsa, y fusiona ambos resultados con Reciprocal Rank Fusion (RRF) antes de pasarlos como contexto a un LLM que genera la respuesta final.

Todo el stack corre localmente —sin dependencias de APIs externas de pago— usando Ollama para embeddings y generación de texto, y Qdrant como base de datos vectorial.

La arquitectura es hexagonal (ports and adapters): el dominio no conoce la infraestructura, los servicios solo conocen interfaces, y cada adaptador de infraestructura puede ser reemplazado sin tocar el núcleo del sistema.

---

## Features

- **Búsqueda híbrida**: combina similitud vectorial (Qdrant) con BM25 esparso mediante Reciprocal Rank Fusion
- **Chunking estructural**: detecta headings por análisis de font-size y chunka por sección en lugar de por caracteres, preservando la coherencia semántica del documento
- **Caché de embeddings**: los vectores se cachean en bbolt indexados por sha256 del texto, eliminando llamadas redundantes a Ollama en re-indexaciones
- **Múltiples formatos**: PDF y DOCX via go-fitz (motor MuPDF), HTML via x/net/html, Markdown via parser propio
- **Preprocesamiento Python** (opcional): script para PDFs con tablas complejas que usa PyMuPDF `find_tables()` y produce JSON compatible directamente con el pipeline Go
- **API REST**: endpoints de indexación y consulta con soporte de streaming SSE para respuesta en tiempo real
- **Arquitectura hexagonal**: domain, ports, service e infra claramente separados con dependencias unidireccionales
- **100% local**: Ollama + Qdrant, sin APIs externas de pago, sin envío de datos a terceros

---

## Stack técnico

| Capa | Tecnología |
|---|---|
| Lenguaje | Go 1.22+ |
| LLM + Embeddings | Ollama (`phi3:mini - qwen2.5:3b - etc.`, `nomic-embed-text`) |
| Base de datos vectorial | Qdrant (gRPC) |
| Caché de embeddings | bbolt (BoltDB embedded) |
| Búsqueda esparsa | BM25 in-memory (implementación propia) |
| Extracción PDF/DOCX | go-fitz (wrapper CGO del motor MuPDF) |
| Extracción HTML | golang.org/x/net/html |
| HTTP router | gorilla/mux |
| Preprocesamiento Python | PyMuPDF (opcional, para tablas complejas) |

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
                                     │ Fitz extractor    │
                                     └───────────────────┘
```

Las dependencias siempre apuntan hacia el dominio. El dominio no importa nada de infra.

---

## Pipeline de indexación

```
Archivo (PDF / DOCX / HTML / .md)
        │
        ▼
  ExtractorDispatcher
  ├── FitzExtractor   → go-fitz HTML() + análisis font-size → headings/párrafos/listas
  ├── HTMLExtractor   → x/net/html → elementos semánticos
  └── MarkdownExtractor → parser propio → ##headings / tablas / listas
        │
        ▼
  Document{ []Element{ Type · Level · Text · Cells · Page · SectionPath } }
  [se cachea como JSON en data/cache/]
        │
        ▼
  Chunker (estrategia: section / element / sliding)
  ├── Tablas: chunk atómico (nunca se parten)
  ├── Headings: límite de sección
  └── Secciones largas: subdivisión por elemento
        │
        ▼
  Embedder (Ollama / nomic-embed-text)
  └── Cache bbolt: sha256(texto) → vector, evita re-embeder en reindexaciones
        │
        ▼
  ┌─────────────┐
  │   Qdrant    │  ← búsqueda densa (similitud coseno)
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
        │
        ├──────────────────────────┐
        ▼                          ▼
  Qdrant Search (TopK=20)    BM25 Search (TopK=20)
  dense results              sparse results
        │                          │
        └──────────┬───────────────┘
                   ▼
         RRF Fusion (k=60)
         score = Σ 1/(k + rank)
                   │
                   ▼
         Top K chunks fusionados
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
│       └── main.go               # Wiring: crea todos los componentes y levanta el servidor
│
├── internal/
│   ├── configs/
│   │   └── config.go             # Structs de configuración (Config, ServerConfig, etc.)
|   |   └── config.yaml           # Configuración del sistema
│   │
│   └── core/
│       ├── domain/               # Tipos del dominio y lógica pura
│       │   ├── chunk/            # Chunk, ChunkConfig, ChunkStrategy, Chunker
│       │   ├── embed/            # Vector, EmbedModel, CacheEntry
│       │   ├── index/            # IndexRequest, IndexResult
│       │   ├── search/           # SearchRequest, SearchResult, BM25*, HybridSearchRequest, RRF
│       │   ├── llm/              # GenerateRequest, GenerateToken, LLMModel, BuildRequest
│       │   └── query/            # QueryResult, BuildQueryResult, Source
│       │   └── element.go
│       │   └── source.go
│       │   └── document.go       # Document, Element, ElementType, DocType
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
├── infra/
│   ├── driven/
│   │   ├── clients/
│   │   │   └── ollama/
│   │   │       ├── embedder.go   # OllamaEmbedder → IEmbedderPort
│   │   │       └── llm.go        # OllamaLLM → ILLMPort
│   │   ├── repositories/
│   │   │   ├── qdrant/
│   │   │   │   ├── vector_repository.go  # QdrantRepository → IVectorRepository
│   │   │   │   └── mapping.go            # Conversión chunk ↔ payload Qdrant
│   │   │   ├── bbolt/
│   │   │   │   └── cache_repository.go  # BboltCacheRepository → IEmbedCacheRepository
│   │   │   └── bm25/
│   │   │       └── bm25_repository.go   # BM25Repository → IBM25Repository
│   │   └── extractor/                   # FitzExtractor + MarkdownExtractor + HTMLExtractor + Dispatcher
│   │       └── cache.go.go
|   |       └── dispatcher.go
|   |       └── helper.go
|   |       └── html.go
|   |       └── md.go
|   |       └── pdf_fitz.go
|   |       └── postprocess.go
│   │
│   └── driver/
│       └── http/
│           ├── handler/
│           │   ├── index.go  # POST /api/v1/index
│           │   └── query.go  # POST /api/v1/query · GET /api/v1/query/stream
│           └── middlewares/
│               └── middlewares.go    # Logging + Recover (panic → 500)
│
├── scripts/
│   └── preprocess.py   # Preprocesamiento Python para PDFs con tablas complejas
│
├── data/
│   ├── embeddings.db   # Documentos a indexar
│   ├── processed/      # JSON pre-procesados por preprocess.py (tablas complejas)
│   └── cache/          # JSON cacheados por el extractor Go (evita re-extraer)
│
├── docs/               # Documentos de prueba
└── go.mod
└── go.sum
└── README.md
└── .env
└── test.http
```

---

## Prerrequisitos

### Sistema
- Go 1.22+
- Docker (para Qdrant)
- Ollama instalado y corriendo

### Modelos Ollama

```bash
ollama pull nomic-embed-text   # embeddings (768 dimensiones)
ollama pull qwen2.5:3b-instruct  # LLM generativo
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

### Python (opcional, solo para PDFs con tablas complejas)

```bash
pip install pymupdf
```

---

## Instalación

```bash
git clone https://github.com/LautiSeverino/rag-go
cd rag-go
go mod download
```

---

## Configuración

Editá `config.yaml` en la raíz del proyecto:

```yaml
server:
  port: 8080

extract:
  processed_dir: "data/processed"   # JSON pre-procesados por Python
  cache_dir: "data/cache"           # JSON cacheados por go-fitz

chunk:
  strategy: "section"               # section | element | sliding
  max_size: 1000                    # caracteres máximos por chunk
  overlap: 80                       # solo aplica con strategy: sliding
  context_prefix: true              # antepone SectionPath al texto embebible

embed:
  model: "nomic-embed-text"
  ollama_url: "http://localhost:11434"
  batch_size: 8

store:
  qdrant_host: "localhost"
  qdrant_port: 6334                 # puerto gRPC
  collection_name: "rag_docs"
  vector_dimension: 768             # dimensión de nomic-embed-text
  bbolt_path: "data/embeddings.db"

llm:
  model: "qwen2.5:3b-instruct"
  ollama_url: "http://localhost:11434"
  options:
    temperature: 0.1
    num_predict: 512
    num_ctx: 4096

search:
  rrf_k: 60                         # constante RRF (paper original: 60)
  top_k: 5                          # chunks que llegan al LLM como contexto
```

El sistema crea automáticamente los directorios `data/` necesarios al iniciar.

---

## Uso

### Iniciar el servidor

```bash
go run cmd/server/main.go
# 2026/06/30 12:00:00 servidor RAG escuchando en :8080
```

### Indexar un documento

```bash
curl -X POST http://localhost:8080/api/v1/index \
  -H "Content-Type: application/json" \
  -d '{"path": "docs/manual.pdf"}'
```

Respuesta:

```json
{
  "doc_id": "b5db8234-6f4b",
  "source": "docs/manual.pdf",
  "chunk_count": 87,
  "cache_hits": 0,
  "duration_ns": 0
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
  "answer": "Los elementos de juego son: un mapa (planisferio dividido en 50 países agrupados en 6 continentes), fichas (cada una representa 1 ejército, 100 por color), 6 dados, un mazo de 50 tarjetas de países, 15 tarjetas de objetivos secretos y un reglamento [Chunk #1].",
  "sources": [
    {
      "file": "docs/teg.pdf",
      "page": 2,
      "section_path": ["2. ELEMENTOS DE JUEGO"],
      "element_type": "paragraph",
      "score": 0.048,
      "excerpt": "Un planisferio dividido en 50 países agrupados en 6 continentes..."
    }
  ],
  "duration_ns": 0
}
```

### Consulta con streaming (SSE)

Para recibir tokens del LLM en tiempo real, usá el endpoint de streaming:

```bash
curl -N "http://localhost:8080/api/v1/query/stream?q=¿cuáles+son+los+elementos+de+juego?"
```

El servidor emite Server-Sent Events mientras el LLM genera la respuesta. Útil para integrar en un frontend tipo chat.

### Health check

```bash
curl http://localhost:8080/health
# {"status":"ok"}
```

---

## Preprocesamiento Python para tablas

go-fitz (MuPDF) extrae texto con la misma calidad que PyMuPDF porque es el mismo motor C. La única diferencia real es la detección de tablas: PyMuPDF tiene `page.find_tables()` que analiza coordenadas para detectar celdas y filas, incluso en tablas sin bordes.

Para PDFs donde las tablas contienen información crítica (reportes financieros, especificaciones técnicas), ejecutá el script de preprocesamiento antes de indexar:

```bash
python scripts/preprocess.py data/pdfs/reporte.pdf
# Procesando data/pdfs/reporte.pdf...
#   247 elementos extraídos
#   18 headings, 12 tablas
#   → data/processed/reporte.json
```

El JSON producido sigue exactamente el mismo schema que `domain.Document` en Go. Al indexar, el `FitzExtractor` lo detecta automáticamente y lo usa en lugar de re-extraer con go-fitz:

```
POST /api/v1/index {"path": "data/pdfs/reporte.pdf"}
→ FitzExtractor busca data/processed/reporte.json
→ Si existe: lo usa directamente (con tablas estructuradas)
→ Si no existe: extrae con go-fitz (sin detección de tablas)
```

Para documentos sin tablas complejas (libros, manuales, reglamentos), go-fitz es completamente suficiente y no hace falta el script Python.

---

## Formatos de documento soportados

| Formato | Extractor | Headings | Tablas | Listas |
|---|---|---|---|---|
| `.pdf` | go-fitz (MuPDF) | ✓ por font-size | ✗ texto plano¹ | ✓ por prefijo |
| `.docx` | go-fitz (MuPDF) | ✓ por font-size | ✗ texto plano¹ | ✓ por prefijo |
| `.html` | x/net/html | ✓ `<h1>...<h6>` | ✓ `<table>` | ✓ `<ul>/<ol>` |
| `.md` | parser propio | ✓ `##` headings | ✓ pipes `\|` | ✓ `- / 1.` |

¹ Para tablas en PDF/DOCX con estructura crítica, usar `preprocess.py` que produce JSON con `cells[][]` correctamente parseados.

---

## Estrategias de chunking

El sistema soporta tres estrategias configurables en `config.yaml`:

**`section`** (recomendada): agrupa todos los elementos bajo el mismo heading como un chunk. Las secciones que superan `max_size` se subdividen por elemento. Las tablas son siempre atómicas (nunca se parten). Es la estrategia que mejor preserva la coherencia semántica para retrieval.

**`element`**: un chunk por cada element del documento. Útil para documentos muy estructurados donde cada párrafo es autónomo.

**`sliding`**: ventana deslizante de caracteres con overlap. Fallback para documentos sin estructura detectable (texto plano, PDFs escaneados).

---

## Variables de entorno y defaults

Si un campo no está en `config.yaml`, el sistema aplica estos defaults:

| Campo | Default |
|---|---|
| `server.port` | `8080` |
| `search.rrf_k` | `60` |
| `search.top_k` | `5` |
| `chunk.max_size` | `1000` |
| `store.vector_dimension` | `768` |
| `extract.processed_dir` | `data/processed` |
| `extract.cache_dir` | `data/cache` |
| `llm.options.temperature` | `0.1` |
| `llm.options.num_predict` | `512` |
| `llm.options.num_ctx` | `4096` |

---

## Dependencias principales

```
github.com/gen2brain/go-fitz          # extracción PDF/DOCX (wrapper MuPDF)
github.com/gorilla/mux                # HTTP router
github.com/qdrant/go-client           # cliente Qdrant gRPC
go.etcd.io/bbolt                      # caché de embeddings (BoltDB)
golang.org/x/net/html                 # parser HTML semántico
gopkg.in/yaml.v3                      # lectura de config.yaml
```

---

## Notas de diseño

**Por qué go-fitz y no una librería Go pura**: go-fitz es un wrapper CGO del motor MuPDF en C, el mismo motor que usa PyMuPDF. La calidad de extracción de texto y el manejo de layouts multi-columna es idéntica a Python. Las librerías PDF puras en Go (ledongthuc/pdf, dslipak/pdf) son significativamente inferiores para documentos complejos.

**Por qué BM25 in-memory**: para un RAG local con cientos de documentos, el índice BM25 cabe perfectamente en RAM. El tradeoff es que el índice se pierde al reiniciar — hay que re-indexar. Para producción con miles de documentos, el repositorio BM25 se puede reemplazar por una implementación que persiste el índice en disco sin cambiar una línea del servicio (el port `IBM25Repository` abstrae la implementación).

**Por qué RRF puro (sin ponderar por score)**: RRF combina rankings por posición, no por score. El score de BM25 y el score de similitud coseno de Qdrant no son comparables en magnitud ni escala, así que ponderarlos directamente introduciría bias hacia uno de los dos. RRF solo usa el rank (posición en el ranking) que sí es comparable. El parámetro `k=60` estabiliza los scores cuando los dos rankings divergen mucho.

---

## Licencia

MIT
