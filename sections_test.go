package nttypeset

import (
	"context"
	"errors"
	"image"
	"image/color"
	"reflect"
	"strings"
	"testing"
)

func sectionDocument() *Document {
	blocks := make([]Block, 20)
	for i := range blocks {
		blocks[i] = Block{Size: 16, Format: &BlockFormat{LineHeight: 24, SpaceAfter: 8}, Spans: []Span{{Text: strings.Repeat("Readable content with stable identity. ", 3)}}}
	}
	blocks[1].SourceID = "paragraph"
	return &Document{Blocks: blocks, Sections: []Section{
		{Start: 0, End: 20, SourceID: "outer", Padding: Insets{Top: 10, Right: 10, Bottom: 10, Left: 10}, Background: color.RGBA{30, 40, 50, 255}, Top: Border{2, color.RGBA{200, 0, 0, 255}}, Bottom: Border{2, color.RGBA{200, 0, 0, 255}}, Left: Border{4, color.RGBA{200, 0, 0, 255}}},
		{Start: 1, End: 3, SourceID: "inner", Padding: Insets{Top: 8, Right: 8, Bottom: 8, Left: 8}, Background: color.RGBA{50, 60, 70, 255}, Left: Border{3, color.RGBA{0, 200, 0, 255}}},
	}}
}

func TestSectionsProgressiveClipping(t *testing.T) {
	ctx := context.Background()
	doc := sectionDocument()
	l := mustLayout(t, testEngine(t), doc, 420, LayoutOptions{})
	first := mustRaster(t, l, View{Width: 420, Height: 150, Scale: 1}, LightPalette())
	if l.Done() || l.Laid() > 3 {
		t.Fatalf("viewport forced %d blocks", l.Laid())
	}
	for _, p := range []struct {
		x, y int
		c    color.RGBA
	}{{60, 50, doc.Sections[0].Top.Color}, {50, 80, doc.Sections[0].Left.Color}, {55, 80, doc.Sections[0].Background}} {
		if got := first.RGBAAt(p.x, p.y); got != p.c {
			t.Errorf("pixel (%d,%d): %v want %v", p.x, p.y, got, p.c)
		}
	}
	partial, found, err := l.AnchorBounds(ctx, "outer", false)
	if err != nil || !found || partial.Complete {
		t.Fatalf("partial: %+v %v %v", partial, found, err)
	}
	// Compare a scrolled crop to the same pixels in a full render, including
	// nested side borders. Neither image is used as a golden reference.
	view := View{Top: 140, Left: 45, Width: 320, Height: 140, Scale: 1}
	crop := mustRaster(t, l, view, LightPalette())
	if err := l.Complete(ctx); err != nil {
		t.Fatal(err)
	}
	full := mustRaster(t, l, View{Width: 420, Height: l.Extent(), Scale: 1}, LightPalette())
	for y := 0; y < 140; y++ {
		for x := 0; x < 320; x++ {
			if crop.RGBAAt(x, y) != full.RGBAAt(x+45, y+140) {
				t.Fatalf("crop mismatch at %d,%d", x, y)
			}
		}
	}
	a, _, _ := l.AnchorBounds(ctx, "outer", false)
	if !a.Complete || full.RGBAAt(60, a.Rect.Max.Y-1) != doc.Sections[0].Bottom.Color {
		t.Fatal("missing final edge")
	}
	inner, _, _ := l.AnchorBounds(ctx, "inner", false)
	if full.RGBAAt(inner.Rect.Min.X, inner.Rect.Min.Y+1) != doc.Sections[1].Left.Color {
		t.Fatal("missing nested border")
	}
}

