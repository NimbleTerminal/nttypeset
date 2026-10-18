package nttypeset

import (
	"context"
	"errors"
	"image"
	"image/color"
	"slices"
	"strings"
	"testing"

	"nimbleterminal.dev/nttypeset/internal/testfonts"
)

// aliasSource serves the Go fonts under the families of the built-in readable
// and technical looks, so those looks resolve in tests. It has no family for
// the editorial and compact looks, so switching to them fails to load.
type aliasSource struct{}

func (aliasSource) Face(family, variant string) ([]byte, error) {
	switch family {
	case "Literata", "IBM Plex Sans":
		family = testfonts.Body
	case "Iosevka", "IBM Plex Mono":
		family = testfonts.Code
	}
	return testfonts.Source.Face(family, variant)
}

func testEngine(t *testing.T) *Engine {
	t.Helper()
	e, err := NewEngine(WithFonts(aliasSource{}))
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestNewEngineRequiresFonts(t *testing.T) {
	_, err := NewEngine()
	if err == nil || err.Error() != "nttypeset: WithFonts is required" {
		t.Fatalf("err = %v", err)
	}
	if _, err := NewEngine(WithFonts(testfonts.Source)); err == nil {
		t.Fatal("default readable look should fail on a source without Literata")
	}
	if _, err := NewEngine(WithFonts(aliasSource{}), WithLook("nope", "")); err == nil {
		t.Fatal("unknown look accepted")
	}
	e := testEngine(t)
	if e.Look().Name != "readable" || e.FontKey() != "readable/" {
		t.Fatalf("look = %+v key = %q", e.Look(), e.FontKey())
	}
}

func TestNewEngineLimits(t *testing.T) {
	e, err := NewEngine(WithFonts(aliasSource{}),
		WithLimits(Limits{Runes: -1, Glyphs: 10, ShapeCache: 1 << 40}))
	if err != nil {
		t.Fatal(err)
	}
	if e.limits.Runes != 16384 || e.limits.Glyphs != 10 || e.limits.ShapeCache != 32<<20 {
		t.Fatalf("limits = %+v", e.limits)
	}
}

func TestSetFontsClearsCaches(t *testing.T) {
	e := testEngine(t)
	s := e.shape([]Span{{Text: "hello"}}, 25)
	if len(e.cache) == 0 {
		t.Fatal("nothing cached")
	}
	e.rasterizer.Draw(image.NewRGBA(image.Rect(0, 0, 200, 60)), &s.runs[0], 25, 1, 0, 40, 40, color.RGBA{A: 255})
	if e.GlyphCache().Entries == 0 {
		t.Fatal("nothing rasterized")
	}
	changed, err := e.SetFonts("readable", "")
	if err != nil || changed {
		t.Fatalf("same look: changed=%v err=%v", changed, err)
	}
	if len(e.cache) == 0 {
		t.Fatal("unchanged look cleared the cache")
	}
	changed, err = e.SetFonts("technical", "")
	if err != nil || !changed {
		t.Fatalf("new look: changed=%v err=%v", changed, err)
	}
	if len(e.cache) != 0 || e.cacheBytes != 0 || e.GlyphCache().Entries != 0 {
		t.Fatal("caches not cleared")
	}
	if e.FontKey() != "technical/" || e.Look().Name != "technical" {
		t.Fatalf("look = %+v", e.Look())
	}
	changed, err = e.SetFonts("technical", "")
	if err != nil || changed {
		t.Fatalf("repeat: changed=%v err=%v", changed, err)
	}
	// An error leaves the engine as it was.
	if _, err := e.SetFonts("nope", ""); err == nil {
		t.Fatal("unknown look accepted")
	}
	if e.FontKey() != "technical/" {
		t.Fatalf("key after error = %q", e.FontKey())
	}
	// A look whose faces fail to load also leaves it as it was.
	if _, err := e.SetFonts("compact", ""); err == nil {
		t.Fatal("unloadable look accepted")
	}
	if e.FontKey() != "technical/" || e.Look().Body != "IBM Plex Sans" {
		t.Fatalf("state after load error: %q", e.FontKey())
	}
}

func TestShapeCacheReuseAndKeys(t *testing.T) {
	e := testEngine(t)
	spans := []Span{{Text: "Café naïve "}, {Text: "résumé ", Style: Bold}, {Text: "élan and", Style: Italic}}
	s := e.shape(spans, 25)
	for _, run := range s.runs {
		for _, g := range run.Glyphs {
			if g.GlyphID == 0 {
				t.Fatal("missing Latin glyph")
			}
		}
	}
	if len(s.text) != len([]rune(spans[0].Text+spans[1].Text+spans[2].Text)) || len(s.styles) != len(s.text) ||
		len(s.tokens) != len(s.text) || len(s.inks) != len(s.text) || len(s.roles) != len(s.text) || s.size != 25 {
		t.Fatal("per-rune slices out of step")
	}
	before := len(e.cache)
	if e.shape(spans, 25) != s || len(e.cache) != before {
		t.Fatal("identical shaping was not reused")
	}
	e.shape([]Span{{Text: "changed"}}, 25)
	if len(e.cache) <= before {
		t.Fatal("different text was not cached separately")
	}
	// Href is not part of shaping.
	linked := []Span{{Text: "x", Href: "a"}}
	if e.shape(linked, 25) != e.shape([]Span{{Text: "x", Href: "b"}}, 25) {
		t.Fatal("Href split the cache")
	}
	// Ink, Token, Role, Style and size are.
	base := e.shape(linked, 25)
	tinted := e.shape([]Span{{Text: "x", Ink: color.RGBA{1, 2, 3, 255}}}, 25)
	if base == tinted {
		t.Fatal("span colors collided in shape cache")
	}
	if base == e.shape(linked, 26) || base == e.shape([]Span{{Text: "x", Role: RoleAccent}}, 25) ||
		base == e.shape([]Span{{Text: "x", Token: 1}}, 25) {
		t.Fatal("size, role or token collided in shape cache")
	}
}

func TestShapeCacheEvicts(t *testing.T) {
	e, err := NewEngine(WithFonts(aliasSource{}), WithLimits(Limits{ShapeCache: 4096}))
	if err != nil {
		t.Fatal(err)
	}
	for i := range 50 {
		e.shape([]Span{{Text: strings.Repeat("a", 10+i)}}, 25)
		if e.cacheBytes > 4096 {
			t.Fatalf("cacheBytes = %d over the budget", e.cacheBytes)
		}
	}
}

func TestShapeRuneLimit(t *testing.T) {
	e := testEngine(t)
	if _, err := e.shapeForTest([]Span{{Text: strings.Repeat("a", 16384)}}, 25); err != nil {
		t.Fatal(err)
	}
	_, err := e.shapeForTest([]Span{{Text: strings.Repeat("é", 16385)}}, 25)
	var le *LimitError
	if !errors.As(err, &le) || !errors.Is(err, ErrLimit) {
		t.Fatalf("err = %v", err)
	}
	if le.What != "typeset paragraph exceeds 16,384 runes" || le.Limit != 16384 {
		t.Fatalf("limit error = %+v", le)
	}
}

func TestEngineCheckAndEnter(t *testing.T) {
	e := testEngine(t)
	e.check() // no context: no panic
	ctx, cancel := context.WithCancel(context.Background())
	restore := e.enter(ctx)
	e.check()
	cancel()
	if _, err := e.shapeForTest([]Span{{Text: "x"}}, 25); !errors.Is(err, ctx.Err()) {
		t.Fatalf("err = %v", err)
	}
	restore()
	if e.ctx != nil {
		t.Fatal("enter did not restore the context")
	}
	e.check()
}

func TestFontFallbackMissingAndLigatures(t *testing.T) {
	e := testEngine(t)
	if len(e.Missing()) != 0 || len(e.Fallbacks()) != 0 {
		t.Fatal("fresh engine reports missing or fallback")
	}
	e.shape([]Span{{Text: "🙂漢"}}, 25)
	if got := e.Missing(); len(got) != 2 || !slices.IsSorted(got) || got[0] != '漢' || got[1] != '🙂' {
		t.Fatalf("Missing = %q", got)
	}
	if len(e.Fallbacks()) != 0 {
		t.Fatalf("Fallbacks = %v", e.Fallbacks())
	}
	s := e.shape([]Span{{Text: "ffi"}}, 25)
	n := 0
	for _, run := range s.runs {
		n += len(run.Glyphs)
	}
	if n != 3 {
		t.Fatal("ligature enabled", n)
	}
}

func TestFallbacksSorted(t *testing.T) {
	e := testEngine(t)
	e.faces.fallbackUsed["Iosevka"] = true
	e.faces.fallbackUsed["Literata"] = true
	if got := e.Fallbacks(); !slices.Equal(got, []string{"Iosevka", "Literata"}) {
		t.Fatalf("Fallbacks = %v", got)
	}
}

func TestSetGlyphBudget(t *testing.T) {
	e := testEngine(t)
	e.SetGlyphBudget(1234)
	if e.GlyphCache().Budget != 1234 {
		t.Fatalf("stats = %+v", e.GlyphCache())
	}
}

func TestSetGlyphBudgetClamps(t *testing.T) {
	e, err := NewEngine(WithFonts(aliasSource{}), WithLimits(Limits{GlyphCache: 4096}))
	if err != nil {
		t.Fatal(err)
	}
	e.SetGlyphBudget(1 << 30)
	if e.GlyphCache().Budget != 4096 {
		t.Fatalf("budget = %d, want the 4096 limit", e.GlyphCache().Budget)
	}
	e.SetGlyphBudget(0)
	if e.GlyphCache().Budget != 4096 {
		t.Fatalf("zero budget changed it: %d", e.GlyphCache().Budget)
	}
}

func TestZeroEngineAccessors(t *testing.T) {
	var e Engine
	if e.Fallbacks() != nil || e.Missing() != nil {
		t.Fatal("zero Engine accessors returned values")
	}
}

func TestSetFontsInvalidatesLayouts(t *testing.T) {
	ctx := context.Background()
	e := testEngine(t)
	doc := &Document{Blocks: []Block{{Kind: KindParagraph, Spans: []Span{{Text: "hello", Href: "#a"}}}}}
	l := mustLayout(t, e, doc, 400, LayoutOptions{})
	if err := l.Ensure(ctx, 100); err != nil {
		t.Fatal(err)
	}
	if changed, err := e.SetFonts("technical", ""); err != nil || !changed {
		t.Fatalf("SetFonts = %v, %v", changed, err)
	}
	const want = "nttypeset: layout predates a font change"
	if err := l.Ensure(ctx, 100); err == nil || err.Error() != want {
		t.Fatalf("Ensure = %v", err)
	}
	if err := l.EnsureBlock(ctx, 0); err == nil || err.Error() != want {
		t.Fatalf("EnsureBlock = %v", err)
	}
	if err := l.Complete(ctx); err == nil || err.Error() != want {
		t.Fatalf("Complete = %v", err)
	}
	if _, _, err := l.Find(ctx, "#a"); err == nil || err.Error() != want {
		t.Fatalf("Find = %v", err)
	}
	if _, err := l.Raster(ctx, View{Width: 100, Height: 100, Scale: 1}, RasterOptions{}); err == nil || err.Error() != want {
		t.Fatalf("Raster = %v", err)
	}
	if _, err := l.Select(ctx, Selection{}); err == nil || err.Error() != want {
		t.Fatalf("Select = %v", err)
	}
	// A failed or unchanged SetFonts keeps layouts valid.
	fresh := mustLayout(t, e, doc, 400, LayoutOptions{})
	_, _ = e.SetFonts("technical", "")
	_, _ = e.SetFonts("compact", "")
	if err := fresh.Ensure(ctx, 100); err != nil {
		t.Fatalf("fresh Ensure = %v", err)
	}
}
