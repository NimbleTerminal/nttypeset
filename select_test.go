package nttypeset

import (
	"context"
	"image"
	"image/color"
	"strings"
	"testing"
	"unicode"
)

const selectWidth = 400

// selectDocument mirrors the gloss selection fixture: a heading, two
// paragraphs, a two-line code block, a table, and a paragraph that wraps.
func selectDocument() *Document {
	return &Document{Blocks: []Block{
		{Kind: KindHeading, Spans: []Span{{Text: "Title", Style: Bold}}, Size: 50},
		{Kind: KindParagraph, Spans: []Span{{Text: "Hello wide world of words."}}},
		{Kind: KindParagraph, Spans: []Span{{Text: "Second paragraph here."}}},
		{Kind: KindCode, Spans: []Span{{Text: "code line one\ncode line two", Style: Mono}}},
		{Kind: KindTable, Rows: [][][]Span{
			{{{Text: "A", Style: Bold}}, {{Text: "B", Style: Bold}}},
			{{{Text: "alpha"}}, {{Text: "beta"}}},
		}},
		{Kind: KindParagraph, Spans: []Span{{Text: strings.Repeat("wrapped words ", 40) + "end."}}},
	}}
}

func selectLayout(t *testing.T, e *Engine, doc *Document, width int, opts LayoutOptions) *Layout {
	t.Helper()
	l := mustLayout(t, e, doc, width, opts)
	if err := l.Ensure(context.Background(), 100000); err != nil {
		t.Fatal(err)
	}
	return l
}

// runAt is the first text operation whose paragraph holds s, and the page x
// where s starts in it.
func runAt(t *testing.T, l *Layout, s string) (*operation, int) {
	t.Helper()
	for i := range l.ops {
		op := &l.ops[i]
		if op.run == nil || op.gutter {
			continue
		}
		at := strings.Index(string(op.text), s)
		if at < 0 {
			continue
		}
		r := len([]rune(string(op.text)[:at]))
		if r < op.run.Runes.Offset || r >= op.run.Runes.Offset+op.run.Runes.Count {
			continue
		}
		return op, runX(op, r-op.run.Runes.Offset)
	}
	t.Fatalf("no run holds %q", s)
	return nil, 0
}

func mid(op *operation) int { return (op.rect.Min.Y + op.rect.Max.Y) / 2 }

// endX is the page x just inside the end of s in op's paragraph.
func endX(op *operation, s string) int {
	return runX(op, strings.Index(string(op.text), s)+len([]rune(s))-op.run.Runes.Offset) - 1
}

func TestTypesetSelectionTextAndPlaces(t *testing.T) {
	ctx := context.Background()
	e := testEngine(t)
	doc := selectDocument()
	l := selectLayout(t, e, doc, selectWidth, LayoutOptions{})
	from, x0 := runAt(t, l, "wide")
	to, _ := runAt(t, l, "Second")
	s, err := l.Select(ctx, Selection{
		Anchor: Place{Point: image.Pt(x0+1, mid(from))},
		Focus:  Place{Point: image.Pt(endX(to, "Second"), mid(to))},
	})
	if err != nil || s == nil {
		t.Fatalf("select: %v %v", s, err)
	}
	const want = "wide world of words.\n\nSecond"
	if got := s.Text(); got != want {
		t.Fatalf("selected %q", got)
	}
	p := s.Places()
	if !p.Anchor.Placed || !p.Focus.Placed || p.Anchor.Block == p.Focus.Block {
		t.Fatalf("places not resolved: %+v", p)
	}
	if p.Anchor.Point != image.Pt(x0+1, mid(from)) {
		t.Fatalf("anchor point not kept: %+v", p.Anchor)
	}
	// The same text reads back when the places, not the points, are given,
	// in either order and at another width.
	reversed := Selection{Anchor: p.Focus, Focus: p.Anchor}
	reversed.Anchor.Point, reversed.Focus.Point = image.Point{}, image.Point{}
	wide := selectLayout(t, e, doc, 2*selectWidth, LayoutOptions{})
	ws, err := wide.Select(ctx, reversed)
	if err != nil || ws == nil || ws.Text() != want {
		t.Fatalf("reflowed selection %v %v", ws, err)
	}
	p.Anchor.Point, p.Focus.Point = image.Point{}, image.Point{}
	if got := ws.Places(); got.Anchor != p.Focus || got.Focus != p.Anchor {
		t.Fatalf("places changed across widths: %+v", got)
	}
	// A place that is not in the document selects nothing.
	gone, err := l.Select(ctx, Selection{Anchor: Place{Block: 99, Placed: true}, Focus: p.Focus})
	if err != nil || gone != nil {
		t.Fatalf("missing place selected %v %v", gone, err)
	}
	gone, err = l.Select(ctx, Selection{Anchor: p.Anchor, Focus: Place{Block: 1, Para: 7, Placed: true}})
	if err != nil || gone != nil {
		t.Fatalf("missing paragraph selected %v %v", gone, err)
	}
}

