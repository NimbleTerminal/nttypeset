package raster

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/shaping"
	"github.com/srwiley/rasterx"
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

func TestDrawBitmapAndSVGGlyphs(t *testing.T) {
	g := shaping.Glyph{
		Width:    fixed.I(16),
		Height:   -fixed.I(16),
		XBearing: 0,
		YBearing: fixed.I(16),
	}

	// 1. Black-and-white bitmap glyph
	bwDst := image.NewRGBA(image.Rect(0, 0, 32, 32))
	bwScanner := rasterx.NewScannerGV(32, 32, bwDst, bwDst.Bounds())
	bwFiller := rasterx.NewFiller(32, 32, bwScanner)
	bwBitmap := font.GlyphBitmap{
		Width:  8,
		Height: 8,
		Format: font.BlackAndWhite,
		Data:   []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
	}
	ink := color.RGBA{R: 30, G: 60, B: 120, A: 255}
	if err := drawBitmap(g, bwBitmap, bwDst, bwFiller, 1, 1, ink, 4, 20); err != nil {
		t.Fatalf("drawBitmap BlackAndWhite: %v", err)
	}
	if bwDst.RGBAAt(8, 8) != ink {
		t.Fatalf("BlackAndWhite pixel = %v, want %v", bwDst.RGBAAt(8, 8), ink)
	}

	// 2. PNG bitmap glyph
	srcPNG := image.NewRGBA(image.Rect(0, 0, 4, 4))
	wantPNG := color.RGBA{R: 200, G: 40, B: 60, A: 255}
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			srcPNG.SetRGBA(x, y, wantPNG)
		}
	}
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, srcPNG); err != nil {
		t.Fatal(err)
	}
	pngDst := image.NewRGBA(image.Rect(0, 0, 32, 32))
	pngScanner := rasterx.NewScannerGV(32, 32, pngDst, pngDst.Bounds())
	pngFiller := rasterx.NewFiller(32, 32, pngScanner)
	pngBitmap := font.GlyphBitmap{
		Width:  4,
		Height: 4,
		Format: font.PNG,
		Data:   pngBuf.Bytes(),
	}
	if err := drawBitmap(g, pngBitmap, pngDst, pngFiller, 1, 1, ink, 4, 20); err != nil {
		t.Fatalf("drawBitmap PNG: %v", err)
	}
	if pngDst.RGBAAt(8, 8) != wantPNG {
		t.Fatalf("PNG pixel = %v, want %v", pngDst.RGBAAt(8, 8), wantPNG)
	}

	// 3. OpenType SVG glyph
	svgDst := image.NewRGBA(image.Rect(0, 0, 32, 32))
	svgGlyph := font.GlyphSVG{
		Source: []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><rect width="16" height="16" fill="#108040"/></svg>`),
	}
	if err := drawSVG(g, svgGlyph, svgDst, 1, 4, 20); err != nil {
		t.Fatalf("drawSVG: %v", err)
	}
	wantSVG := color.RGBA{R: 0x10, G: 0x80, B: 0x40, A: 0xff}
	if svgDst.RGBAAt(8, 8) != wantSVG {
		t.Fatalf("SVG pixel = %v, want %v", svgDst.RGBAAt(8, 8), wantSVG)
	}
}
