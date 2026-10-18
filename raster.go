package nttypeset

import (
	"context"
	"errors"
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
	Selection    *Selected
	OmitAnimated bool // leave Animated images as background for the caller to composite
}

// Link is a link hit area.
type Link struct {
	Rect image.Rectangle // raster pixels, clipped to the view
	URL  string
}

// Placed is an image placed by the layout.
type Placed struct {
	Rect image.Rectangle // raster pixels, unclipped
	Key  string          // the Image's Key
}

// Raster lays out through the view's bottom and draws the view into a new
// picture of v.Size(). A view without a positive Width, Height and Scale, or
// with only one side of Pixels set, is an error; a raster over
// Limits.Pixels is a *LimitError.
func (l *Layout) Raster(ctx context.Context, v View, opts RasterOptions) (img *image.RGBA, err error) {
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
		panic(&LimitError{What: "typeset raster exceeds pixel limit", Limit: l.e.limits.Pixels})
	}
	l.ensure(v.Top + v.Height)
	return l.rasterTo(v, w, h, opts), nil
}

// rasterTo draws the view into a w×h picture, scaling each operation by
// v.Scale. The region normally maps onto the picture; a viewer may ask for its
// picture's exact device size, which can differ from the scaled region by a
// rounding pixel at the bottom or right edge. With OmitAnimated, animated
// images stay as blank reserved rectangles for the viewer to composite over.
func (l *Layout) rasterTo(v View, w, h int, opts RasterOptions) *image.RGBA {
	p := opts.Palette
	top, scale := v.Top, v.Scale
	out := image.NewRGBA(image.Rect(0, 0, w, h))
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
			out = append(out, Link{Rect: r, URL: op.href})
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