func TestTypesetSelectionSeparators(t *testing.T) {
	ctx := context.Background()
	e := testEngine(t)
	l := selectLayout(t, e, selectDocument(), selectWidth, LayoutOptions{})
	pick := func(from, to string) string {
		t.Helper()
		a, x0 := runAt(t, l, from)
		b, _ := runAt(t, l, to)
		s, err := l.Select(ctx, Selection{
			Anchor: Place{Point: image.Pt(x0+1, mid(a))},
			Focus:  Place{Point: image.Pt(endX(b, to), mid(b))},
		})
		if err != nil || s == nil {
			t.Fatalf("select: %v %v", s, err)
		}
		return s.Text()
	}
	if got := pick("code line one", "code line two"); got != "code line one\ncode line two" {
		t.Fatalf("code lines %q", got)
	}
	if got := pick("alpha", "beta"); got != "alpha\tbeta" {
		t.Fatalf("table cells %q", got)
	}
	if got := pick("Title", "Hello"); got != "Title\n\nHello" {
		t.Fatalf("blocks %q", got)
	}
	got := pick("wrapped", "end.")
	if strings.Contains(got, "  ") || strings.Contains(got, "\n") || !strings.HasSuffix(got, "end.") || strings.Count(got, "wrapped") != 40 {
		t.Fatalf("wrapped paragraph %q", got)
	}
	// Points off the text snap to the nearest line: the margin left of a
	// line is its start, below the last line is its end.
	a, _ := runAt(t, l, "Hello")
	s, err := l.Select(ctx, Selection{Anchor: Place{Point: image.Pt(0, a.rect.Min.Y)}, Focus: Place{Point: image.Pt(1<<20, 1<<20)}})
	if err != nil || s == nil || !strings.HasPrefix(s.Text(), "Hello wide") || !strings.HasSuffix(s.Text(), "end.") {
		t.Fatalf("snapped selection %v %v", s, err)
	}
}

func TestTypesetSelectionHighlight(t *testing.T) {
	ctx := context.Background()
	e := testEngine(t)
	l := selectLayout(t, e, selectDocument(), selectWidth, LayoutOptions{})
	view := View{Width: selectWidth, Height: 1200, Scale: 1}
	pal := LightPalette()
	plain, err := l.Raster(ctx, view, RasterOptions{Palette: pal})
	if err != nil {
		t.Fatal(err)
	}
	a, x0 := runAt(t, l, "wide")
	x1 := runX(a, strings.Index(string(a.text), "world")-a.run.Runes.Offset)
	y := mid(a)
	s, err := l.Select(ctx, Selection{Anchor: Place{Point: image.Pt(x0+1, y)}, Focus: Place{Point: image.Pt(x1-1, y)}})
	if err != nil || s == nil || s.Text() != "wide " {
		t.Fatalf("selected %v %v", s, err)
	}
	r, err := l.Raster(ctx, view, RasterOptions{Palette: pal, Selection: s})
	if err != nil {
		t.Fatal(err)
	}
	b := plain.Bounds()
	inside, outside := 0, 0
	for yy := b.Min.Y; yy < b.Max.Y; yy++ {
		for xx := b.Min.X; xx < b.Max.X; xx++ {
			before, after := plain.At(xx, yy), r.At(xx, yy)
			if before == after {
				continue
			}
			if color.RGBAModel.Convert(after) == pal.Selection {
				if xx >= x0 && xx < x1 && yy >= a.rect.Min.Y && yy < a.rect.Max.Y {
					inside++
				} else {
					outside++
				}
			}
		}
	}
	if inside == 0 || outside > 0 {
		t.Fatalf("highlight pixels inside %d outside %d", inside, outside)
	}
	// A selection of another layout is not drawn.
	other := selectLayout(t, e, selectDocument(), selectWidth, LayoutOptions{})
	r, err = l.Raster(ctx, view, RasterOptions{Palette: pal, Selection: mustSelect(t, other, s.Places())})
	if err != nil {
		t.Fatal(err)
	}
	if !equalImages(plain, r) {
		t.Fatal("a selection of another layout was drawn")
	}
}

func mustSelect(t *testing.T, l *Layout, s Selection) *Selected {
	t.Helper()
	sel, err := l.Select(context.Background(), s)
	if err != nil || sel == nil {
		t.Fatalf("select: %v %v", sel, err)
	}
	return sel
}

func equalImages(a, b *image.RGBA) bool {
	return a.Rect == b.Rect && string(a.Pix) == string(b.Pix)
}

