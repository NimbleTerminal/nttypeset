// Command sections writes docs/sections-before.png and docs/sections-after.png.
// The content and palette are a generic adaptation of go-thinkt's conversation
// fixture (internal/conversation/testdata/conversation.html, 2026-10-26).
package main

import (
	"context"
	"fmt"
	"image/color"
	"image/png"
	"os"

	"nimbleterminal.dev/nttypeset"
	"nimbleterminal.dev/nttypeset/internal/testfonts"
)

type source struct{}

func (source) Face(family, variant string) ([]byte, error) {
	switch family {
	case "Literata":
		family = testfonts.Body
	case "Iosevka":
		family = testfonts.Code
	}
	return testfonts.Source.Face(family, variant)
}
func rgba(r, g, b uint8) color.RGBA { return color.RGBA{R: r, G: g, B: b, A: 255} }
func main() {
	if err := render(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func render() error {
	e, err := nttypeset.NewEngine(nttypeset.WithFonts(source{}))
	if err != nil {
		return err
	}
	blocks := []nttypeset.Block{
		{Kind: nttypeset.KindHeading, Size: 20, SourceID: "entry-title", Spans: []nttypeset.Span{{Text: "A readable document", Style: nttypeset.Bold, Ink: rgba(198, 210, 225)}}},
		{Size: 13, Spans: []nttypeset.Span{{Text: "Metadata · stable source identity", Ink: rgba(170, 185, 204)}}},
		{Size: 16, Spans: []nttypeset.Span{{Text: "Notes", Style: nttypeset.Bold, Ink: rgba(227, 232, 240)}}},
		{Size: 16, Spans: []nttypeset.Span{{Text: "Keep related content together. Preserve source anchors while the document reflows to the available width.", Style: nttypeset.Mono, Ink: rgba(227, 232, 240)}}},
		{Size: 16, Spans: []nttypeset.Span{{Text: "Details", Style: nttypeset.Bold, Ink: rgba(227, 232, 240)}}},
		{Size: 16, SourceID: "detail-text", Spans: []nttypeset.Span{{Text: "Nested sections carry their own backgrounds, padding, and border colors. The host owns filtering and collapse.", Ink: rgba(227, 232, 240)}}},
		{Size: 16, Spans: []nttypeset.Span{{Text: "Text selection and links retain their logical positions.", Ink: rgba(159, 195, 255), Href: "https://example.org/details"}}},
	}
	for i := range blocks {
		blocks[i].Format = &nttypeset.BlockFormat{LineHeight: 24, SpaceAfter: 8}
	}
	sections := []nttypeset.Section{
		{Start: 0, End: 7, SourceID: "entry", Padding: nttypeset.Insets{Top: 16, Right: 16, Bottom: 16, Left: 16}, Background: rgba(25, 33, 45), Top: nttypeset.Border{Width: 1, Color: rgba(52, 65, 85)}, Right: nttypeset.Border{Width: 1, Color: rgba(52, 65, 85)}, Bottom: nttypeset.Border{Width: 1, Color: rgba(52, 65, 85)}, Left: nttypeset.Border{Width: 4, Color: rgba(144, 180, 250)}},
		{Start: 2, End: 4, SourceID: "notes", SpaceBefore: 12, SpaceAfter: 12, Padding: nttypeset.Insets{Top: 12, Right: 12, Bottom: 12, Left: 12}, Background: rgba(36, 40, 60), Left: nttypeset.Border{Width: 3, Color: rgba(182, 165, 245)}},
		{Start: 4, End: 6, SourceID: "details", SpaceAfter: 12, Padding: nttypeset.Insets{Top: 12, Right: 12, Bottom: 12, Left: 12}, Background: rgba(20, 44, 43), Left: nttypeset.Border{Width: 3, Color: rgba(112, 214, 180)}},
	}
	palette := nttypeset.DarkPalette()
	palette.Background = rgba(16, 20, 28)
	for _, after := range []bool{false, true} {
		doc := &nttypeset.Document{Blocks: blocks}
		name := "before"
		if after {
			doc.Sections = sections
			name = "after"
		}
		l, err := e.Layout(doc, 640, nttypeset.LayoutOptions{})
		if err != nil {
			return err
		}
		img, err := l.Raster(context.Background(), nttypeset.View{Width: 640, Height: 650, Scale: 1}, nttypeset.RasterOptions{Palette: palette})
		if err != nil {
			return err
		}
		f, err := os.Create("docs/sections-" + name + ".png")
		if err != nil {
			return err
		}
		err = png.Encode(f, img)
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}
