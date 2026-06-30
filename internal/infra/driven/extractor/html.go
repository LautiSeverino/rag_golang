package extractor

import (
	"fmt"
	"os"
	"path/filepath"
	"rag_golang/internal/core/domain"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// HTMLExtractor implementa out.IExtractorPort para archivos .html.
type HTMLExtractor struct{}

func NewHTMLExtractor() *HTMLExtractor { return &HTMLExtractor{} }

func (e *HTMLExtractor) CanHandle(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".html" || ext == ".htm"
}

func (e *HTMLExtractor) Extract(path string) (*domain.Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("html extractor: read %q: %w", path, err)
	}

	elements, err := parseHTMLSemantic(string(data))
	if err != nil {
		return nil, fmt.Errorf("html extractor: parse: %w", err)
	}
	elements = attachSectionPath(elements)

	return &domain.Document{
		ID: generateID(path),
		Metadata: domain.DocumentMetadata{
			Source:    path,
			DocType:   domain.DocHTML,
			PageCount: 1,
			Checksum:  fileChecksum(path),
			IndexedAt: time.Now(),
		},
		Elements: elements,
	}, nil
}

func parseHTMLSemantic(content string) ([]domain.Element, error) {
	root, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return nil, err
	}

	var elements []domain.Element
	var tableRows [][]string
	inTable := false

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type != html.ElementNode {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
			return
		}

		switch n.Data {
		case "h1", "h2", "h3", "h4", "h5", "h6":
			level := int(n.Data[1] - '0')
			elements = append(elements, domain.Element{
				Type:  domain.ElemHeading,
				Level: level,
				Text:  strings.TrimSpace(nodeText(n)),
				Page:  1,
			})

		case "p":
			text := strings.TrimSpace(nodeText(n))
			if text != "" {
				elements = append(elements, domain.Element{
					Type: domain.ElemParagraph,
					Text: text,
					Page: 1,
				})
			}

		case "table":
			inTable = true
			tableRows = nil
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
			inTable = false
			if len(tableRows) > 0 {
				elements = append(elements, domain.Element{
					Type:  domain.ElemTable,
					Text:  rowsToMarkdown(tableRows),
					Cells: tableRows,
					Page:  1,
				})
			}
			return // ya procesamos los hijos

		case "tr":
			if inTable {
				var row []string
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					if c.Type == html.ElementNode && (c.Data == "td" || c.Data == "th") {
						row = append(row, strings.TrimSpace(nodeText(c)))
					}
				}
				if len(row) > 0 {
					tableRows = append(tableRows, row)
				}
				return
			}

		case "li":
			text := strings.TrimSpace(nodeText(n))
			if text != "" {
				elements = append(elements, domain.Element{
					Type: domain.ElemListItem,
					Text: text,
					Page: 1,
				})
			}
			return // no procesar hijos recursivamente

		case "script", "style", "nav", "footer", "header":
			return // ignorar
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return elements, nil
}

func nodeText(n *html.Node) string {
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

func rowsToMarkdown(rows [][]string) string {
	if len(rows) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, row := range rows {
		sb.WriteString("| ")
		sb.WriteString(strings.Join(row, " | "))
		sb.WriteString(" |\n")
		if i == 0 {
			sb.WriteString("|")
			for range row {
				sb.WriteString("---|")
			}
			sb.WriteString("\n")
		}
	}
	return sb.String()
}
