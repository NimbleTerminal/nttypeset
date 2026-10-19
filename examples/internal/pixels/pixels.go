// Package pixels counts pixels for the demos' tests.
package pixels

import (
	"image"
	"image/color"
)

// CountNot counts the pixels of img that are not c.
func CountNot(img *image.RGBA, c color.RGBA) int {
	n := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.RGBAAt(x, y) != c {
				n++
			}
		}
	}
	return n
}
