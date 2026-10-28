// Package raster draws go-text shaped runs into RGBA images. It owns only
// glyph-mask caching; document layout and semantic styling belong to callers.
//
// Direct shaped-run outline, bitmap, and SVG glyph rendering is adapted from
// github.com/go-text/render v0.2.1 (Copyright 2021 The go-text authors,
// Unlicense OR BSD-3-Clause).
package raster

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"math"

	"github.com/NimbleMarkets/oksvg"
	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/shaping"
	"github.com/srwiley/rasterx"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/tiff"
)

const glyphQuanta = 4

type glyphKey struct {
	face  *font.Face
	gid   font.GID
	size  int
	scale float32
	fx    uint8
	fy    uint8
}

type glyphMask struct {
	mask   *image.Alpha
	offset image.Point
}

// Stats describes the bounded cache owned by a Rasterizer.
type Stats struct {
	Entries, Bytes, Budget int
	Hits, Misses           int
}

// Rasterizer is not safe for concurrent use. A caller should serialize access
// or give each independent layout its own rasterizer.
type Rasterizer struct {
	glyphs map[glyphKey]*glyphMask
	bytes  int
	budget int
	hits   int
	misses int
}

func New(budget int) *Rasterizer {
	if budget <= 0 {
		budget = 32 << 20
	}
	return &Rasterizer{glyphs: map[glyphKey]*glyphMask{}, budget: budget}
}

func (r *Rasterizer) Stats() Stats {
	return Stats{Entries: len(r.glyphs), Bytes: r.bytes, Budget: r.budget, Hits: r.hits, Misses: r.misses}
}

// SetBudget changes the cache limit. Existing masks remain until the next
// insertion would exceed it, when the cache is cleared.
func (r *Rasterizer) SetBudget(budget int) {
	if budget > 0 {
		r.budget = budget
	}
}

// Clear drops masks for old font faces after a font-set change.
func (r *Rasterizer) Clear() {
	r.glyphs = map[glyphKey]*glyphMask{}
	r.bytes = 0
}

// Draw paints one shaped run. Outline glyphs use cached quarter-pixel masks;
// bitmap or SVG glyph data falls back to direct shaped-run rendering.
func (r *Rasterizer) Draw(dst *image.RGBA, run *shaping.Output, size int, scale float64, x, baseline, height int, ink color.RGBA) {
	for _, g := range run.Glyphs {
		if _, ok := run.Face.GlyphData(g.GlyphID).(font.GlyphOutline); !ok {
			r.DrawDirect(dst, run, size, scale, x, baseline, height, ink)
			return
		}
	}
	pixScale := float32(scale)
	src := image.NewUniform(ink)
	pos := float32(x)
	for _, g := range run.Glyphs {
		gx := pos + float32(g.XOffset)/64*pixScale
		gy := float32(baseline) - float32(g.YOffset)/64*pixScale
		pos += float32(g.Advance) / 64 * pixScale
		ix, fx := quantize(gx)
		iy, fy := quantize(gy)
		m := r.glyph(glyphKey{run.Face, g.GlyphID, size, pixScale, fx, fy}, g, run.Face, height)
		if m == nil {
			continue
		}
		b := m.mask.Bounds()
		dstRect := image.Rect(ix+m.offset.X, iy+m.offset.Y, ix+m.offset.X+b.Dx(), iy+m.offset.Y+b.Dy())
		draw.DrawMask(dst, dstRect, src, image.Point{}, m.mask, b.Min, draw.Over)
	}
}

// DrawDirect paints a run directly without consulting the mask cache.
//
// Outline, bitmap, and SVG glyph rendering in DrawDirect and drawShapedRunAt
// is adapted from github.com/go-text/render v0.2.1 (Copyright 2021 The go-text
// authors, Unlicense OR BSD-3-Clause).
func (r *Rasterizer) DrawDirect(dst *image.RGBA, run *shaping.Output, size int, scale float64, x, baseline, height int, ink color.RGBA) {
	pad := max(1, int(math.Ceil(float64(size)*scale)))
	width := 0
	for _, g := range run.Glyphs {
		width += int(math.Ceil(float64(g.Advance) / 64 * scale))
	}
	tile := image.NewRGBA(image.Rect(0, 0, max(1, width)+2*pad, max(1, height)+2*pad))
	drawShapedRunAt(tile, *run, float32(size), float32(scale), ink, pad, pad)
	draw.Draw(dst, tile.Bounds().Add(image.Pt(x-pad, baseline-pad)), tile, image.Point{}, draw.Over)
}

func quantize(v float32) (int, uint8) {
	i := int(math.Floor(float64(v)))
	q := int(math.Round(float64(v-float32(i)) * glyphQuanta))
	if q == glyphQuanta {
		return i + 1, 0
	}
	return i, uint8(q)
}

