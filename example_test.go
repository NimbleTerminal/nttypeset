package nttypeset_test

import (
	"context"
	"fmt"

	"github.com/alecthomas/chroma/v2/styles"
	"nimbleterminal.dev/nttypeset"
	"nimbleterminal.dev/nttypeset/internal/testfonts"
)

// aliasSource stands in for ntfonts.Embedded: it serves the Go test fonts
// under the families of the built-in readable look.
type aliasSource struct{}

func (aliasSource) Face(family, variant string) ([]byte, error) {
	switch family {
	case "Literata":
		family = testfonts.Body
	case "Iosevka":
		family = testfonts.Code
	}
	return testfonts.Source.Face(family, variant)
}

func ExampleLines() {
	ctx := context.Background()
	eng, err := nttypeset.NewEngine(nttypeset.WithFonts(aliasSource{}))
	if err != nil {
		panic(err)
	}
	doc, err := nttypeset.Lines(ctx, "func main() {}", "go", styles.Get("github"))
	if err != nil {
		panic(err)
	}
	lay, err := eng.Layout(doc, 400, nttypeset.LayoutOptions{})
	if err != nil {
		panic(err)
	}
	img, err := lay.Raster(ctx, nttypeset.View{Width: 400, Height: 100, Scale: 1},
		nttypeset.RasterOptions{Palette: nttypeset.LightPalette()})
	if err != nil {
		panic(err)
	}
	fmt.Println(img.Bounds())
	// Output: (0,0)-(400,100)
}

// The same model and raster calls run natively and under js/wasm. A host owns
// display, scrolling, filtering, and any filesystem or browser integration.
func ExampleSection() {
	ctx := context.Background()
	e, err := nttypeset.NewEngine(nttypeset.WithFonts(aliasSource{}))
	if err != nil {
		panic(err)
	}
	doc := &nttypeset.Document{
		Blocks:   []nttypeset.Block{{SourceID: "paragraph", Size: 16, Format: &nttypeset.BlockFormat{LineHeight: 24}, Spans: []nttypeset.Span{{Text: "A section that can reflow."}}}},
		Sections: []nttypeset.Section{{Start: 0, End: 1, SourceID: "card", Padding: nttypeset.Insets{Top: 12, Right: 12, Bottom: 12, Left: 12}}},
	}
	l, err := e.Layout(doc, 400, nttypeset.LayoutOptions{})
	if err != nil {
		panic(err)
	}
	_, err = l.Raster(ctx, nttypeset.View{Width: 400, Height: 150, Scale: 1}, nttypeset.RasterOptions{Palette: nttypeset.LightPalette()})
	if err != nil {
		panic(err)
	}
	a, found, err := l.AnchorBounds(ctx, "card", false)
	if err != nil {
		panic(err)
	}
	id, _, err := l.SourceAt(ctx, nttypeset.Place{Block: 0, Placed: true})
	if err != nil {
		panic(err)
	}
	fmt.Println(found, a.Complete, id)
	// Output: true true paragraph
}
