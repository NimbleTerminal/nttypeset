// Package raster draws go-text shaped runs into RGBA images. It owns only
// glyph-mask caching; document layout and semantic styling belong to callers.
package raster

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"github.com/go-text/render"
	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"
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
	renderer render.Renderer
	glyphs   map[glyphKey]*glyphMask
	bytes    int
	budget   int
	hits     int
	misses   int
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
// bitmap or SVG glyph data falls back to go-text's direct renderer.
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

// DrawDirect paints a run through go-text without consulting the mask cache.
func (r *Rasterizer) DrawDirect(dst *image.RGBA, run *shaping.Output, size int, scale float64, x, baseline, height int, ink color.RGBA) {
	r.renderer.FontSize, r.renderer.PixScale, r.renderer.Color = float32(size), float32(scale), ink
	pad := max(1, int(math.Ceil(float64(size)*scale)))
	width := 0
	for _, g := range run.Glyphs {
		width += int(math.Ceil(float64(g.Advance) / 64 * scale))
	}
	tile := image.NewRGBA(image.Rect(0, 0, max(1, width)+2*pad, max(1, height)+2*pad))
	r.renderer.DrawShapedRunAt(*run, tile, pad, pad)
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
	r.renderer.FontSize, r.renderer.PixScale, r.renderer.Color = float32(key.size), key.scale, color.RGBA{255, 255, 255, 255}
	single := shaping.Output{Face: face, Glyphs: []shaping.Glyph{g}}
	single.Glyphs[0].XOffset = fixed.Int26_6(math.Round(float64(g.XOffset) + float64(key.fx)/glyphQuanta/float64(key.scale)*64))
	single.Glyphs[0].YOffset = fixed.Int26_6(math.Round(float64(g.YOffset) - float64(key.fy)/glyphQuanta/float64(key.scale)*64))
	r.renderer.DrawShapedRunAt(single, tile, pad, pad)
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
