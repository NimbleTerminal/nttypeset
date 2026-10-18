package nttypeset

import (
	"context"
	"image"
	"testing"

	"nimbleterminal.dev/nttypeset/internal/testfonts"
)

// previewSource serves the Go fonts under every family of the four built-in
// looks, so Preview can run over all of them.
type previewSource struct{}

func (previewSource) Face(family, variant string) ([]byte, error) {
	switch family {
	case "Iosevka", "IBM Plex Mono", "JetBrains Mono":
		family = testfonts.Code
	default:
		family = testfonts.Body
	}
	return testfonts.Source.Face(family, variant)
}

func rowHasInk(img *image.RGBA, y0, y1 int, bg [4]uint8) bool {
	for y := y0; y < y1; y++ {
		for x := img.Rect.Min.X; x < img.Rect.Max.X; x++ {
			c := img.RGBAAt(x, y)
			if [4]uint8{c.R, c.G, c.B, c.A} != bg {
				return true
			}
		}
	}
	return false
}

func TestPreviewBoundedAndCancelled(t *testing.T) {
	p := LightPalette()
	bg := [4]uint8{p.Background.R, p.Background.G, p.Background.B, p.Background.A}
	// Four copies of one look that the alias source can serve.
	looks := []FontLook{fontLooks[0], fontLooks[0], fontLooks[0], fontLooks[0]}
	fonts := WithFonts(aliasSource{})
	ctx := context.Background()

	img, err := Preview(ctx, looks, "", "The quick brown fox", "func main() {}", 480, 320, p, fonts)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds() != image.Rect(0, 0, 480, 320) {
		t.Fatalf("bounds = %v", img.Bounds())
	}
	for i := range 4 {
		if !rowHasInk(img, i*80, (i+1)*80, bg) {
			t.Errorf("row %d has no ink", i)
		}
	}

	// The real set of four looks, over a source that serves them all.
	img, err = Preview(ctx, FontLooks(), "jetbrains", "The quick brown fox", "func main() {}", 480, 320, p, WithFonts(previewSource{}))
	if err != nil {
		t.Fatal(err)
	}
	for i := range 4 {
		if !rowHasInk(img, i*80, (i+1)*80, bg) {
			t.Errorf("builtin row %d has no ink", i)
		}
	}

	// Rows follow the number of looks.
	img, err = Preview(ctx, looks[:2], "", "body", "", 480, 320, p, fonts)
	if err != nil {
		t.Fatal(err)
	}
	if !rowHasInk(img, 0, 160, bg) || !rowHasInk(img, 160, 320, bg) {
		t.Error("two-look preview has an empty row")
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := Preview(cancelled, looks, "", "a", "b", 480, 320, p, fonts); err != context.Canceled {
		t.Errorf("cancelled err = %v", err)
	}
	for _, size := range [][2]int{{5000, 5000}, {100, 100}, {480, 159}, {1201, 480}} {
		_, err := Preview(ctx, looks, "", "a", "b", size[0], size[1], p, fonts)
		if err == nil || err.Error() != "nttypeset: preview size out of range" {
			t.Errorf("%v: err = %v", size, err)
		}
	}
	if _, err := Preview(ctx, looks, "", "a", "b", 480, 320, p); err == nil {
		t.Error("no WithFonts: want an error")
	}
	if _, err := Preview(ctx, nil, "", "a", "b", 480, 320, p, fonts); err == nil || err.Error() != "nttypeset: preview needs at least one look" {
		t.Errorf("zero looks err = %v", err)
	}
	if _, err := Preview(ctx, looks, "bogus", "a", "b", 480, 320, p, fonts); err == nil {
		t.Error("bad override: want an error")
	}
}

func TestPreviewNilContext(t *testing.T) {
	img, err := Preview(nil, FontLooks()[:1], "", "body", "code", 400, 400, LightPalette(), WithFonts(previewSource{})) //nolint:staticcheck // nil ctx is documented
	if err != nil || img == nil {
		t.Fatalf("Preview(nil) = %v, %v", img, err)
	}
}