func (r *Rasterizer) glyph(key glyphKey, g shaping.Glyph, face *font.Face, height int) *glyphMask {
	if m, ok := r.glyphs[key]; ok {
		r.hits++
		return m
	}
	r.misses++
	pad := max(1, int(math.Ceil(float64(key.size)*float64(key.scale))))
	advance := int(math.Ceil(float64(g.Advance) / 64 * float64(key.scale)))
	tile := image.NewRGBA(image.Rect(0, 0, max(1, advance)+2*pad, max(1, height)+2*pad))
	single := shaping.Output{Face: face, Glyphs: []shaping.Glyph{g}}
	single.Glyphs[0].XOffset = fixed.Int26_6(math.Round(float64(g.XOffset) + float64(key.fx)/glyphQuanta/float64(key.scale)*64))
	single.Glyphs[0].YOffset = fixed.Int26_6(math.Round(float64(g.YOffset) - float64(key.fy)/glyphQuanta/float64(key.scale)*64))
	drawShapedRunAt(tile, single, float32(key.size), key.scale, color.RGBA{255, 255, 255, 255}, pad, pad)
	bounds := image.Rectangle{Min: image.Pt(tile.Rect.Max.X, tile.Rect.Max.Y)}
	for y := 0; y < tile.Rect.Dy(); y++ {
		for x := 0; x < tile.Rect.Dx(); x++ {
			if tile.Pix[y*tile.Stride+x*4+3] != 0 {
				bounds = bounds.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	var m *glyphMask
	if !bounds.Empty() {
		mask := image.NewAlpha(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
		for y := 0; y < bounds.Dy(); y++ {
			for x := 0; x < bounds.Dx(); x++ {
				mask.Pix[y*mask.Stride+x] = tile.Pix[(bounds.Min.Y+y)*tile.Stride+(bounds.Min.X+x)*4+3]
			}
		}
		m = &glyphMask{mask: mask, offset: bounds.Min.Sub(image.Pt(pad, pad))}
		if r.bytes+len(mask.Pix) > r.budget {
			r.Clear()
		}
		r.bytes += len(mask.Pix)
	}
	r.glyphs[key] = m
	return m
}

// drawShapedRunAt rasterizes a shaped glyph run into img starting at
// (startX, startY) in output pixel coordinates.
//
// Adapted from github.com/go-text/render v0.2.1 (Renderer.DrawShapedRunAt,
// drawOutline, drawBitmap, and drawSVG; Copyright 2021 The go-text authors,
// Unlicense OR BSD-3-Clause), updated to import github.com/NimbleMarkets/oksvg
// directly and decode bitmap glyph formats without global image.RegisterFormat
// side effects.
func drawShapedRunAt(img draw.Image, run shaping.Output, fontSize, pixScale float32, ink color.Color, startX, startY int) {
	if run.Face == nil {
		return
	}
	upem := run.Face.Upem()
	if upem == 0 {
		return
	}
	if pixScale == 0 {
		pixScale = 1
	}
	scale := fontSize * pixScale / float32(upem)

	b := img.Bounds()
	scanner := rasterx.NewScannerGV(b.Dx(), b.Dy(), img, b)
	f := rasterx.NewFiller(b.Dx(), b.Dy(), scanner)
	f.SetColor(ink)
	x := float32(startX)
	y := float32(startY)
	for _, g := range run.Glyphs {
		xPos := x + fixed266ToFloat(g.XOffset)*pixScale
		yPos := y - fixed266ToFloat(g.YOffset)*pixScale
		switch format := run.Face.GlyphData(g.GlyphID).(type) {
		case font.GlyphOutline:
			drawOutline(format, f, scale, xPos, yPos)
		case font.GlyphBitmap:
			_ = drawBitmap(g, format, img, f, scale, pixScale, ink, xPos, yPos)
		case font.GlyphSVG:
			_ = drawSVG(g, format, img, pixScale, xPos, yPos)
		}
		x += fixed266ToFloat(g.Advance) * pixScale
	}
	f.Draw()
}

// drawOutline feeds scaled glyph outline segments into f.
// Adapted from github.com/go-text/render v0.2.1 (Copyright 2021 The go-text authors).
func drawOutline(outline font.GlyphOutline, f *rasterx.Filler, scale, x, y float32) {
	for _, s := range outline.Segments {
		switch s.Op {
		case opentype.SegmentOpMoveTo:
			f.Start(fixed.Point26_6{X: floatToFixed266(s.Args[0].X*scale + x), Y: floatToFixed266(-s.Args[0].Y*scale + y)})
		case opentype.SegmentOpLineTo:
			f.Line(fixed.Point26_6{X: floatToFixed266(s.Args[0].X*scale + x), Y: floatToFixed266(-s.Args[0].Y*scale + y)})
		case opentype.SegmentOpQuadTo:
			f.QuadBezier(
				fixed.Point26_6{X: floatToFixed266(s.Args[0].X*scale + x), Y: floatToFixed266(-s.Args[0].Y*scale + y)},
				fixed.Point26_6{X: floatToFixed266(s.Args[1].X*scale + x), Y: floatToFixed266(-s.Args[1].Y*scale + y)},
			)
		case opentype.SegmentOpCubeTo:
			f.CubeBezier(
				fixed.Point26_6{X: floatToFixed266(s.Args[0].X*scale + x), Y: floatToFixed266(-s.Args[0].Y*scale + y)},
				fixed.Point26_6{X: floatToFixed266(s.Args[1].X*scale + x), Y: floatToFixed266(-s.Args[1].Y*scale + y)},
				fixed.Point26_6{X: floatToFixed266(s.Args[2].X*scale + x), Y: floatToFixed266(-s.Args[2].Y*scale + y)},
			)
		}
	}
	f.Stop(true)
}

// drawBitmap renders a bitmap glyph (and any fallback outline) into img.
// Adapted from github.com/go-text/render v0.2.1 (Copyright 2021 The go-text authors).
func drawBitmap(g shaping.Glyph, bitmap font.GlyphBitmap, img draw.Image, f *rasterx.Filler, fillerScale, pixScale float32, ink color.Color, x, y float32) error {
	top := y - fixed266ToFloat(g.YBearing)*pixScale
	bottom := top - fixed266ToFloat(g.Height)*pixScale
	right := x + fixed266ToFloat(g.Width)*pixScale
	switch bitmap.Format {
	case font.BlackAndWhite:
		if bitmap.Width > 0 && bitmap.Height > 0 {
			rec := image.Rect(0, 0, bitmap.Width, bitmap.Height)
			sub := image.NewPaletted(rec, color.Palette{color.Transparent, ink})
			for i := range sub.Pix {
				sub.Pix[i] = bitAt(bitmap.Data, i)
			}
			rect := image.Rect(int(x), int(top), int(right), int(bottom))
			xdraw.NearestNeighbor.Scale(img, rect, sub, sub.Bounds(), draw.Over, nil)
		}
	case font.JPG, font.PNG, font.TIFF:
		pix, err := decodeBitmapGlyph(bitmap.Format, bitmap.Data)
		if err != nil {
			return err
		}
		rect := image.Rect(int(x), int(top), int(right), int(bottom))
		xdraw.BiLinear.Scale(img, rect, pix, pix.Bounds(), draw.Over, nil)
	}
	if bitmap.Outline != nil {
		drawOutline(*bitmap.Outline, f, fillerScale, x, y)
	}
	return nil
}

func decodeBitmapGlyph(format font.BitmapFormat, data []byte) (image.Image, error) {
	r := bytes.NewReader(data)
	switch format {
	case font.PNG:
		return png.Decode(r)
	case font.JPG:
		return jpeg.Decode(r)
	case font.TIFF:
		return tiff.Decode(r)
	default:
		img, _, err := image.Decode(r)
		return img, err
	}
}

func bitAt(b []byte, i int) byte {
	if i/8 >= len(b) {
		return 0
	}
	return (b[i/8] >> (7 - i%8)) & 1
}

// drawSVG renders an OpenType SVG glyph into img using oksvg and rasterx.
// Adapted from github.com/go-text/render v0.2.1 (Copyright 2021 The go-text authors).
func drawSVG(g shaping.Glyph, svg font.GlyphSVG, img draw.Image, pixScale, x, y float32) error {
	pixWidth := int(fixed266ToFloat(g.Width) * pixScale)
	pixHeight := int(fixed266ToFloat(-g.Height) * pixScale)
	if pixWidth <= 0 || pixHeight <= 0 {
		return nil
	}
	pix, err := renderSVGStream(bytes.NewReader(svg.Source), pixWidth, pixHeight)
	if err != nil {
		return err
	}
	rect := image.Rect(
		int(fixed266ToFloat(g.XBearing)*pixScale),
		int(fixed266ToFloat(-g.YBearing)*pixScale),
		pixWidth,
		pixHeight,
	)
	draw.Draw(img, rect.Add(image.Point{X: int(x), Y: int(y)}), pix, image.Point{}, draw.Over)
	return nil
}

func renderSVGStream(stream io.Reader, width, height int) (*image.NRGBA, error) {
	icon, err := oksvg.ReadIconStream(stream)
	if err != nil {
		return nil, err
	}
	if icon.ViewBox.W <= 0 || icon.ViewBox.H <= 0 {
		return nil, io.ErrUnexpectedEOF
	}
	iconAspect := float32(icon.ViewBox.W / icon.ViewBox.H)
	viewAspect := float32(width) / float32(height)
	imgW, imgH := width, height
	if viewAspect > iconAspect {
		imgW = int(float32(height) * iconAspect)
	} else if viewAspect < iconAspect {
		imgH = int(float32(width) / iconAspect)
	}
	if imgW <= 0 || imgH <= 0 {
		return nil, io.ErrUnexpectedEOF
	}
	icon.SetTarget(icon.ViewBox.X, icon.ViewBox.Y, float64(imgW), float64(imgH))

	out := image.NewNRGBA(image.Rect(0, 0, imgW, imgH))
	scanner := rasterx.NewScannerGV(int(icon.ViewBox.W), int(icon.ViewBox.H), out, out.Bounds())
	raster := rasterx.NewDasher(width, height, scanner)
	icon.Draw(raster, 1)
	return out, nil
}

func fixed266ToFloat(i fixed.Int26_6) float32 {
	return float32(float64(i) / 64)
}

func floatToFixed266(f float32) fixed.Int26_6 {
	return fixed.Int26_6(int(float64(f) * 64))
}
