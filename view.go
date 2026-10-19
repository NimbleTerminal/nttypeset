package nttypeset

import (
	"image"
	"math"
)

// View is a region of the page and the raster it maps to. Top, Left, Width
// and Height are in layout pixels. Scale is raster pixels per layout pixel (1
// at 150 DPI), and it is what every page rectangle is projected with: when
// Pixels is set, Scale must be the scale the caller rasters at, as
// float64(Pixels.X)/float64(Width) for a picture that fills the device width.
// Pixels, if both of its sides are set, is the exact raster size, which may
// differ from the scaled region by a rounding pixel at the bottom or right.
type View struct {
	Top, Left, Width, Height int
	Scale                    float64
	Pixels                   image.Point
}

// Size returns the raster size: Pixels if both of its sides are positive,
// otherwise the region at Scale, each side rounded and at least 1.
func (v View) Size() image.Point {
	if v.Pixels.X > 0 && v.Pixels.Y > 0 {
		return v.Pixels
	}
	w, h := rasterSize(v.Width, v.Height, v.Scale)
	return image.Pt(w, h)
}

// Project maps a page rectangle to raster pixels: offset by the view's top
// left, then each edge scaled by Scale and rounded.
func (v View) Project(r image.Rectangle) image.Rectangle {
	r = r.Sub(image.Pt(v.Left, v.Top))
	scale := v.Scale
	return image.Rect(int(math.Round(float64(r.Min.X)*scale)), int(math.Round(float64(r.Min.Y)*scale)), int(math.Round(float64(r.Max.X)*scale)), int(math.Round(float64(r.Max.Y)*scale)))
}

// Unproject maps a raster pixel to the page point it was projected from: each
// coordinate divided by Scale and rounded, then offset by the view's top left.
// It inverts Project for the top left of a projected rectangle at a Scale of 1
// or more; below 1 a raster pixel covers several page pixels and the nearest is
// returned. A Scale that is not positive and finite returns the view's top
// left.
func (v View) Unproject(p image.Point) image.Point {
	if !(v.Scale > 0) || math.IsInf(v.Scale, 0) {
		return image.Pt(v.Left, v.Top)
	}
	return image.Pt(
		int(math.Round(float64(p.X)/v.Scale))+v.Left,
		int(math.Round(float64(p.Y)/v.Scale))+v.Top,
	)
}

// rasterSize is the raster of a width×height layout region at scale.
func rasterSize(width, height int, scale float64) (w, h int) {
	return max(1, int(math.Round(float64(width)*scale))), max(1, int(math.Round(float64(height)*scale)))
}
