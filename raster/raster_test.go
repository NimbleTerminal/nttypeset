package raster

import (
	"bytes"
	"image"
	"image/color"
	"testing"

	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/math/fixed"
)

func TestDrawCachesShapedGlyphs(t *testing.T) {
	face, err := font.ParseTTF(bytes.NewReader(goregular.TTF))
	if err != nil {
		t.Fatal(err)
	}
	shaper := shaping.HarfbuzzShaper{}
	text := []rune("Raster cache")
	run := shaper.Shape(shaping.Input{
		Text: text, RunEnd: len(text), Face: face, Size: fixed.I(24), Direction: di.DirectionLTR,
	})
	r := New(1 << 20)
	first := image.NewRGBA(image.Rect(0, 0, 240, 48))
	second := image.NewRGBA(first.Bounds())
	for _, dst := range []*image.RGBA{first, second} {
		r.Draw(dst, &run, 24, 1.5, 8, 36, 36, color.RGBA{R: 20, G: 40, B: 80, A: 255})
	}
	for i := range first.Pix {
		if first.Pix[i] != second.Pix[i] {
			t.Fatal("cached raster differs from the first raster")
		}
	}
	stats := r.Stats()
	if stats.Entries == 0 || stats.Hits == 0 || stats.Bytes > stats.Budget {
		t.Fatalf("unexpected cache stats: %+v", stats)
	}
}