func TestSourceIdentityReflow(t *testing.T) {
	ctx := context.Background()
	doc := sectionDocument()
	e := testEngine(t)
	var heights []int
	for _, width := range []int{320, 600} {
		l := mustLayout(t, e, doc, width, LayoutOptions{})
		a, found, err := l.AnchorBounds(ctx, "paragraph", false)
		if err != nil || !found || a.Complete || !a.Rect.Empty() {
			t.Fatal("unlaid anchor", a, err)
		}
		if _, found, err = l.AnchorBounds(ctx, "hidden", true); err != nil || found || l.Laid() != 0 {
			t.Fatal("absent anchor caused layout")
		}
		a, _, err = l.AnchorBounds(ctx, "paragraph", true)
		if err != nil || !a.Complete || l.Laid() != 2 {
			t.Fatal("ensure anchor", a, err)
		}
		heights = append(heights, a.Rect.Dy())
		p := Place{Block: 1, Para: 0, Rune: 5, Placed: true}
		id, ok, err := l.SourceAt(ctx, p)
		if err != nil || !ok || id != "paragraph" {
			t.Fatal(id, ok, err)
		}
		sel, err := l.Select(ctx, Selection{Anchor: p, Focus: Place{Block: 1, Para: 0, Rune: 12, Placed: true}})
		if err != nil || sel.Text() != "ble con" {
			t.Fatalf("selection %q %v", sel.Text(), err)
		}
		if id, _, _ := l.SourceAt(ctx, sel.Places().Anchor); id != "paragraph" {
			t.Fatal(id)
		}
		if id, _, _ := l.SourceAt(ctx, Place{Block: 2, Placed: true}); id != "inner" {
			t.Fatal(id)
		}
		for _, op := range l.ops {
			if op.run != nil && op.block == 1 {
				hit, ok := l.TextAt(image.Pt(op.rect.Min.X+2, op.rect.Min.Y+2))
				if !ok {
					t.Fatal("no hit")
				}
				id, _, _ := l.SourceAt(ctx, Place{Block: hit.Block, Para: hit.Para, Rune: hit.Rune, Placed: true})
				if id != "paragraph" {
					t.Fatal(id)
				}
				break
			}
		}
	}
	if heights[0] <= heights[1] {
		t.Fatal("no reflow", heights)
	}
}

func TestSectionValidationAndRollback(t *testing.T) {
	ctx := context.Background()
	e := testEngine(t)
	cases := []*Document{
		{Blocks: make([]Block, 3), Sections: []Section{{Start: 0, End: 2}, {Start: 1, End: 3}}},
		{Blocks: make([]Block, 1), Sections: []Section{{Start: 0, End: 0}}},
		{Blocks: []Block{{SourceID: "duplicate"}}, Sections: []Section{{Start: 0, End: 1, SourceID: "duplicate"}}},
		{Blocks: []Block{{Format: &BlockFormat{LineHeight: -1}}}},
	}
	for _, doc := range cases {
		if _, err := e.Layout(doc, 400, LayoutOptions{}); err == nil {
			t.Fatal("accepted invalid document")
		}
	}
	doc := sectionDocument()
	for range 65 {
		doc.Sections = append(doc.Sections, Section{Start: 0, End: 20})
	}
	if _, err := e.Layout(doc, 400, LayoutOptions{}); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
	l := mustLayout(t, e, sectionDocument(), 400, LayoutOptions{})
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := l.Ensure(cancelled, 200); err != context.Canceled {
		t.Fatal(err)
	}
	if l.Laid() != 0 || len(l.active) != 0 || len(l.ops) != 0 {
		t.Fatal("cancelled layout mutated")
	}
	if err := l.Ensure(ctx, 200); err != nil {
		t.Fatal(err)
	}
	// Force a failure after sections were opened and decoration was emitted.
	e2 := testEngine(t)
	e2.limits.Operations = 1
	l2 := mustLayout(t, e2, sectionDocument(), 400, LayoutOptions{})
	if err := l2.Ensure(ctx, 200); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
	if l2.Laid() != 0 || l2.y != Margin || len(l2.active) != 0 || !l2.sectionBounds[0].Empty() {
		t.Fatal("partial section state escaped rollback")
	}
}

func TestZeroSectionCompatibility(t *testing.T) {
	e := testEngine(t)
	doc := fixtureDocument(t)
	old := mustLayout(t, e, doc, 500, LayoutOptions{})
	baseline := mustRaster(t, old, View{Width: 500, Height: 600, Scale: 1}, LightPalette())
	doc2 := *doc
	doc2.Sections = []Section{{Start: 0, End: len(doc.Blocks)}}
	newer := mustLayout(t, e, &doc2, 500, LayoutOptions{})
	got := mustRaster(t, newer, View{Width: 500, Height: 600, Scale: 1}, LightPalette())
	if !reflect.DeepEqual(got.Pix, baseline.Pix) || !reflect.DeepEqual(old.Starts(), newer.Starts()) {
		t.Fatal("zero decoration changed output")
	}
}