// Copy uses logical text, including whitespace omitted by visual wrapping.
func TestTypesetSelectionPreservesWrappedText(t *testing.T) {
	ctx := context.Background()
	token := strings.Repeat("abcdefghij", 25)
	for _, tc := range []struct {
		name  string
		block Block
		want  string
	}{
		{"token", Block{Spans: []Span{{Text: token}}}, token},
		{"url", Block{Spans: []Span{{Text: "https://example.com/" + token}}}, "https://example.com/" + token},
		{"spaces", Block{Spans: []Span{{Text: strings.Repeat("café   résumé ", 20) + "end"}}}, strings.Repeat("café   résumé ", 20) + "end"},
		{"code", Block{Kind: KindCode, Spans: []Span{{Text: token + "   tail\n    next line", Style: Mono}}}, token + "   tail\n    next line"},
		{"styles", Block{Spans: []Span{{Text: "start "}, {Text: token, Style: Bold}, {Text: " end"}}}, "start " + token + " end"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := testEngine(t)
			doc := &Document{Blocks: []Block{tc.block}}
			l := selectLayout(t, e, doc, selectWidth, LayoutOptions{})
			s := mustSelect(t, l, Selection{Anchor: Place{Point: image.Pt(0, 0)}, Focus: Place{Point: image.Pt(99999, 99999)}})
			if got := s.Text(); got != tc.want {
				t.Fatalf("copy differs: got %q, want %q", got, tc.want)
			}
			// Placed endpoints preserve the exact bytes through font and width changes.
			if _, err := e.SetFonts("technical", ""); err != nil {
				t.Fatal(err)
			}
			wide := selectLayout(t, e, doc, 2*selectWidth, LayoutOptions{})
			ws, err := wide.Select(ctx, s.Places())
			if err != nil || ws == nil || ws.Text() != tc.want {
				t.Fatalf("reflowed copy %v: %v", ws, err)
			}
		})
	}
}

func TestSelectPastDocument(t *testing.T) {
	ctx := context.Background()
	e := testEngine(t)
	doc := selectDocument()
	l := mustLayout(t, e, doc, selectWidth, LayoutOptions{})
	s, err := l.Select(ctx, Selection{Anchor: Place{Placed: true, Block: 10_000}, Focus: Place{Placed: true}})
	if err != nil || s != nil {
		t.Fatalf("select past the document: %v %v", s, err)
	}
	if l.Laid() != len(doc.Blocks) {
		t.Fatalf("laid %d blocks, want %d", l.Laid(), len(doc.Blocks))
	}
	// Negative places and an empty layout are not found either.
	for _, p := range []Place{{Placed: true, Block: -1}, {Placed: true, Para: -1}, {Point: image.Pt(5, 5)}} {
		fresh := mustLayout(t, e, &Document{}, selectWidth, LayoutOptions{})
		if s, err := fresh.Select(ctx, Selection{Anchor: p, Focus: p}); err != nil || s != nil {
			t.Fatalf("empty layout selected %+v: %v %v", p, s, err)
		}
	}
	if s, err := l.Select(ctx, Selection{Anchor: Place{Placed: true, Block: -1}, Focus: Place{Placed: true}}); err != nil || s != nil {
		t.Fatalf("negative block selected: %v %v", s, err)
	}
	// A cancelled request is an error, not a panic.
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	late := mustLayout(t, e, doc, selectWidth, LayoutOptions{})
	if _, err := late.Select(cancelled, Selection{Anchor: Place{Placed: true, Block: 3}, Focus: Place{Placed: true}}); err == nil {
		t.Fatal("cancelled select returned no error")
	}
}

func TestSelectionSkipsGutterNumbers(t *testing.T) {
	ctx := context.Background()
	e := testEngine(t)
	doc, err := Lines(ctx, "alpha one\nbeta two\ngamma three\n", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	l := selectLayout(t, e, doc, 800, LayoutOptions{Numbers: true})
	// From the gutter of line 1 to the far corner: the numbers are not text.
	s := mustSelect(t, l, Selection{Anchor: Place{Point: image.Pt(0, 0)}, Focus: Place{Point: image.Pt(99999, 99999)}})
	const want = "alpha one\n\nbeta two\n\ngamma three"
	if got := s.Text(); got != want {
		t.Fatalf("selected %q, want %q", got, want)
	}
	if strings.IndexFunc(s.Text(), unicode.IsDigit) >= 0 {
		t.Fatalf("gutter digits in %q", s.Text())
	}
	p := s.Places()
	if p.Anchor.Para < 0 || p.Focus.Para < 0 {
		t.Fatalf("a gutter place: %+v", p)
	}
}

func TestNilSelected(t *testing.T) {
	var s *Selected
	if got := s.Places(); got != (Selection{}) {
		t.Fatalf("Places = %+v", got)
	}
	if got := s.Text(); got != "" {
		t.Fatalf("Text = %q", got)
	}
}
