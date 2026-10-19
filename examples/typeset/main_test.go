package main

import (
	"bytes"
	"context"
	"testing"
	"testing/fstest"

	"nimbleterminal.dev/ntfonts"
	"nimbleterminal.dev/nttypeset"
	"nimbleterminal.dev/nttypeset/examples/internal/pixels"
)

func TestParseSelection(t *testing.T) {
	sel, err := parseSelection("1.0-2.12")
	if err != nil {
		t.Fatal(err)
	}
	want := nttypeset.Selection{
		Anchor: nttypeset.Place{Placed: true, Block: 1},
		Focus:  nttypeset.Place{Placed: true, Block: 2, Rune: 12},
	}
	if sel != want {
		t.Fatalf("selection = %+v, want %+v", sel, want)
	}
	for _, bad := range []string{"", "1.0", "1-2", "a.b-1.0", "1.0-2", "-1.0-2.0", "1.0x-2.0"} {
		if _, err := parseSelection(bad); err == nil {
			t.Errorf("%q: no error", bad)
		}
	}
}

func TestSampleIsMarkdownWithItsImage(t *testing.T) {
	p, err := open(context.Background(), "", options{})
	if err != nil {
		t.Fatal(err)
	}
	if p.title != "sample.md" || p.doc.Lines || len(p.doc.Blocks) < 8 {
		t.Fatalf("page %q, lines %v, %d blocks", p.title, p.doc.Lines, len(p.doc.Blocks))
	}
	var stills, placeholders int
	for _, b := range p.doc.Blocks {
		if b.Kind != nttypeset.KindImage {
			continue
		}
		if b.Image != nil && b.Image.Still != nil {
			stills++
		} else {
			placeholders++
		}
	}
	if stills != 1 || placeholders != 1 {
		t.Fatalf("stills %d, placeholders %d; want one of each", stills, placeholders)
	}
}

func TestTextFilesAreLinesHighlightedByName(t *testing.T) {
	ctx := context.Background()
	doc, err := build(ctx, "main.go", []byte("package main\n\nfunc main() {}\n"), fstest.MapFS{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !doc.Lines || len(doc.Blocks) != 3 {
		t.Fatalf("lines %v, %d blocks", doc.Lines, len(doc.Blocks))
	}
	if doc.Blocks[0].Spans[0].Token == 0 {
		t.Fatal("first line of a Go file has no token")
	}
	plain, err := build(ctx, "notes.unknown", []byte("just words\n"), fstest.MapFS{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if plain.Blocks[0].Spans[0].Token != 0 {
		t.Fatal("an unknown extension was highlighted")
	}
}

func TestRenderPaintsAndSelectionShows(t *testing.T) {
	ctx := context.Background()
	eng, err := nttypeset.NewEngine(nttypeset.WithFonts(ntfonts.Embedded))
	if err != nil {
		t.Fatal(err)
	}
	p, err := open(ctx, "", options{})
	if err != nil {
		t.Fatal(err)
	}
	v := nttypeset.View{Width: 800, Height: 300, Scale: 1}
	lay, err := eng.Layout(p.doc, 800, p.opts)
	if err != nil {
		t.Fatal(err)
	}
	light, err := p.render(ctx, lay, v, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := light.Bounds().Size(); got.X != 800 || got.Y != 300 {
		t.Fatalf("size %v", got)
	}
	if pixels.CountNot(light, nttypeset.LightPalette().Background) == 0 {
		t.Fatal("light page has no ink")
	}
	dark, err := p.render(ctx, lay, v, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if pixels.CountNot(dark, nttypeset.DarkPalette().Background) == 0 {
		t.Fatal("dark page has no ink")
	}

	sel, err := parseSelection("1.0-1.20")
	if err != nil {
		t.Fatal(err)
	}
	selected, err := p.render(ctx, lay, v, false, &sel)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(selected.Pix, light.Pix) {
		t.Fatal("a selection changed no pixel")
	}
}
