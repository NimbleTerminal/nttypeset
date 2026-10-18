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
