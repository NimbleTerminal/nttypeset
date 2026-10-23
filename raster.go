package nttypeset

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"math"

	xdraw "golang.org/x/image/draw"
)

// RasterOptions are the choices of one raster.
type RasterOptions struct {
	Palette Palette // the colors to paint with
	// Selection is the text to highlight behind its runs, in Palette.Selection.
	// A Selected from another Layout is ignored.
	Selection *Selected
	// Marks are stretches of text to tint behind their runs, as for the link
	// under a pointer: Palette.Selection halfway to Palette.Background, under
	// any Selection. A Mark of no text, as a picture's, draws nothing.
	Marks        []Mark
	OmitAnimated bool // leave Animated images as background for the caller to composite
}

// Mark is a stretch of a paragraph's text: the runes [Start, End) of a
// paragraph of a block, the same at every width. A Link says the Mark of
// its line of text.
type Mark struct {
	Block, Para, Start, End int
}

// Mark is the stretch of text the link covers on its line; nothing for a
// picture's link.
func (k Link) Mark() Mark {
	return Mark{Block: k.Block, Para: k.Para, Start: k.Runes[0], End: k.Runes[1]}
}

// Link is a link hit area.
type Link struct {
	Rect image.Rectangle // raster pixels, clipped to the view
	URL  string
	// Where it comes from, the same at every width: its block, and for a
	// text link the paragraph of the block and the runes of the paragraph
	// the rectangle covers, [start, end). A link wrapped over lines gives a
	// rectangle a line, with runes that meet; two links to one URL do not.
	Block, Para int
	Runes       [2]int
}

// Placed is an image placed by the layout.
type Placed struct {
	Rect image.Rectangle // raster pixels, unclipped
	Key  string          // the Image's Key
}

// Raster lays out through the view's bottom and draws the view into a new
// picture of v.Size(). A view without a positive Width, Height and Scale, or
// with only one side of Pixels set, is an error; a raster over
// Limits.Pixels, or one that would draw text at more than Limits.Em raster
// pixels per em, is a *LimitError.
func (l *Layout) Raster(ctx context.Context, v View, opts RasterOptions) (img *image.RGBA, err error) {
	if err := l.stale(); err != nil {
		return nil, err
	}
	defer l.guard(ctx, &err)
	defer l.e.enter(ctx)()
	if v.Width < 1 || v.Height < 1 || !(v.Scale > 0) || math.IsInf(v.Scale, 0) {
		return nil, errors.New("nttypeset: view must have a positive width, height and scale")
	}
	size := v.Size()
	w, h := size.X, size.Y
	if w < 1 || h < 1 || v.Pixels != (image.Point{}) && (v.Pixels.X < 1 || v.Pixels.Y < 1) {
		return nil, errors.New("nttypeset: view has no pixels")
	}
	if w > l.e.limits.Pixels/h {
		panic(&LimitError{What: "typeset raster exceeds pixel limit", Limit: int64(l.e.limits.Pixels)})
	}
	l.ensure(v.Top + v.Height)
	return l.rasterTo(v, w, h, opts), nil
}

