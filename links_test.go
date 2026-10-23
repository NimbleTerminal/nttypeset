package nttypeset

import (
	"context"
	"image"
	"testing"
)

// Links say which runes of which paragraph they cover: the pieces of one
// wrapped link meet, two links to one URL do not.
func TestLinkOrigins(t *testing.T) {
	e := testEngine(t)
	const u = "https://example.net/"
	doc := &Document{Blocks: []Block{
		{Kind: KindParagraph, Spans: []Span{{Text: "intro"}}},
		{Kind: KindParagraph, Spans: []Span{{Text: "see "}, {Text: "the first link here", Href: u}, {Text: " and "}, {Text: "again", Href: u}}},
	}}
	links := func(width int) []Link {
		l := mustLayout(t, e, doc, width, LayoutOptions{})
		if err := l.Complete(context.Background()); err != nil {
			t.Fatal(err)
		}
		return l.Links(View{Top: 0, Height: l.Extent(), Width: l.Width(), Scale: 1})
	}
	wide := links(2000)
	var first, second []Link
	for _, link := range wide {
		if link.Block != 1 || link.Para != 0 || link.URL != u {
			t.Fatalf("link %+v", link)
		}
		if link.Runes[0] < 23 {
			first = append(first, link)
		} else {
			second = append(second, link)
		}
	}
	if len(first) == 0 || first[0].Runes[0] != 4 || first[len(first)-1].Runes[1] != 23 || len(second) == 0 || second[0].Runes != [2]int{28, 33} {
		t.Fatalf("runes: %+v", wide)
	}
	narrow := links(200)
	ends := 4
	for _, link := range narrow {
		if link.Runes[0] >= 23 {
			break
		}
		if link.Runes[0] > ends {
			t.Fatalf("a gap in one wrapped link: %+v", narrow)
		}
		ends = max(ends, link.Runes[1])
	}
	if ends != 23 || len(narrow) <= len(wide) {
		t.Fatalf("the link did not wrap whole: %+v", narrow)
	}
}

// TextAt finds the rune under a point, in its whole paragraph, and nothing
// off the text.
func TestTextAt(t *testing.T) {
	e := testEngine(t)
	doc := &Document{Blocks: []Block{
		{Kind: KindParagraph, Spans: []Span{{Text: "go to "}, {Text: "linked", Href: "https://example.net/"}, {Text: " now"}}},
	}}
	l := mustLayout(t, e, doc, 1000, LayoutOptions{})
	if err := l.Complete(context.Background()); err != nil {
		t.Fatal(err)
	}
	link := l.Links(View{Top: 0, Height: l.Extent(), Width: l.Width(), Scale: 1})[0]
	mid := image.Pt((link.Rect.Min.X+link.Rect.Max.X)/2, (link.Rect.Min.Y+link.Rect.Max.Y)/2)
	hit, ok := l.TextAt(mid)
	if !ok || hit.Text != "go to linked now" || hit.Block != 0 || hit.Para != 0 || hit.Rune < 6 || hit.Rune >= 12 {
		t.Fatalf("hit %+v %v", hit, ok)
	}
	for _, p := range []image.Point{{5, mid.Y}, {mid.X, 5}, {900, mid.Y}} {
		if hit, ok := l.TextAt(p); ok {
			t.Fatalf("%v hit %+v", p, hit)
		}
	}
}

// Marks tint a link's text behind its glyphs, only where the link is, and
// a selection over a mark wins.
func TestRasterMarks(t *testing.T) {
	e := testEngine(t)
	doc := &Document{Blocks: []Block{
		{Kind: KindParagraph, Spans: []Span{{Text: "go to "}, {Text: "the linked words", Href: "https://example.net/"}, {Text: " now"}}},
	}}
	l := mustLayout(t, e, doc, 1000, LayoutOptions{})
	if err := l.Complete(context.Background()); err != nil {
		t.Fatal(err)
	}
	view := View{Top: 0, Height: l.Extent(), Width: l.Width(), Scale: 1}
	link := l.Links(view)[0]
	p := LightPalette()
	raster := func(opts RasterOptions) *image.RGBA {
		opts.Palette = p
		img, err := l.Raster(context.Background(), view, opts)
		if err != nil {
			t.Fatal(err)
		}
		return img
	}
	plain := raster(RasterOptions{})
	marked := raster(RasterOptions{Marks: []Mark{link.Mark()}})
	tinted := 0
	for y := plain.Bounds().Min.Y; y < plain.Bounds().Max.Y; y++ {
		for x := plain.Bounds().Min.X; x < plain.Bounds().Max.X; x++ {
			if plain.RGBAAt(x, y) == marked.RGBAAt(x, y) {
				continue
			}
			if !image.Pt(x, y).In(link.Rect) {
				t.Fatalf("a mark drew at %d,%d, off the link %v", x, y, link.Rect)
			}
			if marked.RGBAAt(x, y) == p.mark() {
				tinted++
			}
		}
	}
	if tinted == 0 {
		t.Fatal("the mark drew nothing")
	}
	if other := raster(RasterOptions{Marks: []Mark{{Block: 0, Para: 1, Start: 0, End: 99}, {Block: 0, Para: 0, Start: 2, End: 2}}}); !other.Bounds().Eq(plain.Bounds()) || string(other.Pix) != string(plain.Pix) {
		t.Fatal("a mark of no text drew")
	}
	sel, err := l.Select(context.Background(), Selection{Anchor: Place{Point: link.Rect.Min}, Focus: Place{Point: link.Rect.Max.Sub(image.Pt(1, 1))}})
	if err != nil || sel == nil {
		t.Fatalf("select: %v", err)
	}
	both := raster(RasterOptions{Marks: []Mark{link.Mark()}, Selection: sel})
	corner := link.Rect.Min.Add(image.Pt(1, 1))
	if both.RGBAAt(corner.X, corner.Y) != p.Selection {
		t.Fatalf("under a selection the mark shows: %v", both.RGBAAt(corner.X, corner.Y))
	}
}
