package nttypeset

import (
	"context"
	"strings"

	"github.com/alecthomas/chroma/v2"
)

// maxLines bounds a Lines document.
const maxLines = 8192

// Lines builds a Lines document from text: one Line block of size 25 per
// line, tabs as four spaces, an empty line as one space. With a language the
// text is highlighted as a whole first, with style, so multi-line tokens keep
// their color on every line; without one each line is one plain span. A
// trailing newline does not make a last empty line. More than 8,192 lines is
// a *LimitError. A cancelled ctx does not fail Lines: as with Highlight, it
// disables highlighting and the document comes back with plain spans.
func Lines(ctx context.Context, text, language string, style *chroma.Style) (*Document, error) {
	text = strings.TrimSuffix(text, "\n")
	tooMany := &LimitError{What: "typeset text exceeds 8,192 lines", Limit: maxLines}
	if strings.Count(text, "\n")+1 > maxLines {
		return nil, tooMany
	}
	doc := &Document{Lines: true}
	// One block per line bounds shaping and preserves blank lines and indentation.
	// Highlight the complete source first so multiline tokens retain their color.
	spans := []Span{{Text: text}}
	if language != "" {
		spans = Highlight(ctx, text, language, style)
	}
	b := Block{Kind: KindLine, Size: 25}
	for _, run := range spans {
		for i, line := range strings.Split(run.Text, "\n") {
			if i > 0 {
				if len(b.Spans) == 0 {
					b.Spans = []Span{{Text: " "}}
				}
				doc.Blocks = append(doc.Blocks, b)
				b = Block{Kind: KindLine, Size: 25}
			}
			part := run
			part.Text = strings.ReplaceAll(line, "\t", "    ")
			if part.Text != "" {
				b.Spans = append(b.Spans, part)
			}
		}
	}
	if len(b.Spans) == 0 {
		b.Spans = []Span{{Text: " "}}
	}
	doc.Blocks = append(doc.Blocks, b)
	if len(doc.Blocks) > maxLines {
		return nil, tooMany
	}
	return doc, nil
}