// rasterTo draws the view into a w×h picture, scaling each operation by
// v.Scale. The region normally maps onto the picture; a viewer may ask for its
// picture's exact device size, which can differ from the scaled region by a
// rounding pixel at the bottom or right edge. With OmitAnimated, animated
// images stay as blank reserved rectangles for the viewer to composite over.
// A text run in the view drawn at more than Limits.Em pixels per em panics
// with a *LimitError before anything is allocated: glyph tiles grow with it.
func (l *Layout) rasterTo(v View, w, h int, opts RasterOptions) *image.RGBA {
	p := opts.Palette
	top, scale := v.Top, v.Scale
	bounds := image.Rect(0, 0, w, h)
	for i := range l.ops {
		op := &l.ops[i]
		if op.run != nil && float64(op.size)*scale > float64(l.e.limits.Em) && v.Project(op.rect).Overlaps(bounds) {
			panic(&LimitError{What: fmt.Sprintf("typeset glyph size exceeds %s pixels", groupDigits(l.e.limits.Em)), Limit: int64(l.e.limits.Em)})
		}
	}
	out := image.NewRGBA(bounds)
	draw.Draw(out, out.Bounds(), image.NewUniform(p.Background), image.Point{}, draw.Src)
	for i := range l.ops {
		op := &l.ops[i]
		l.e.check()
		r := v.Project(op.rect)
		if !r.Overlaps(out.Bounds()) {
			continue
		}
		switch {
		case op.fill != fillNone:
			draw.Draw(out, r, &image.Uniform{C: p.fillColor(op.fill)}, image.Point{}, draw.Src)
		case op.asset != nil:
			if opts.OmitAnimated && op.animated {
				continue
			}
			xdraw.CatmullRom.Scale(out, r, op.asset, op.asset.Bounds(), draw.Over, nil)
		case op.run != nil:
			ink := p.runInk(op)
			for _, m := range opts.Marks {
				if x0, x1, ok := markSpan(m, op); ok {
					hr := image.Rect(int(math.Round(float64(x0-v.Left)*scale)), r.Min.Y, int(math.Round(float64(x1-v.Left)*scale)), r.Max.Y)
					draw.Draw(out, hr, image.NewUniform(p.mark()), image.Point{}, draw.Src)
				}
			}
			if x0, x1, ok := opts.Selection.span(l, i, op); ok {
				hr := image.Rect(int(math.Round(float64(x0-v.Left)*scale)), r.Min.Y, int(math.Round(float64(x1-v.Left)*scale)), r.Max.Y)
				draw.Draw(out, hr, image.NewUniform(p.Selection), image.Point{}, draw.Src)
			}
			baseline := int(math.Round(float64(op.baseline-top) * scale))
			l.e.rasterizer.Draw(out, op.run, op.size, scale, r.Min.X, baseline, r.Dy(), ink)
			if op.style&Strike != 0 {
				y := int(math.Round(float64(op.baseline-top-op.size/3) * scale))
				draw.Draw(out, image.Rect(r.Min.X, y, r.Max.X, y+1), image.NewUniform(ink), image.Point{}, draw.Src)
			}
		}
	}
	return out
}

// Links returns the link hit areas in the view: every text run and image with
// an Href laid so far, projected and clipped to the raster, dropped if
// nothing is left. Callers Ensure through the view first, as Raster does.
func (l *Layout) Links(v View) []Link {
	size := v.Size()
	bounds := image.Rect(0, 0, size.X, size.Y)
	var out []Link
	for _, op := range l.ops {
		if op.href == "" {
			continue
		}
		r := v.Project(op.rect).Intersect(bounds)
		if !r.Empty() {
			link := Link{Rect: r, URL: op.href, Block: op.block, Para: op.para}
			if op.run != nil {
				link.Runes = [2]int{op.run.Runes.Offset, op.run.Runes.Offset + op.run.Runes.Count}
			}
			out = append(out, link)
		}
	}
	return out
}

// Images returns the images with a Still laid so far whose projected
// rectangle overlaps the view's raster, unclipped.
func (l *Layout) Images(v View) []Placed {
	size := v.Size()
	viewport := image.Rect(0, 0, size.X, size.Y)
	var out []Placed
	for _, op := range l.ops {
		if op.asset == nil {
			continue
		}
		if r := v.Project(op.rect); r.Overlaps(viewport) {
			out = append(out, Placed{Rect: r, Key: op.destination})
		}
	}
	return out
}

// markSpan is the page x range of the part of a text operation a mark
// covers.
func markSpan(m Mark, op *operation) (x0, x1 int, ok bool) {
	if op.run == nil || op.gutter || op.block != m.Block || op.para != m.Para {
		return 0, 0, false
	}
	off, count := op.run.Runes.Offset, op.run.Runes.Count
	return runSpan(op, max(m.Start, off)-off, min(m.End, off+count)-off)
}