func TestResolvedBlockFormat(t *testing.T) {
	ctx := context.Background()
	e := testEngine(t)
	bg := color.RGBA{15, 25, 35, 255}
	ink := color.RGBA{200, 210, 220, 255}
	doc := &Document{Blocks: []Block{{Kind: KindCode, Size: 16, Spans: []Span{{Text: "one\ntwo", Ink: ink}}, Format: &BlockFormat{LineHeight: 24, SpaceBefore: 8, SpaceAfter: 7, Padding: Insets{Top: 3, Right: 4, Bottom: 5, Left: 6}, Background: bg}}}}
	l := mustLayout(t, e, doc, 400, LayoutOptions{})
	im := mustRaster(t, l, View{Width: 400, Height: 150, Scale: 1}, LightPalette())
	if l.y != Margin+8+3+48+5+7 {
		t.Fatal("resolved height", l.y)
	}
	if im.RGBAAt(50, 58) != bg || im.RGBAAt(50, 57) != LightPalette().Background || im.RGBAAt(50, l.y-1) != LightPalette().Background {
		t.Fatal("resolved background includes spacing")
	}
	for _, op := range l.ops {
		if op.run != nil && (op.size != 16 || op.ink != ink || op.rect.Min.X != 56) {
			t.Fatal("resolved text lost", op)
		}
	}
	if _, _, err := l.AnchorBounds(ctx, "", true); err != nil {
		t.Fatal(err)
	}
}

func TestSectionOutsideSpacingAndEmptySource(t *testing.T) {
	ctx := context.Background()
	e := testEngine(t)
	red := color.RGBA{255, 0, 0, 255}
	green := color.RGBA{0, 255, 0, 255}
	doc := &Document{Blocks: []Block{{SourceID: "empty", Format: &BlockFormat{LineHeight: 24}}}, Sections: []Section{
		{Start: 0, End: 1, SourceID: "outer", Background: red},
		{Start: 0, End: 1, SourceID: "inner", Background: green, SpaceBefore: 7, SpaceAfter: 9},
	}}
	l := mustLayout(t, e, doc, 300, LayoutOptions{})
	im := mustRaster(t, l, View{Width: 300, Height: 150, Scale: 1}, LightPalette())
	a, _, _ := l.AnchorBounds(ctx, "inner", false)
	if a.Rect.Min.Y != Margin+7 || a.Rect.Max.Y != Margin+7+24 || l.y != Margin+7+24+9 {
		t.Fatal(a, l.y)
	}
	if im.RGBAAt(60, Margin) != red || im.RGBAAt(60, Margin+7) != green || im.RGBAAt(60, a.Rect.Max.Y) != red {
		t.Fatal("outside spacing painted")
	}
	if id, ok, err := l.SourceAt(ctx, Place{Block: 0, Placed: true}); err != nil || !ok || id != "empty" {
		t.Fatal(id, ok, err)
	}
	// All geometry-producing additions reject layouts invalidated by font changes.
	e.fontGeneration++
	if _, _, err := l.SourceAt(ctx, Place{}); err != errStale {
		t.Fatal(err)
	}
	if _, _, err := l.AnchorBounds(ctx, "inner", false); err != errStale {
		t.Fatal(err)
	}
}

func TestZeroSectionNarrowWidth(t *testing.T) {
	e := testEngine(t)
	doc := &Document{Blocks: []Block{{Spans: []Span{{Text: "narrow width content"}}}}}
	plain := mustLayout(t, e, doc, 90, LayoutOptions{})
	a := mustRaster(t, plain, View{Width: 90, Height: 250, Scale: 1}, LightPalette())
	doc2 := *doc
	doc2.Sections = []Section{{Start: 0, End: 1}}
	decorated := mustLayout(t, e, &doc2, 90, LayoutOptions{})
	b := mustRaster(t, decorated, View{Width: 90, Height: 250, Scale: 1}, LightPalette())
	if !reflect.DeepEqual(a.Pix, b.Pix) || plain.Extent() != decorated.Extent() {
		t.Fatal("zero section altered narrow layout")
	}
}
