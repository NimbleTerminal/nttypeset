package nttypeset

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
)

const fixtureCode = "func greeting() string {\n    return \"Hello, café!\"\n}"

// fixtureDocument mirrors testdata/typeset.md's block kinds, built by hand,
// with enough paragraphs after it that a 200-pixel view does not lay out
// every block.
func fixtureDocument(t *testing.T) *Document {
	t.Helper()
	data, err := os.ReadFile("testdata/landscape.png")
	if err != nil {
		t.Fatal(err)
	}
	landscape, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	heading := []Span{{Text: "Typeset Markdown", Style: Bold, Role: RoleAccent}}
	doc := &Document{Blocks: []Block{
		{Kind: KindHeading, Spans: heading, Size: 50},
		{Kind: KindParagraph, Spans: []Span{
			{Text: "A quiet page with café, naïve, and résumé in an embedded font. "},
			{Text: "Bold text", Style: Bold},
			{Text: ", "},
			{Text: "italic text", Style: Italic},
			{Text: ", and "},
			{Text: "struck text", Style: Strike},
			{Text: " stay on the page with the "},
			{Text: "pictures", Href: "https://example.org/", Role: RoleAccent},
			{Text: "."},
		}},
		{Kind: KindQuote, Spans: []Span{{Text: "A quotation has room to breathe."}}, Indent: 24, Quotes: []Quote{{Indent: 24, Last: true}}},
		{Kind: KindCode, Spans: Highlight(context.Background(), fixtureCode, "go", styles.Get("github")), Size: 25},
		{Kind: KindTable, Rows: [][][]Span{
			{{{Text: "Element", Style: Bold}}, {{Text: "Purpose", Style: Bold}}, {{Text: "Notes", Style: Bold}}},
			{{{Text: "Heading"}}, {{Text: "A place to start"}}, {{Text: "first"}}},
			{{{Text: "Table"}}, {{Text: "A few facts together"}}, {{Text: "second"}}},
		}},
		{Kind: KindParagraph, Spans: []Span{{Text: "• "}, {Text: "[x] ", Style: Mono}, {Text: "Read a page"}}, Indent: 24},
		{Kind: KindParagraph, Spans: []Span{{Text: "• "}, {Text: "[ ] ", Style: Mono}, {Text: "Turn to the next one"}}, Indent: 24},
		{Kind: KindImage, Image: &Image{Key: "landscape.png", Still: landscape}, Spans: []Span{{Text: "[image unavailable: landscape.png]", Style: Italic}}},
		{Kind: KindImage, Image: &Image{Key: "missing-typeset-image.png"}, Spans: []Span{{Text: "[image unavailable: missing-typeset-image.png]", Style: Italic}}},
	}}
	for range 20 {
		doc.Blocks = append(doc.Blocks, Block{Kind: KindParagraph, Spans: []Span{{Text: "A closing paragraph with "}, {Text: "bold", Style: Bold}, {Text: " words that fill the page."}}})
	}
	return doc
}

