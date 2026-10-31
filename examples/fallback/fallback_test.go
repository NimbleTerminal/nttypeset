package fallback_test

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"nimbleterminal.dev/ntfonts"
	"nimbleterminal.dev/nttypeset"
)

// The published v0.2.0 source has the package function but not the method.
// This adapter exercises the optional contract without a sibling replacement.
type symbolSource struct{ ntfonts.Source }

func (symbolSource) Fallbacks() []string { return ntfonts.Fallbacks() }

var _ nttypeset.FallbackSource = symbolSource{}

func TestEmbeddedSymbols(t *testing.T) {
	for _, style := range []nttypeset.Style{0, nttypeset.Bold, nttypeset.Italic, nttypeset.Bold | nttypeset.Italic, nttypeset.Mono} {
		e, err := nttypeset.NewEngine(nttypeset.WithFonts(symbolSource{}))
		if err != nil {
			t.Fatal(err)
		}
		rasterText(t, e, "Check ✓ ✗. Arrows → ⇒ ↔. Shapes ■ ▲ ●.", style)
		if got := e.Missing(); len(got) != 0 {
			t.Fatalf("style %v missing=%U", style, got)
		}
		want := []string{"Noto Sans Symbols 2"}
		if style&nttypeset.Mono != 0 {
			want = nil
		}
		if got := e.Fallbacks(); !slices.Equal(got, want) {
			t.Fatalf("style %v fallbacks=%v, want %v", style, got, want)
		}
		// Repeat through the shape and glyph caches, then invalidate with a new look.
		rasterText(t, e, "Check ✓ ✗. Arrows → ⇒ ↔. Shapes ■ ▲ ●.", style)
		if _, err := e.SetFonts("technical", ""); err != nil {
			t.Fatal(err)
		}
		if len(e.Fallbacks()) != 0 || len(e.Missing()) != 0 {
			t.Fatal("look change retained fallback reports")
		}
		rasterText(t, e, "漢", style)
		if got := e.Missing(); !slices.Equal(got, []rune{'漢'}) {
			t.Fatalf("missing=%U", got)
		}
	}
}

func TestSourceWithoutFallbackMethod(t *testing.T) {
	// Hide optional methods even after the published font dependency is updated.
	source := struct{ nttypeset.FaceSource }{ntfonts.Embedded}
	e, err := nttypeset.NewEngine(nttypeset.WithFonts(source))
	if err != nil {
		t.Fatal(err)
	}
	rasterText(t, e, "✓ ✗ ⇒ ↔ ●", 0)
	want := []rune("✓✗⇒↔●")
	slices.Sort(want)
	if got := e.Missing(); !slices.Equal(got, want) {
		t.Fatalf("missing=%U, want %U", got, want)
	}
	if len(e.Fallbacks()) != 0 {
		t.Fatalf("fallbacks=%v", e.Fallbacks())
	}
}

func rasterText(t *testing.T, e *nttypeset.Engine, text string, style nttypeset.Style) {
	t.Helper()
	doc := &nttypeset.Document{Blocks: []nttypeset.Block{{Spans: []nttypeset.Span{{Text: text, Style: style}}}}}
	lay, err := e.Layout(doc, 600, nttypeset.LayoutOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lay.Raster(context.Background(), nttypeset.View{Width: 600, Height: 200, Scale: 1}, nttypeset.RasterOptions{Palette: nttypeset.LightPalette()}); err != nil {
		t.Fatal(err)
	}
}

// Chain sources deliberately alias families to independently known font bytes.
type chainSource struct{ calls []string }

func (*chainSource) Fallbacks() []string { return []string{"broken", "text-only", "symbols", "unused"} }
func (s *chainSource) Face(family, variant string) ([]byte, error) {
	s.calls = append(s.calls, family+"/"+variant)
	switch family {
	case "broken":
		return nil, fmt.Errorf("unavailable")
	case "text-only":
		family = "Literata"
	case "symbols", "unused":
		family = "Noto Sans Symbols 2"
	// Force the technical mono primary to miss checkmarks, then try Iosevka.
	case "IBM Plex Mono":
		family = "Literata"
	}
	return ntfonts.Embedded.Face(family, variant)
}

func TestOrderedChainAndMonoBuiltin(t *testing.T) {
	src := &chainSource{}
	e, err := nttypeset.NewEngine(nttypeset.WithFonts(src))
	if err != nil {
		t.Fatal(err)
	}
	src.calls = nil
	rasterText(t, e, "✓ ✗ ⇒ ↔ ●", nttypeset.Bold)
	wantCalls := []string{"Literata/Bold", "broken/Bold", "text-only/Bold", "symbols/Bold"}
	if !slices.Equal(src.calls, wantCalls) {
		t.Fatalf("calls=%v, want %v", src.calls, wantCalls)
	}
	if !slices.Equal(e.Fallbacks(), []string{"symbols"}) || len(e.Missing()) != 0 {
		t.Fatalf("fallbacks=%v missing=%U", e.Fallbacks(), e.Missing())
	}
	if _, err := e.SetFonts("technical", ""); err != nil {
		t.Fatal(err)
	}
	src.calls = nil
	rasterText(t, e, "✓ ✗ ⇒ ↔ ●", nttypeset.Mono)
	if !slices.Equal(src.calls, []string{"Iosevka/Regular"}) {
		t.Fatalf("mono calls=%v", src.calls)
	}
	if !slices.Equal(e.Fallbacks(), []string{"Iosevka"}) || len(e.Missing()) != 0 {
		t.Fatalf("mono fallbacks=%v missing=%U", e.Fallbacks(), e.Missing())
	}
}
