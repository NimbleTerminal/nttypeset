package nttypeset

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2/styles"
)

func linesText(doc *Document) []string {
	var out []string
	for _, b := range doc.Blocks {
		var s strings.Builder
		for _, sp := range b.Spans {
			s.WriteString(sp.Text)
		}
		out = append(out, s.String())
	}
	return out
}

func TestLinesDocument(t *testing.T) {
	ctx := context.Background()
	doc, err := Lines(ctx, "a\tb\n\n  c\n", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !doc.Lines || len(doc.Blocks) != 3 {
		t.Fatalf("doc %+v", doc)
	}
	if got := strings.Join(linesText(doc), "|"); got != "a    b| |  c" {
		t.Fatalf("lines %q", got)
	}
	for _, b := range doc.Blocks {
		if b.Kind != KindLine || b.Size != 25 || len(b.Spans) != 1 {
			t.Fatalf("block %+v", b)
		}
	}
	if doc.Blocks[1].Spans[0] != (Span{Text: " "}) {
		t.Fatalf("empty line %+v", doc.Blocks[1].Spans)
	}
	if doc, err := Lines(ctx, "", "", nil); err != nil || len(doc.Blocks) != 1 || doc.Blocks[0].Spans[0].Text != " " {
		t.Fatalf("empty text: %v", err)
	}

	if _, err := Lines(ctx, strings.Repeat("line\n", 8192), "", nil); err != nil {
		t.Fatal(err)
	}
	_, err = Lines(ctx, strings.Repeat("line\n", 8193), "", nil)
	var le *LimitError
	if !errors.Is(err, ErrLimit) || !errors.As(err, &le) || le.What != "typeset text exceeds 8,192 lines" || le.Limit != 8192 {
		t.Fatalf("8,193 lines: %v", err)
	}

	// A cancelled context disables highlighting and still returns the document.
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	doc, err = Lines(cancelled, "x := 1\n", "go", styles.Get("github"))
	if err != nil || len(doc.Blocks) != 1 || len(doc.Blocks[0].Spans) != 1 || doc.Blocks[0].Spans[0].Token != 0 {
		t.Fatalf("cancelled ctx: %+v, %v", doc, err)
	}

	// A raw string across lines keeps its token, and its color, on each line.
	src := "package main\n\nvar s = `one\ntwo\nthree`\n\nfunc main() {\n\tprintln(s)\n}\n"
	doc, err = Lines(ctx, src, "go", styles.Get("github"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(linesText(doc), "\n"); got != "package main\n \nvar s = `one\ntwo\nthree`\n \nfunc main() {\n    println(s)\n}" {
		t.Fatalf("highlighted text %q", got)
	}
	last := doc.Blocks[2].Spans[len(doc.Blocks[2].Spans)-1]
	mid, end := doc.Blocks[3].Spans[0], doc.Blocks[4].Spans[0]
	if last.Token == 0 || mid.Token != last.Token || end.Token != last.Token || mid.Text != "two" {
		t.Fatalf("multi-line token: %+v %+v %+v", last, mid, end)
	}
	for _, b := range doc.Blocks {
		for _, sp := range b.Spans {
			if strings.TrimSpace(sp.Text) != "" && sp.Style&Mono == 0 {
				t.Fatal("source lost monospace")
			}
		}
	}
}
