package goldmark_test

import (
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"strings"
	"testing"

	gm "github.com/yuin/goldmark"
	emoji "github.com/yuin/goldmark-emoji"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
	"nimbleterminal.dev/nttypeset"
	"nimbleterminal.dev/nttypeset/goldmark"
)

func parse(t *testing.T, source []byte) ast.Node {
	t.Helper()
	md := gm.New(gm.WithExtensions(extension.GFM, emoji.Emoji))
	return md.Parser().Parse(text.NewReader(source))
}

func landscape(t *testing.T) image.Image {
	t.Helper()
	f, err := os.Open("../testdata/landscape.png")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func spanText(spans []nttypeset.Span) string {
	var b strings.Builder
	for _, s := range spans {
		b.WriteString(s.Text)
	}
	return b.String()
}

func TestBuildKinds(t *testing.T) {
	source, err := os.ReadFile("../testdata/typeset.md")
	if err != nil {
		t.Fatal(err)
	}
	root := parse(t, source)
	still := landscape(t)

	doc, err := goldmark.Build(context.Background(), root, source, goldmark.Options{
		Images: map[string]nttypeset.Image{"landscape.png": {Key: "landscape.png", Still: still}},
	})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[nttypeset.Kind]bool{}
	var images []nttypeset.Block
	for _, b := range doc.Blocks {
		seen[b.Kind] = true
		if b.Kind == nttypeset.KindImage {
			images = append(images, b)
		}
	}
	for _, k := range []nttypeset.Kind{nttypeset.Heading, nttypeset.Paragraph, nttypeset.KindQuote, nttypeset.Code, nttypeset.Table, nttypeset.KindImage} {
		if !seen[k] {
			t.Errorf("no block of kind %d in %+v", k, doc.Blocks)
		}
	}
	if len(images) != 2 {
		t.Fatalf("got %d image blocks, want 2", len(images))
	}
	if im := images[0].Image; im.Key != "landscape.png" || im.Still == nil {
		t.Errorf("landscape image = %+v", im)
	}
	if im := images[1].Image; im.Key != "missing-typeset-image.png" || im.Still != nil {
		t.Errorf("missing image = %+v", im)
	}
	if got := spanText(images[1].Spans); got != "[image unavailable: missing-typeset-image.png]" {
		t.Errorf("missing image spans = %q", got)
	}
	if images[1].Spans[0].Style != nttypeset.Italic {
		t.Errorf("placeholder style = %v, want Italic", images[1].Spans[0].Style)
	}

	// Without Images, both are placeholders.
	doc, err = goldmark.Build(context.Background(), root, source, goldmark.Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range doc.Blocks {
		if b.Kind != nttypeset.KindImage {
			continue
		}
		if b.Image.Still != nil {
			t.Errorf("image %q has a Still without Options.Images", b.Image.Key)
		}
	}
	var got []string
	for _, b := range doc.Blocks {
		if b.Kind == nttypeset.KindImage {
			got = append(got, spanText(b.Spans))
		}
	}
	want := []string{"[image unavailable: landscape.png]", "[image unavailable: missing-typeset-image.png]"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("placeholders = %q, want %q", got, want)
	}
}

func TestBuildListsAndQuotes(t *testing.T) {
	source := []byte("1. one\n2. two\n\n- bullet\n\n> a\n>\n> > b\n")
	doc, err := goldmark.Build(context.Background(), parse(t, source), source, goldmark.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var markers, quoted []nttypeset.Block
	for _, b := range doc.Blocks {
		switch {
		case len(b.Quotes) > 0:
			quoted = append(quoted, b)
		default:
			markers = append(markers, b)
		}
	}
	if len(markers) != 3 {
		t.Fatalf("got %d list blocks: %+v", len(markers), doc.Blocks)
	}
	for i, want := range []string{"1. ", "2. ", "• "} {
		if got := markers[i].Spans[0].Text; got != want {
			t.Errorf("list block %d marker = %q, want %q", i, got, want)
		}
		if markers[i].Indent != 24 {
			t.Errorf("list block %d indent = %d, want 24", i, markers[i].Indent)
		}
	}
	if len(quoted) != 2 {
		t.Fatalf("got %d quoted blocks: %+v", len(quoted), doc.Blocks)
	}
	outer, inner := quoted[0], quoted[1]
	if outer.Kind != nttypeset.KindQuote || inner.Kind != nttypeset.KindQuote {
		t.Errorf("kinds = %d, %d, want KindQuote", outer.Kind, inner.Kind)
	}
	if len(outer.Quotes) != 1 || outer.Quotes[0] != (nttypeset.Quote{Indent: 24, Last: false}) {
		t.Errorf("outer quotes = %+v", outer.Quotes)
	}
	// The inner quote is recorded first, then the outer one.
	wantInner := []nttypeset.Quote{{Indent: 48, Last: true}, {Indent: 24, Last: true}}
	if len(inner.Quotes) != 2 || inner.Quotes[0] != wantInner[0] || inner.Quotes[1] != wantInner[1] {
		t.Errorf("inner quotes = %+v, want %+v", inner.Quotes, wantInner)
	}
}

func TestBuildLimits(t *testing.T) {
	ctx := context.Background()
	check := func(t *testing.T, err error, what string, limit int) {
		t.Helper()
		var le *nttypeset.LimitError
		if !errors.Is(err, nttypeset.ErrLimit) || !errors.As(err, &le) {
			t.Fatalf("err = %v, want a LimitError", err)
		}
		if le.What != what || le.Limit != limit {
			t.Errorf("LimitError = %+v, want %q / %d", le, what, limit)
		}
	}

	t.Run("code lines", func(t *testing.T) {
		source := []byte("```\n" + strings.Repeat("x\n", 8193) + "```\n")
		_, err := goldmark.Build(ctx, parse(t, source), source, goldmark.Options{})
		check(t, err, "typeset code block exceeds 8,192 lines", 8192)

		source = []byte("```\n" + strings.Repeat("x\n", 8192) + "```\n")
		if _, err := goldmark.Build(ctx, parse(t, source), source, goldmark.Options{}); err != nil {
			t.Errorf("8,192 lines: %v", err)
		}
	})
	t.Run("blocks", func(t *testing.T) {
		source := []byte(strings.Repeat("p\n\n", 32769))
		_, err := goldmark.Build(ctx, parse(t, source), source, goldmark.Options{})
		check(t, err, "typeset document exceeds 32,768 blocks", 32768)

		source = []byte(strings.Repeat("p\n\n", 32768))
		if _, err := goldmark.Build(ctx, parse(t, source), source, goldmark.Options{}); err != nil {
			t.Errorf("32,768 blocks: %v", err)
		}
	})
	t.Run("cancelled", func(t *testing.T) {
		source := []byte("a\n\nb\n")
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		doc, err := goldmark.Build(cancelled, parse(t, source), source, goldmark.Options{})
		if err != context.Canceled || doc != nil {
			t.Errorf("Build = %v, %v, want nil, context.Canceled unwrapped", doc, err)
		}
	})
}

func TestBuildBoundsHook(t *testing.T) {
	source := []byte("![pic](landscape.png)\n")
	var seen []string
	doc, err := goldmark.Build(context.Background(), parse(t, source), source, goldmark.Options{
		Images: map[string]nttypeset.Image{"landscape.png": {Still: landscape(t), Animated: true}},
		Bounds: func(im *ast.Image) nttypeset.Bounds {
			seen = append(seen, string(im.Destination))
			return nttypeset.Bounds{Width: 10, Align: "center"}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Blocks) != 1 || doc.Blocks[0].Kind != nttypeset.KindImage {
		t.Fatalf("blocks = %+v", doc.Blocks)
	}
	im := doc.Blocks[0].Image
	if im.Bounds != (nttypeset.Bounds{Width: 10, Align: "center"}) {
		t.Errorf("Bounds = %+v", im.Bounds)
	}
	if im.Key != "landscape.png" || !im.Animated || im.Still == nil {
		t.Errorf("image = %+v", im)
	}
	if len(seen) != 1 || seen[0] != "landscape.png" {
		t.Errorf("hook saw %q", seen)
	}
}

func TestBuildLinkedImageAndNoPanic(t *testing.T) {
	source := []byte("[![pic](a.png)](https://example.com)\n")
	doc, err := goldmark.Build(context.Background(), parse(t, source), source, goldmark.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Blocks) != 1 || doc.Blocks[0].Href != "https://example.com" {
		t.Errorf("blocks = %+v", doc.Blocks)
	}
	// A nil root is the caller's error, not a panic.
	if _, err := goldmark.Build(context.Background(), nil, nil, goldmark.Options{}); err == nil {
		t.Error("nil root: want an error")
	}
}
