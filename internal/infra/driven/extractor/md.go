package extractor

import (
	"fmt"
	"os"
	"path/filepath"
	"rag_golang/internal/core/domain"
	"regexp"
	"strings"
	"time"
)

// MarkdownExtractor implementa out.IExtractorPort para archivos .md.
// Es puro stdlib, sin dependencias externas.
type MarkdownExtractor struct{}

func NewMarkdownExtractor() *MarkdownExtractor { return &MarkdownExtractor{} }

func (e *MarkdownExtractor) CanHandle(path string) bool {
	return strings.ToLower(filepath.Ext(path)) == ".md"
}

func (e *MarkdownExtractor) Extract(path string) (*domain.Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("md extractor: read %q: %w", path, err)
	}

	elements := parseMarkdown(string(data))
	elements = attachSectionPath(elements)

	return &domain.Document{
		ID: generateID(path),
		Metadata: domain.DocumentMetadata{
			Source:    path,
			DocType:   domain.DocMarkdown,
			PageCount: 1,
			Checksum:  fileChecksum(path),
			IndexedAt: time.Now(),
		},
		Elements: elements,
	}, nil
}

var (
	mdHeadingRe = regexp.MustCompile(`^(#{1,6})\s+(.+)`)
	mdTableRe   = regexp.MustCompile(`^\|`)
	mdBulletRe  = regexp.MustCompile(`^[-*+]\s+`)
	mdNumRe     = regexp.MustCompile(`^\d+\.\s+`)
	mdCodeRe    = regexp.MustCompile("^```")
)

func parseMarkdown(content string) []domain.Element {
	lines := strings.Split(content, "\n")
	var elements []domain.Element
	var paraBuffer []string
	var tableBuffer []string
	var listBuffer []string
	inCode := false

	flushPara := func() {
		if len(paraBuffer) == 0 {
			return
		}
		text := strings.TrimSpace(strings.Join(paraBuffer, "\n"))
		if text != "" {
			elements = append(elements, domain.Element{Type: domain.ElemParagraph, Text: text, Page: 1})
		}
		paraBuffer = nil
	}
	flushTable := func() {
		if len(tableBuffer) == 0 {
			return
		}
		elements = append(elements, domain.Element{
			Type: domain.ElemTable,
			Text: strings.Join(tableBuffer, "\n"),
			Page: 1,
		})
		tableBuffer = nil
	}
	flushList := func() {
		if len(listBuffer) == 0 {
			return
		}
		elements = append(elements, domain.Element{
			Type: domain.ElemListItem,
			Text: strings.Join(listBuffer, "\n"),
			Page: 1,
		})
		listBuffer = nil
	}

	for _, line := range lines {
		if mdCodeRe.MatchString(line) {
			inCode = !inCode
			if !inCode {
				flushPara()
			}
			continue
		}
		if inCode {
			paraBuffer = append(paraBuffer, line)
			continue
		}

		if m := mdHeadingRe.FindStringSubmatch(line); m != nil {
			flushPara()
			flushTable()
			flushList()
			elements = append(elements, domain.Element{
				Type:  domain.ElemHeading,
				Level: len(m[1]),
				Text:  strings.TrimSpace(m[2]),
				Page:  1,
			})
			continue
		}

		if mdTableRe.MatchString(line) {
			flushPara()
			flushList()
			tableBuffer = append(tableBuffer, line)
			continue
		}
		if len(tableBuffer) > 0 && !mdTableRe.MatchString(line) {
			flushTable()
		}

		if mdBulletRe.MatchString(line) || mdNumRe.MatchString(line) {
			flushPara()
			text := mdBulletRe.ReplaceAllString(line, "")
			text = mdNumRe.ReplaceAllString(text, "")
			listBuffer = append(listBuffer, strings.TrimSpace(text))
			continue
		}
		if len(listBuffer) > 0 && strings.TrimSpace(line) == "" {
			flushList()
			continue
		}
		if len(listBuffer) > 0 {
			listBuffer = append(listBuffer, strings.TrimSpace(line))
			continue
		}

		if strings.TrimSpace(line) == "" {
			flushPara()
			continue
		}
		paraBuffer = append(paraBuffer, line)
	}

	flushPara()
	flushTable()
	flushList()

	return elements
}
