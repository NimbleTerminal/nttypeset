package nttypeset

import (
	"image"
	"testing"
)

func TestUnprojectInvertsProject(t *testing.T) {
	r := image.Rect(130, 2040, 530, 2100)
	for _, scale := range []float64{1, 1.5} {
		v := View{Top: 2000, Left: 100, Width: 800, Height: 600, Scale: scale}
		if got := v.Unproject(v.Project(r).Min); got != r.Min {
			t.Errorf("scale %v: unproject %v, want %v", scale, got, r.Min)
		}
	}
	v := View{Top: 2000, Left: 100, Width: 800, Height: 600, Scale: 0.125}
	last := v.Size().Sub(image.Pt(1, 1))
	got := v.Unproject(last)
	if got.X < v.Left || got.X >= v.Left+v.Width || got.Y < v.Top || got.Y >= v.Top+v.Height {
		t.Errorf("last pixel %v unprojects to %v, outside the region", last, got)
	}
	if got := (View{Top: 7, Left: 3, Width: 10, Height: 10}).Unproject(image.Pt(5, 5)); got != image.Pt(3, 7) {
		t.Errorf("scale 0: %v", got)
	}
}
