package extractor

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gen2brain/go-fitz"
	"golang.org/x/net/html"

	"rag_golang/internal/configs"
	"rag_golang/internal/core/domain"
)

// FitzExtractor implementa out.IExtractorPort para PDF y DOCX.
// Pipeline:
//  1. Si existe un JSON pre-procesado (e.g. por el script Python), lo carga.
//  2. Si no, usa go-fitz: extrae HTML por página y detecta estructura por font-size.
type FitzExtractor struct {
	processedDir string // data/processed/
	cacheDir     string // data/cache/
}

func NewFitzExtractor(cfg configs.ExtractConfig) *FitzExtractor {
	return &FitzExtractor{
		processedDir: cfg.ProcessedDir,
		cacheDir:     cfg.CacheDir,
	}
}

func (e *FitzExtractor) CanHandle(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".pdf" || ext == ".docx"
}

// Extract primero busca un JSON pre-procesado en ProcessedDir.
// Si no existe, extrae con go-fitz y guarda el resultado en CacheDir.
func (e *FitzExtractor) Extract(path string) (*domain.Document, error) {
	// 1. Intentar cargar JSON pre-procesado (output del script Python)
	if doc, ok := e.loadPreProcessed(path); ok {
		return doc, nil
	}

	// 2. Intentar cargar desde caché (output de una extracción previa con go-fitz)
	if doc, ok := e.loadCache(path); ok {
		return doc, nil
	}

	// 3. Extraer con go-fitz
	doc, err := e.extractWithFitz(path)
	if err != nil {
		return nil, err
	}

	// 4. Guardar en caché para evitar re-extraer
	_ = e.saveCache(path, doc)

	return doc, nil
}

// ─── go-fitz extraction ───────────────────────────────────────────────────────

func (e *FitzExtractor) extractWithFitz(path string) (*domain.Document, error) {
	doc, err := fitz.New(path)
	if err != nil {
		return nil, fmt.Errorf("fitz: open %q: %w", path, err)
	}
	defer doc.Close()

	numPages := doc.NumPage()
	allBlocks := make([]rawBlock, 0, numPages*20)

	for n := 0; n < numPages; n++ {
		pageHTML, err := doc.HTML(n, true)
		if err != nil {
			return nil, fmt.Errorf("fitz: page %d HTML: %w", n, err)
		}
		blocks, err := parsePageHTML(pageHTML, n+1)
		if err != nil {
			return nil, fmt.Errorf("fitz: parse page %d: %w", n, err)
		}
		allBlocks = append(allBlocks, blocks...)
	}

	elements := classifyBlocks(allBlocks)
	elements = attachSectionPath(elements)
	elements = filterPageHeaders(elements)

	checksum := fileChecksum(path)
	docID := generateID(path)

	return &domain.Document{
		ID: docID,
		Metadata: domain.DocumentMetadata{
			Source:    path,
			DocType:   inferDocType(path),
			PageCount: numPages,
			Checksum:  checksum,
			IndexedAt: time.Now(),
		},
		Elements: elements,
	}, nil
}

// ─── HTML parsing ─────────────────────────────────────────────────────────────

// rawBlock es un bloque de texto extraído del HTML posicional de go-fitz.
type rawBlock struct {
	Text       string
	FontSize   float64
	FontFamily string
	IsBold     bool
	Top        float64
	Page       int
}

var (
	fontSizeRe   = regexp.MustCompile(`font-size:\s*([\d.]+)pt`)
	fontFamilyRe = regexp.MustCompile(`font-family:\s*([^;]+)`)
	topRe        = regexp.MustCompile(`top:\s*([\d.]+)pt`)
)

func parsePageHTML(pageHTML string, pageNum int) ([]rawBlock, error) {
	root, err := html.Parse(strings.NewReader(pageHTML))
	if err != nil {
		return nil, err
	}

	var blocks []rawBlock
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "p" {
			block := extractBlock(n, pageNum)
			if strings.TrimSpace(block.Text) != "" {
				blocks = append(blocks, block)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return blocks, nil
}

func extractBlock(p *html.Node, pageNum int) rawBlock {
	block := rawBlock{Page: pageNum}

	// Extraer top del <p>
	for _, attr := range p.Attr {
		if attr.Key == "style" {
			if m := topRe.FindStringSubmatch(attr.Val); m != nil {
				block.Top, _ = strconv.ParseFloat(m[1], 64)
			}
		}
	}

	// El texto y estilo de fuente vienen del <span> interior
	// Tomamos el span con más texto (el dominante)
	var texts []string
	var dominantSpan *html.Node
	maxLen := 0

	var walkSpans func(*html.Node)
	walkSpans = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "span" {
			text := extractText(n)
			if len(text) > maxLen {
				maxLen = len(text)
				dominantSpan = n
			}
			texts = append(texts, text)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walkSpans(c)
		}
	}
	walkSpans(p)

	block.Text = strings.Join(texts, "")

	if dominantSpan != nil {
		for _, attr := range dominantSpan.Attr {
			if attr.Key == "style" {
				if m := fontSizeRe.FindStringSubmatch(attr.Val); m != nil {
					block.FontSize, _ = strconv.ParseFloat(m[1], 64)
				}
				if m := fontFamilyRe.FindStringSubmatch(attr.Val); m != nil {
					block.FontFamily = strings.TrimSpace(m[1])
				}
			}
		}
	}

	lower := strings.ToLower(block.FontFamily)
	block.IsBold = strings.Contains(lower, "bold") ||
		strings.Contains(lower, "heavy") ||
		strings.Contains(lower, "black")

	return block
}

func extractText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}