func mustLayout(t *testing.T, e *Engine, doc *Document, width int, opts LayoutOptions) *Layout {
	t.Helper()
	l, err := e.Layout(doc, width, opts)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func mustRaster(t *testing.T, l *Layout, v View, p Palette) *image.RGBA {
	t.Helper()
	img, err := l.Raster(context.Background(), v, RasterOptions{Palette: p})
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func TestFixtureAndProgressiveLayout(t *testing.T) {
	doc := fixtureDocument(t)
	kinds := map[Kind]bool{}
	for _, b := range doc.Blocks {
		kinds[b.Kind] = true
	}
	for _, k := range []Kind{KindHeading, KindParagraph, KindQuote, KindCode, KindTable, KindImage} {
		if !kinds[k] {
			t.Errorf("missing kind %d", k)
		}
	}
	ctx := context.Background()
	light := LightPalette()
	e := testEngine(t)
	l := mustLayout(t, e, doc, 800, LayoutOptions{})
	img := mustRaster(t, l, View{Width: 800, Height: 200, Scale: 1}, light)
	if img.Bounds() != image.Rect(0, 0, 800, 200) || l.Done() {
		t.Fatal("first viewport laid out the whole fixture")
	}
	firstOps := len(l.ops)
	if err := l.Complete(ctx); err != nil {
		t.Fatal(err)
	}
	if !l.Done() || l.Laid() != len(doc.Blocks) || len(l.ops) <= firstOps || len(l.Starts()) != len(doc.Blocks) {
		t.Fatal("remaining layout did not finish")
	}
	full := mustRaster(t, l, View{Width: 800, Height: l.Extent(), Scale: 1}, light)
	if full.RGBAAt(50, 50) == (color.RGBA{}) {
		t.Fatal("transparent canvas")
	}
	placed := l.Images(View{Width: 800, Height: l.Extent(), Scale: 1})
	if len(placed) != 1 || placed[0].Key != "landscape.png" || placed[0].Rect.Empty() {
		t.Fatalf("placed images %+v", placed)
	}
	// A scrolled viewport must agree pixel-for-pixel with the same full layout.
	top, height := 317, 240
	scrolled := mustRaster(t, l, View{Top: top, Width: 800, Height: height, Scale: 1}, light)
	for y := 0; y < height; y++ {
		for x := 0; x < 800; x++ {
			if full.RGBAAt(x, y+top) != scrolled.RGBAAt(x, y) {
				t.Fatalf("scroll differs at %d,%d", x, y)
			}
		}
	}
	// Cached shaping must produce the same narrow image as a fresh engine.
	cachedNarrow := mustRaster(t, mustLayout(t, e, doc, 600, LayoutOptions{}), View{Width: 600, Height: 900, Scale: 1}, light)
	freshNarrow := mustRaster(t, mustLayout(t, testEngine(t), doc, 600, LayoutOptions{}), View{Width: 600, Height: 900, Scale: 1}, light)
	if !bytes.Equal(cachedNarrow.Pix, freshNarrow.Pix) {
		t.Fatal("wrapping mutated cached shaping across widths")
	}
	small := mustLayout(t, e, doc, 400, LayoutOptions{})
	if err := small.Complete(ctx); err != nil {
		t.Fatal(err)
	}
	if small.Extent() <= l.Extent() {
		t.Fatal("narrow layout should wrap to more lines")
	}
	again := mustRaster(t, l, View{Width: 800, Height: 200, Scale: 1}, light)
	if !bytes.Equal(img.Pix, again.Pix) {
		t.Fatal("resize/remaining layout mutated existing glyph runs")
	}
}

func TestTextWrappingPreservesUnicodeAndStyles(t *testing.T) {
	e := testEngine(t)
	spans := []Span{{Text: "Café naïve "}, {Text: "résumé ", Style: Bold}, {Text: "élan and a long sentence", Style: Italic}}
	s := e.shape(spans, 25)
	for _, run := range s.runs {
		for _, g := range run.Glyphs {
			if g.GlyphID == 0 {
				t.Fatal("missing Latin glyph")
			}
		}
	}
	// A width of 220 leaves the paragraph 120 pixels between the margins.
	doc := &Document{Blocks: []Block{
		{Kind: KindParagraph, Spans: spans},
		{Kind: KindParagraph, Spans: []Span{{Text: "one\n\nthree", Style: Mono}}},
		{Kind: KindParagraph, Spans: []Span{{Text: "end"}}},
	}}
	l := mustLayout(t, e, doc, 220, LayoutOptions{})
	if err := l.EnsureBlock(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if l.Laid() < 2 {
		t.Fatalf("laid %d blocks", l.Laid())
	}
	starts := l.Starts()
	if starts[0] != Margin || starts[1]-starts[0]-16 <= 37 {
		t.Fatalf("text did not wrap: starts %v", starts)
	}
	covered := make([]bool, len(s.text))
	seen := map[Style]bool{}
	for _, op := range l.ops {
		if op.run == nil || op.block != 0 {
			continue
		}
		seen[op.style] = true
		for i := op.run.Runes.Offset; i < op.run.Runes.Offset+op.run.Runes.Count; i++ {
			if i >= len(covered) {
				t.Fatal("run out of bounds")
			}
			covered[i] = true
		}
	}
	for i, r := range s.text {
		if r != ' ' && !covered[i] {
			t.Fatalf("lost rune %q at %d", r, i)
		}
	}
	if !seen[Bold] || !seen[Italic] {
		t.Fatal("lost mixed styles")
	}
	if l.Laid() == 2 {
		if err := l.EnsureBlock(context.Background(), 2); err != nil {
			t.Fatal(err)
		}
	}
	starts = l.Starts()
	if h := starts[2] - starts[1] - 16; h != 3*37 {
		t.Fatalf("hard/empty line height %d", h)
	}
}

func TestHighlightedRasterResizeAndThemeCache(t *testing.T) {
	github := styles.Get("github")
	doc := &Document{Blocks: []Block{{Kind: KindCode, Spans: Highlight(context.Background(), fixtureCode, "go", github), Size: 25}}}
	light := LightPalette()
	e := testEngine(t)
	wide := mustRaster(t, mustLayout(t, e, doc, 800, LayoutOptions{}), View{Width: 800, Height: 300, Scale: 1}, light)
	narrow := mustRaster(t, mustLayout(t, e, doc, 400, LayoutOptions{}), View{Width: 400, Height: 400, Scale: 1}, light)
	fresh := mustRaster(t, mustLayout(t, testEngine(t), doc, 400, LayoutOptions{}), View{Width: 400, Height: 400, Scale: 1}, light)
	if !bytes.Equal(narrow.Pix, fresh.Pix) {
		t.Fatal("highlight colors/spacing changed after resize")
	}
	colors := map[color.RGBA]bool{}
	for _, sp := range doc.Blocks[0].Spans {
		c := light.Syntax.Get(sp.Token).Colour
		ink := color.RGBA{c.Red(), c.Green(), c.Blue(), 255}
		if sp.Token != 0 && c.IsSet() && ink.R != ink.G {
			colors[ink] = false
		}
	}
	for y := 0; y < wide.Bounds().Dy(); y++ {
		for x := 0; x < wide.Bounds().Dx(); x++ {
			if _, ok := colors[wide.RGBAAt(x, y)]; ok {
				colors[wide.RGBAAt(x, y)] = true
			}
		}
	}
	found := 0
	for _, present := range colors {
		if present {
			found++
		}
	}
	if found < 2 {
		t.Fatal("token colors did not reach raster output")
	}
	a := e.shape(Highlight(context.Background(), fixtureCode, "go", github), 25)
	tinted := Highlight(context.Background(), fixtureCode, "go", github)
	tinted[0].Ink = color.RGBA{1, 2, 3, 255}
	if b := e.shape(tinted, 25); a == b {
		t.Fatal("span colors collided in shape cache")
	}
}

// Glyphs are drawn from cached masks: a second raster of the same view hits
// the cache for every glyph and is identical, the result stays within a
// quarter pixel of drawing each run straight from its outlines, and the cache
// stays within its budget.
func TestTypesetGlyphMasks(t *testing.T) {
	doc := &Document{Blocks: []Block{{Kind: KindHeading, Spans: []Span{{Text: "Glyph masks", Style: Bold, Role: RoleAccent}}, Size: 50}}}
	for range 30 {
		doc.Blocks = append(doc.Blocks, Block{Kind: KindParagraph, Spans: []Span{{Text: "Café and "}, {Text: "a paragraph", Style: Italic}, {Text: " with "}, {Text: "code", Style: Mono}, {Text: "."}}})
	}
	e := testEngine(t)
	pal := LightPalette()
	// 400 points at 150 DPI, a 600-pixel view shown in 1000×720 device pixels.
	width := 833
	v := View{Width: width, Height: 600, Scale: 1000.0 / float64(width), Pixels: image.Pt(1000, 720)}
	lay := mustLayout(t, e, doc, width, LayoutOptions{})
	first := mustRaster(t, lay, v, pal)
	if first.Bounds().Size() != v.Pixels {
		t.Fatalf("raster %v", first.Bounds())
	}
	stats := e.GlyphCache()
	if stats.Misses == 0 || stats.Hits == 0 || stats.Entries == 0 {
		t.Fatalf("cache unused: %d misses, %d hits, %d masks", stats.Misses, stats.Hits, stats.Entries)
	}
	misses := stats.Misses
	second := mustRaster(t, lay, v, pal)
	if e.GlyphCache().Misses != misses {
		t.Fatalf("a second raster missed %d more glyphs", e.GlyphCache().Misses-misses)
	}
	if !bytes.Equal(first.Pix, second.Pix) {
		t.Fatal("cached raster differs from the first")
	}
	// Against go-text drawing every run straight from its outlines.
	direct := image.NewRGBA(first.Bounds())
	draw.Draw(direct, direct.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	scale := v.Scale
	inkRuns := 0
	for _, op := range lay.ops {
		if op.run == nil {
			continue
		}
		rr := op.rect.Sub(image.Pt(v.Left, v.Top))
		rr = image.Rect(int(float64(rr.Min.X)*scale+0.5), int(float64(rr.Min.Y)*scale+0.5), int(float64(rr.Max.X)*scale+0.5), int(float64(rr.Max.Y)*scale+0.5))
		if !rr.Overlaps(direct.Bounds()) {
			continue
		}
		inkRuns++
		ink := pal.runInk(&op)
		pad := int(float64(op.size)*scale) + 1
		tile := image.NewRGBA(image.Rect(0, 0, rr.Dx()+2*pad, rr.Dy()+2*pad))
		e.rasterizer.DrawDirect(tile, op.run, op.size, scale, pad, pad+int(float64(op.baseline-v.Top)*scale+0.5)-rr.Min.Y, rr.Dy(), ink)
		draw.Draw(direct, tile.Bounds().Add(rr.Min.Sub(image.Pt(pad, pad))), tile, image.Point{}, draw.Over)
	}
	if inkRuns == 0 {
		t.Fatal("no text runs in view")
	}
	var sum, worst, ink int
	for i := 0; i < len(first.Pix); i += 4 {
		d := 0
		for c := 0; c < 3; c++ {
			d = max(d, int(first.Pix[i+c])-int(direct.Pix[i+c]), int(direct.Pix[i+c])-int(first.Pix[i+c]))
		}
		if first.Pix[i] != 255 || direct.Pix[i] != 255 {
			ink++
			sum += d
		}
		worst = max(worst, d)
	}
	if ink == 0 || sum/ink > 12 || worst > 160 {
		t.Fatalf("cached glyphs stray from direct rendering: mean %d over %d ink pixels, worst %d", sum/max(1, ink), ink, worst)
	}
	// The budget is kept on insertion: a new scale misses on every glyph and
	// empties the cache as needed, so it holds at most the budget plus the
	// one mask that overflowed it.
	e.SetGlyphBudget(1 << 12)
	width = 875 // 420 points
	v = View{Width: width, Height: 600, Scale: 1000.0 / float64(width), Pixels: image.Pt(1000, 720)}
	mustRaster(t, mustLayout(t, e, doc, width, LayoutOptions{}), v, pal)
	if s := e.GlyphCache(); s.Misses == misses || s.Bytes > 2*s.Budget {
		t.Fatalf("cache over budget: %+v after %d misses", s, s.Misses-misses)
	}
}

func TestLayoutLimits(t *testing.T) {
	ctx := context.Background()
	e := testEngine(t)
	long := &Document{Blocks: []Block{{Kind: KindParagraph, Spans: []Span{{Text: strings.Repeat("a", 16385)}}}}}
	l := mustLayout(t, e, long, 800, LayoutOptions{})
	if err := l.Complete(ctx); !errors.Is(err, ErrLimit) {
		t.Fatalf("paragraph: %v", err)
	}
	if l.Laid() != 0 || len(l.Starts()) != 0 {
		t.Fatal("failed block was kept")
	}
	glyphs := &Document{}
	for range 16 {
		glyphs.Blocks = append(glyphs.Blocks, Block{Kind: KindParagraph, Spans: []Span{{Text: strings.Repeat("a", 16384)}}})
	}
	glyphs.Blocks = append(glyphs.Blocks, Block{Kind: KindParagraph, Spans: []Span{{Text: "a"}}})
	l = mustLayout(t, e, glyphs, 800, LayoutOptions{})
	err := l.Complete(ctx)
	var le *LimitError
	if !errors.As(err, &le) || le.What != "typeset layout exceeds 262,144 glyphs" || le.Limit != 262144 {
		t.Fatalf("glyphs: %v", err)
	}
	if l.Laid() != 16 {
		t.Fatalf("laid %d blocks", l.Laid())
	}
	// Runs, blocks, operations and the deadline report their configured limits.
	tight, err := NewEngine(WithFonts(aliasSource{}), WithLimits(Limits{Runs: 2, Blocks: 2, Operations: 3}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tight.Layout(&Document{Blocks: make([]Block, 3)}, 800, LayoutOptions{}); !errors.As(err, &le) || le.What != "typeset document exceeds 2 blocks" {
		t.Fatalf("blocks: %v", err)
	}
	l = mustLayout(t, tight, &Document{Blocks: []Block{{Spans: []Span{{Text: "a"}, {Text: "b"}, {Text: "c"}}}}}, 800, LayoutOptions{})
	if err := l.Complete(ctx); !errors.As(err, &le) || le.What != "typeset block exceeds 2 style runs" {
		t.Fatalf("runs: %v", err)
	}
	l = mustLayout(t, tight, &Document{Blocks: []Block{{Spans: []Span{{Text: "one\ntwo\nthree\nfour"}}}}}, 800, LayoutOptions{})
	if err := l.Complete(ctx); !errors.As(err, &le) || le.What != "typeset layout exceeds display-list limit" {
		t.Fatalf("operations: %v", err)
	}
	if _, err := e.Layout(long, 0, LayoutOptions{}); err == nil || err.Error() != "nttypeset: width must be positive" {
		t.Fatalf("width: %v", err)
	}
}

// countdown is a context that is cancelled after a number of checks, so a
// request can be given up at a chosen point inside layout.
type countdown struct {
	context.Context
	left *int
}

func (c countdown) Err() error {
	if *c.left <= 0 {
		return context.Canceled
	}
	*c.left--
	return nil
}

func TestCancelledEnsureRollsBackOneBlock(t *testing.T) {
	doc := &Document{Blocks: []Block{{Kind: KindHeading, Spans: []Span{{Text: "Title", Style: Bold, Role: RoleAccent}}, Size: 50}}}
	for range 60 {
		doc.Blocks = append(doc.Blocks, Block{Kind: KindParagraph, Spans: []Span{{Text: "A paragraph of a few words that wraps."}}})
	}
	ctx := context.Background()
	lay := mustLayout(t, testEngine(t), doc, 400, LayoutOptions{})
	if err := lay.Ensure(ctx, 600); err != nil {
		t.Fatal(err)
	}
	done, cancel := context.WithCancel(ctx)
	cancel()
	laid := lay.Laid()
	if err := lay.Ensure(done, 3000); !errors.Is(err, context.Canceled) || lay.Laid() != laid {
		t.Fatalf("cancelled ensure: %v, laid %d then %d", err, laid, lay.Laid())
	}
	left := 40
	if err := lay.Ensure(countdown{ctx, &left}, 1<<30); !errors.Is(err, context.Canceled) {
		t.Fatalf("countdown did not cancel the layout: %v", err)
	}
	if len(lay.Starts()) != lay.Laid() || lay.Done() {
		t.Fatalf("layout inconsistent after cancel: %d starts, %d of %d blocks", len(lay.Starts()), lay.Laid(), len(doc.Blocks))
	}
	partial := lay.Laid()
	if err := lay.Complete(ctx); err != nil || lay.Laid() <= partial {
		t.Fatalf("resumed: %v, blocks %d then %d", err, partial, lay.Laid())
	}
	fresh := mustLayout(t, testEngine(t), doc, 400, LayoutOptions{})
	if err := fresh.Complete(ctx); err != nil || len(fresh.ops) != len(lay.ops) {
		t.Fatalf("resumed layout has %d ops, fresh %d", len(lay.ops), len(fresh.ops))
	}
	for i := range lay.ops {
		if lay.ops[i].rect != fresh.ops[i].rect {
			t.Fatalf("op %d differs: %v vs %v", i, lay.ops[i].rect, fresh.ops[i].rect)
		}
	}
}

func linkDocument() *Document {
	const url = "https://example.org/"
	still := image.NewRGBA(image.Rect(0, 0, 40, 30))
	draw.Draw(still, still.Bounds(), image.NewUniform(color.RGBA{200, 50, 90, 255}), image.Point{}, draw.Src)
	return &Document{Blocks: []Block{
		{Kind: KindHeading, Spans: []Span{{Text: "Heading", Style: Bold, Role: RoleAccent}}, Size: 50},
		{Kind: KindParagraph, Spans: []Span{{Text: strings.Repeat("linked words ", 24), Href: url, Role: RoleAccent}}},
		{Kind: KindParagraph, Spans: []Span{{Text: "https://example.com/", Href: "https://example.com/", Role: RoleAccent}}},
		{Kind: KindTable, Rows: [][][]Span{{{{Text: "Table", Style: Bold}}}, {{{Text: "cell", Href: "https://example.net/", Role: RoleAccent}}}}},
		{Kind: KindImage, Image: &Image{Key: "local.png", Still: still}, Href: "https://example.edu/"},
	}}
}

func TestLinksWrapReflowAndViewport(t *testing.T) {
	const url = "https://example.org/"
	doc := linkDocument()
	e := testEngine(t)
	lay := mustLayout(t, e, doc, 400, LayoutOptions{})
	v := View{Width: 400, Height: 1200, Scale: 1}
	img := mustRaster(t, lay, v, LightPalette())
	counts := map[string]int{}
	for _, link := range lay.Links(v) {
		counts[link.URL]++
		if link.Rect.Empty() || !link.Rect.In(img.Bounds()) {
			t.Fatal("bad link rect", link)
		}
	}
	if counts[url] < 2 {
		t.Fatal("link did not wrap", counts)
	}
	for _, u := range []string{"https://example.com/", "https://example.net/", "https://example.edu/"} {
		if counts[u] == 0 {
			t.Fatal("missing link", u)
		}
	}
	// Scrolled and zoomed out: half the width, shown in 384×300 pixels.
	zoomed := View{Top: 100, Width: 200, Height: 75, Scale: 384.0 / 200, Pixels: image.Pt(384, 300)}
	img = mustRaster(t, lay, zoomed, LightPalette())
	links := lay.Links(zoomed)
	if len(links) == 0 {
		t.Fatal("no links in the zoomed view")
	}
	for _, link := range links {
		if !link.Rect.In(img.Bounds()) {
			t.Fatal("link not clipped after scroll/zoom")
		}
	}
	wide := mustLayout(t, e, doc, 800, LayoutOptions{})
	v = View{Width: 800, Height: 1200, Scale: 1}
	mustRaster(t, wide, v, LightPalette())
	n := 0
	for _, link := range wide.Links(v) {
		if link.URL == url {
			n++
		}
	}
	if n >= counts[url] {
		t.Fatal("reflow kept old link rectangles", n, counts[url])
	}
}

func TestDarkPaletteFillsAndTokens(t *testing.T) {
	doc := &Document{Blocks: []Block{{Kind: KindCode, Spans: Highlight(context.Background(), "func hi() string { return \"hello\" }", "go", styles.Get("github")), Size: 25}}}
	l := mustLayout(t, testEngine(t), doc, 600, LayoutOptions{})
	if err := l.Complete(context.Background()); err != nil {
		t.Fatal(err)
	}
	dark := DarkPalette()
	img := mustRaster(t, l, View{Width: 600, Height: l.Extent() - Margin, Scale: 1}, dark)
	var fill, keyword *operation
	for i := range l.ops {
		op := &l.ops[i]
		if op.run == nil && op.asset == nil && op.fill == fillCode && fill == nil {
			fill = op
		}
		if op.run != nil && op.token.InCategory(chroma.Keyword) && keyword == nil {
			keyword = op
		}
	}
	if fill == nil || keyword == nil {
		t.Fatal("no code fill or keyword run in the layout")
	}
	if got := img.RGBAAt(fill.rect.Min.X, fill.rect.Min.Y); got != dark.Code {
		t.Fatalf("code background is %v, want %v", got, dark.Code)
	}
	if got := img.RGBAAt(0, 0); got != dark.Background {
		t.Fatalf("background is %v, want %v", got, dark.Background)
	}
	bg := dark.Code
	dist := func(c color.RGBA) int {
		d := 0
		for _, v := range [3]int{int(c.R) - int(bg.R), int(c.G) - int(bg.G), int(c.B) - int(bg.B)} {
			d += v * v
		}
		return d
	}
	var far color.RGBA
	for y := keyword.rect.Min.Y; y < keyword.rect.Max.Y; y++ {
		for x := keyword.rect.Min.X; x < keyword.rect.Max.X; x++ {
			if c := img.RGBAAt(x, y); dist(c) > dist(far) {
				far = c
			}
		}
	}
	c := dark.Syntax.Get(keyword.token).Colour
	want := color.RGBA{c.Red(), c.Green(), c.Blue(), 255}
	if far != want {
		t.Fatalf("keyword ink is %v, want %v", far, want)
	}
	lc := LightPalette().Syntax.Get(keyword.token).Colour
	if want == dark.Foreground || want == (color.RGBA{lc.Red(), lc.Green(), lc.Blue(), 255}) {
		t.Fatal("dark keyword ink is not distinct")
	}
}

func TestLineNumbersGutterAndChop(t *testing.T) {
	ctx := context.Background()
	e := testEngine(t)
	doc, err := Lines(ctx, "package main\n\nfunc main() {}\n", "go", styles.Get("github"))
	if err != nil {
		t.Fatal(err)
	}
	plain := mustLayout(t, e, doc, 800, LayoutOptions{})
	if err := plain.Complete(ctx); err != nil {
		t.Fatal(err)
	}
	textX := -1
	for _, op := range plain.ops {
		if op.run != nil {
			textX = op.rect.Min.X
			break
		}
	}
	numbered := mustLayout(t, e, doc, 800, LayoutOptions{Numbers: true})
	if err := numbered.Complete(ctx); err != nil {
		t.Fatal(err)
	}
	pal := LightPalette()
	var numbers []string
	shifted := -1
	for _, op := range numbered.ops {
		if op.run == nil {
			continue
		}
		if op.gutter {
			numbers = append(numbers, strings.TrimSpace(string(op.text)))
			if op.style&Mono == 0 || op.rect.Min.X < Margin || op.para != -1 || pal.runInk(&op) != pal.Muted {
				t.Fatalf("gutter op %+v", op.rect)
			}
			continue
		}
		if shifted < 0 {
			shifted = op.rect.Min.X
		}
	}
	if strings.Join(numbers, ",") != "1,2,3" {
		t.Fatalf("gutter numbers %v", numbers)
	}
	if shifted <= textX {
		t.Fatalf("text did not move right of the gutter: %d then %d", textX, shifted)
	}
	for _, op := range numbered.ops {
		if op.gutter && op.rect.Max.X > shifted {
			t.Fatalf("gutter op reaches into the text at %v", op.rect)
		}
	}
	// Numbers are only for Lines documents.
	md := &Document{Blocks: []Block{{Kind: KindParagraph, Spans: []Span{{Text: strings.Repeat("prose ", 300)}}}}}
	page := mustLayout(t, e, md, 800, LayoutOptions{Numbers: true, Chop: true})
	if err := page.Complete(ctx); err != nil {
		t.Fatal(err)
	}
	rows := map[int]bool{}
	for _, op := range page.ops {
		if op.gutter {
			t.Fatal("gutter on a page that is not lines")
		}
		rows[op.rect.Min.Y] = true
	}
	if len(rows) < 2 || page.ExtentX() > page.Width() {
		t.Fatal("a page that is not lines was chopped")
	}

	long := "short\n" + strings.Repeat("a long line of words that will wrap at the measure ", 8) + "\nend\n"
	doc, err = Lines(ctx, long, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	rowsOf := func(l *Layout, block int) int {
		rows := map[int]bool{}
		for _, op := range l.ops {
			if op.run != nil && !op.gutter && op.block == block {
				rows[op.rect.Min.Y] = true
			}
		}
		return len(rows)
	}
	wrapped := mustLayout(t, e, doc, 800, LayoutOptions{})
	if err := wrapped.Complete(ctx); err != nil {
		t.Fatal(err)
	}
	if rowsOf(wrapped, 1) < 2 {
		t.Fatal("long line did not wrap without chop")
	}
	chopped := mustLayout(t, e, doc, 800, LayoutOptions{Chop: true})
	if err := chopped.Complete(ctx); err != nil {
		t.Fatal(err)
	}
	if n := rowsOf(chopped, 1); n != 1 {
		t.Fatalf("chopped line on %d rows", n)
	}
	widest := 0
	for _, op := range chopped.ops {
		widest = max(widest, op.rect.Max.X)
	}
	if chopped.ExtentX() != widest || chopped.ExtentX() <= chopped.Width() || chopped.Width() != 800 {
		t.Fatalf("extentX %d widest %d width %d", chopped.ExtentX(), widest, chopped.Width())
	}
	// A 2,000-rune line lays on one row when chopped.
	doc, err = Lines(ctx, strings.Repeat("word ", 400), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	chopped = mustLayout(t, e, doc, 800, LayoutOptions{Chop: true})
	if err := chopped.Complete(ctx); err != nil {
		t.Fatal(err)
	}
	if n := rowsOf(chopped, 0); n != 1 || chopped.ExtentX() <= chopped.Width() {
		t.Fatalf("2,000-rune line on %d rows, extentX %d", n, chopped.ExtentX())
	}
}

func linesSource(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		b.WriteString("line of the file\n")
	}
	return b.String()
}

func TestLineAt(t *testing.T) {
	ctx := context.Background()
	doc, err := Lines(ctx, linesSource(300), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	l := mustLayout(t, testEngine(t), doc, 800, LayoutOptions{})
	if l.LineAt(0) != 1 {
		t.Fatalf("line at the top %d", l.LineAt(0))
	}
	if err := l.EnsureBlock(ctx, 119); err != nil {
		t.Fatal(err)
	}
	if l.Laid() != 120 {
		t.Fatalf("laid %d", l.Laid())
	}
	top := l.Starts()[119] - Margin
	if got := l.LineAt(top); got != 120 {
		t.Fatalf("line at %d is %d", top, got)
	}
	if err := l.Ensure(ctx, top+600); err != nil {
		t.Fatal(err)
	}
	if got := l.LineAt(top + 200); got <= 120 || got > 130 {
		t.Fatalf("scrolled line %d", got)
	}
	if err := l.EnsureBlock(ctx, 9999); err != nil || !l.Done() {
		t.Fatalf("past the end: %v", err)
	}
}

func TestFind(t *testing.T) {
	ctx := context.Background()
	doc := &Document{}
	for i := range 100 {
		spans := []Span{{Text: "A paragraph that is not the target."}}
		if i == 50 {
			spans = append(spans, Span{Text: " target", Href: "#target", Role: RoleAccent})
		}
		doc.Blocks = append(doc.Blocks, Block{Kind: KindParagraph, Spans: spans})
	}
	l := mustLayout(t, testEngine(t), doc, 800, LayoutOptions{})
	r, ok, err := l.Find(ctx, "#target")
	if err != nil || !ok || r.Empty() {
		t.Fatalf("find: %v %v %v", r, ok, err)
	}
	if l.Laid() != 51 || r.Min.Y < l.Starts()[50] {
		t.Fatalf("laid %d, rect %v", l.Laid(), r)
	}
	if _, ok, err := l.Find(ctx, "#nowhere"); err != nil || ok || !l.Done() {
		t.Fatalf("missing href: %v %v", ok, err)
	}
	done, cancel := context.WithCancel(ctx)
	cancel()
	if _, _, err := mustLayout(t, testEngine(t), doc, 800, LayoutOptions{}).Find(done, "#target"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled find: %v", err)
	}
}

func TestWidthOneTerminates(t *testing.T) {
	l := mustLayout(t, testEngine(t), fixtureDocument(t), 1, LayoutOptions{})
	if err := l.Complete(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !l.Done() {
		t.Fatal("not done")
	}
	// Every run with ink is at least a pixel wide. A run of only spaces at
	// the end of a line has its advance trimmed by the wrapper and is empty,
	// as in gloss, at every width.
	for _, op := range l.ops {
		if op.run == nil || op.rect.Dx() >= 1 {
			continue
		}
		runes := op.text[op.run.Runes.Offset : op.run.Runes.Offset+op.run.Runes.Count]
		if strings.TrimSpace(string(runes)) != "" || op.run.Advance != 0 {
			t.Fatalf("empty text op %v for %q", op.rect, string(runes))
		}
	}
}

func TestEmptyDocument(t *testing.T) {
	l := mustLayout(t, testEngine(t), &Document{}, 800, LayoutOptions{})
	if !l.Done() || l.Laid() != 0 || l.Extent() != 100 || len(l.Starts()) != 0 {
		t.Fatalf("done %v laid %d extent %d", l.Done(), l.Laid(), l.Extent())
	}
	pal := LightPalette()
	img := mustRaster(t, l, View{Width: 100, Height: 50, Scale: 1}, pal)
	if img.Bounds() != image.Rect(0, 0, 100, 50) {
		t.Fatalf("bounds %v", img.Bounds())
	}
	for y := range 50 {
		for x := range 100 {
			if img.RGBAAt(x, y) != pal.Background {
				t.Fatalf("pixel %d,%d is %v", x, y, img.RGBAAt(x, y))
			}
		}
	}
	if l.LineAt(0) != 1 || len(l.Links(View{Width: 100, Height: 50, Scale: 1})) != 0 {
		t.Fatal("empty document has lines or links")
	}
}

func TestViewPixelsErrors(t *testing.T) {
	ctx := context.Background()
	doc := fixtureDocument(t)
	l := mustLayout(t, testEngine(t), doc, 2000, LayoutOptions{})
	for _, v := range []View{
		{Width: 2000, Height: 1000, Scale: 1, Pixels: image.Pt(0, 10)},
		{Width: 2000, Height: 1000, Scale: 1, Pixels: image.Pt(8000, 8000)},
		{Width: 0, Height: 1000, Scale: 1},
		{Width: 2000, Height: 1000, Scale: 0},
		{Width: 2000, Height: 1000, Scale: -1},
	} {
		if img, err := l.Raster(ctx, v, RasterOptions{Palette: LightPalette()}); err == nil || img != nil {
			t.Fatalf("view %+v accepted", v)
		}
	}
	_, err := l.Raster(ctx, View{Width: 2000, Height: 1000, Scale: 1, Pixels: image.Pt(8000, 8000)}, RasterOptions{Palette: LightPalette()})
	var le *LimitError
	if !errors.As(err, &le) || le.What != "typeset raster exceeds pixel limit" || le.Limit != 32<<20 {
		t.Fatalf("pixel limit: %v", err)
	}
	v := View{Top: 100, Width: 2000, Height: 1000, Scale: 1001.0 / 2000, Pixels: image.Pt(1001, 333)}
	if v.Size() != image.Pt(1001, 333) {
		t.Fatalf("size %v", v.Size())
	}
	img := mustRaster(t, l, v, LightPalette())
	if img.Bounds().Size() != image.Pt(1001, 333) {
		t.Fatalf("raster %v", img.Bounds())
	}
	if s := (View{Width: 2000, Height: 1000, Scale: 0.5}).Size(); s != image.Pt(1000, 500) {
		t.Fatalf("scaled size %v", s)
	}
	if r := (View{Top: 10, Left: 4, Scale: 0.5}).Project(image.Rect(4, 10, 9, 15)); r != image.Rect(0, 0, 3, 3) {
		t.Fatalf("project %v", r)
	}
}

func TestOmitAnimatedAndPlaceholder(t *testing.T) {
	still := image.NewRGBA(image.Rect(0, 0, 20, 20))
	draw.Draw(still, still.Bounds(), image.NewUniform(color.RGBA{200, 50, 90, 255}), image.Point{}, draw.Src)
	doc := &Document{Blocks: []Block{{Kind: KindImage, Image: &Image{Key: "a", Still: still, Animated: true}}}}
	l := mustLayout(t, testEngine(t), doc, 400, LayoutOptions{})
	pal := LightPalette()
	v := View{Width: 400, Height: 200, Scale: 1}
	drawn, err := l.Raster(context.Background(), v, RasterOptions{Palette: pal})
	if err != nil {
		t.Fatal(err)
	}
	omitted, err := l.Raster(context.Background(), v, RasterOptions{Palette: pal, OmitAnimated: true})
	if err != nil {
		t.Fatal(err)
	}
	if drawn.RGBAAt(60, 60) != (color.RGBA{200, 50, 90, 255}) || omitted.RGBAAt(60, 60) != pal.Background {
		t.Fatalf("drawn %v omitted %v", drawn.RGBAAt(60, 60), omitted.RGBAAt(60, 60))
	}
	if p := l.Images(v); len(p) != 1 || p[0].Key != "a" || p[0].Rect != image.Rect(50, 50, 70, 70) {
		t.Fatalf("placed %+v", p)
	}
}

func TestGuard(t *testing.T) {
	l := mustLayout(t, testEngine(t), &Document{}, 800, LayoutOptions{})
	run := func(p any) (err error) {
		defer l.guard(context.Background(), &err)
		panic(p)
	}
	limit := &LimitError{What: "x"}
	if err := run(limit); err != limit {
		t.Fatalf("limit: %v", err)
	}
	if err := run(context.Canceled); err != context.Canceled {
		t.Fatalf("canceled: %v", err)
	}
	if err := run(context.DeadlineExceeded); err != context.DeadlineExceeded {
		t.Fatalf("deadline: %v", err)
	}
	if err := run("boom"); err == nil || err.Error() != "nttypeset: boom" {
		t.Fatalf("string: %v", err)
	}
	if err := run(errors.New("other")); err == nil || err.Error() != "nttypeset: other" {
		t.Fatalf("error: %v", err)
	}
}

func TestRasterEmLimit(t *testing.T) {
	ctx := context.Background()
	// 200 layout pixels at scale 40 is 8,000 raster pixels per em.
	doc := &Document{Blocks: []Block{{Kind: KindParagraph, Spans: []Span{{Text: "Hi"}}, Size: 200}}}
	l := mustLayout(t, testEngine(t), doc, 800, LayoutOptions{})
	big := View{Top: 50, Left: 50, Width: 40, Height: 40, Scale: 40}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	_, err := l.Raster(ctx, big, RasterOptions{Palette: LightPalette()})
	runtime.ReadMemStats(&after)
	var le *LimitError
	if !errors.Is(err, ErrLimit) || !errors.As(err, &le) || le.What != "typeset glyph size exceeds 4,096 pixels" || le.Limit != 4096 {
		t.Fatalf("err = %v", err)
	}
	// A 1600×1600 raster would be 10 MB; nothing near that was allocated.
	if n := after.TotalAlloc - before.TotalAlloc; n > 1<<20 {
		t.Fatalf("allocated %d bytes before refusing", n)
	}
	if _, err := l.Raster(ctx, View{Top: 50, Left: 50, Width: 40, Height: 40, Scale: 1}, RasterOptions{Palette: LightPalette()}); err != nil {
		t.Fatalf("scale 1: %v", err)
	}
	// A lowered limit names itself.
	e, err := NewEngine(WithFonts(aliasSource{}), WithLimits(Limits{Em: 100}))
	if err != nil {
		t.Fatal(err)
	}
	small := &Document{Blocks: []Block{{Kind: KindParagraph, Spans: []Span{{Text: "Hi"}}}}}
	_, err = mustLayout(t, e, small, 800, LayoutOptions{}).Raster(ctx, View{Width: 400, Height: 100, Scale: 5}, RasterOptions{Palette: LightPalette()})
	if !errors.As(err, &le) || le.What != "typeset glyph size exceeds 100 pixels" || le.Limit != 100 {
		t.Fatalf("lowered: %v", err)
	}
}

func TestBlockSizeLimit(t *testing.T) {
	ctx := context.Background()
	ok := &Document{Blocks: []Block{{Kind: KindParagraph, Spans: []Span{{Text: "Hi"}}, Size: 1024}}}
	if err := mustLayout(t, testEngine(t), ok, 800, LayoutOptions{}).Complete(ctx); err != nil {
		t.Fatalf("size 1024: %v", err)
	}
	doc := &Document{Blocks: []Block{{Kind: KindParagraph, Spans: []Span{{Text: "Hi"}}, Size: 1025}}}
	l := mustLayout(t, testEngine(t), doc, 800, LayoutOptions{})
	err := l.Complete(ctx)
	var le *LimitError
	if !errors.As(err, &le) || le.What != "typeset block size exceeds 1,024 pixels" || le.Limit != 1024 {
		t.Fatalf("err = %v", err)
	}
	if l.Laid() != 0 {
		t.Fatalf("laid %d blocks", l.Laid())
	}
	for _, size := range []int{-3, math.MinInt} {
		doc := &Document{Blocks: []Block{{Kind: KindParagraph, Spans: []Span{{Text: "Hi"}}, Size: size}}}
		err := mustLayout(t, testEngine(t), doc, 800, LayoutOptions{}).Complete(ctx)
		if !errors.Is(err, ErrLimit) || !errors.As(err, &le) || le.What != "typeset block size is negative" || le.Limit != 0 {
			t.Fatalf("size %d: err = %v", size, err)
		}
	}
}

func TestDeadlineLimitIsNanoseconds(t *testing.T) {
	e := testEngine(t)
	e.limits.Deadline = time.Nanosecond
	doc := &Document{Blocks: []Block{{Kind: KindParagraph, Spans: []Span{{Text: "one"}}}, {Kind: KindParagraph, Spans: []Span{{Text: "two"}}}}}
	err := mustLayout(t, e, doc, 800, LayoutOptions{}).Complete(context.Background())
	var le *LimitError
	if !errors.Is(err, ErrLimit) || !errors.As(err, &le) || le.What != "typeset layout exceeds 0.000000001 seconds" {
		t.Fatalf("err = %v", err)
	}
	if want := int64(time.Nanosecond); le.Limit != want {
		t.Fatalf("Limit = %d, want %d", le.Limit, want)
	}
}

func TestTypedNilStillIsPlaceholder(t *testing.T) {
	var frame *image.RGBA
	doc := &Document{Blocks: []Block{{Kind: KindImage, Image: &Image{Key: "nil", Still: frame}, Spans: []Span{{Text: "[image unavailable]"}}}}}
	l := mustLayout(t, testEngine(t), doc, 800, LayoutOptions{})
	if err := l.Complete(context.Background()); err != nil {
		t.Fatal(err)
	}
	v := View{Width: 800, Height: 200, Scale: 1}
	if p := l.Images(v); len(p) != 0 {
		t.Fatalf("placed %+v", p)
	}
	if _, err := l.Raster(context.Background(), v, RasterOptions{}); err != nil {
		t.Fatal(err)
	}
	if len(l.ops) == 0 || l.ops[0].run == nil || string(l.ops[0].text) != "[image unavailable]" {
		t.Fatal("placeholder text not laid")
	}
}

func TestZeroSizeStillIsPlaceholder(t *testing.T) {
	doc := &Document{Blocks: []Block{{Kind: KindImage, Image: &Image{Key: "empty", Still: image.NewRGBA(image.Rect(0, 0, 0, 0))}, Spans: []Span{{Text: "[image unavailable]"}}}}}
	l := mustLayout(t, testEngine(t), doc, 800, LayoutOptions{})
	if err := l.Complete(context.Background()); err != nil {
		t.Fatal(err)
	}
	v := View{Width: 800, Height: 200, Scale: 1}
	if p := l.Images(v); len(p) != 0 {
		t.Fatalf("placed %+v", p)
	}
	if len(l.ops) == 0 || l.ops[0].run == nil || string(l.ops[0].text) != "[image unavailable]" {
		t.Fatal("placeholder text not laid")
	}
}

func TestFindEmptyHref(t *testing.T) {
	doc := &Document{Blocks: []Block{
		{Kind: KindParagraph, Spans: []Span{{Text: "hello"}}},
		{Kind: KindParagraph, Spans: []Span{{Text: "x", Href: "#a"}}},
	}}
	l := mustLayout(t, testEngine(t), doc, 800, LayoutOptions{})
	r, ok, err := l.Find(context.Background(), "")
	if r != (image.Rectangle{}) || ok || err != nil || l.Laid() != 0 {
		t.Fatalf("Find(\"\") = %v %v %v, laid %d", r, ok, err, l.Laid())
	}
}

func TestTextReusesShapingWithCallerHrefs(t *testing.T) {
	ctx := context.Background()
	e := testEngine(t)
	layoutWith := func(href string) []Link {
		doc := &Document{Blocks: []Block{{Kind: KindParagraph, Spans: []Span{{Text: "same text", Href: href, Role: RoleAccent}}}}}
		l := mustLayout(t, e, doc, 400, LayoutOptions{})
		v := View{Width: 400, Height: 200, Scale: 1}
		if err := l.Ensure(ctx, v.Top+v.Height); err != nil {
			t.Fatal(err)
		}
		return l.Links(v)
	}
	first := layoutWith("https://one.example/")
	entries := len(e.cache)
	second := layoutWith("https://two.example/")
	if len(e.cache) != entries {
		t.Fatalf("second layout missed the shape cache: %d entries, was %d", len(e.cache), entries)
	}
	if len(first) == 0 || len(second) == 0 {
		t.Fatal("no links")
	}
	for _, link := range first {
		if link.URL != "https://one.example/" {
			t.Fatal("cache reused wrong link", link.URL)
		}
	}
	for _, link := range second {
		if link.URL != "https://two.example/" {
			t.Fatal("cache reused wrong link", link.URL)
		}
	}
}
