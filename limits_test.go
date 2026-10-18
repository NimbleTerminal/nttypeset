package nttypeset

import (
	"errors"
	"testing"
	"time"
)

func TestLimitsDefaults(t *testing.T) {
	got := Limits{}.withDefaults()
	want := Limits{
		Blocks:     32768,
		Runs:       32768,
		Runes:      16384,
		Glyphs:     262144,
		Operations: 32768,
		Height:     11250000,
		Pixels:     32 << 20,
		Deadline:   15 * time.Second,
		ShapeCache: 32 << 20,
		GlyphCache: 32 << 20,
	}
	if got != want {
		t.Fatalf("defaults = %+v, want %+v", got, want)
	}
}

func TestLimitsOnlyLower(t *testing.T) {
	if g := (Limits{Glyphs: 1 << 30}).withDefaults().Glyphs; g != 262144 {
		t.Fatalf("raised Glyphs = %d, want 262144", g)
	}
	if g := (Limits{Glyphs: 100}).withDefaults().Glyphs; g != 100 {
		t.Fatalf("lowered Glyphs = %d, want 100", g)
	}
	if g := (Limits{Glyphs: -5}).withDefaults().Glyphs; g != 262144 {
		t.Fatalf("negative Glyphs = %d, want the default 262144", g)
	}
	if d := (Limits{Deadline: -time.Second}).withDefaults().Deadline; d != 15*time.Second {
		t.Fatalf("negative Deadline = %v, want 15s", d)
	}
	if d := (Limits{Deadline: time.Hour}).withDefaults().Deadline; d != 15*time.Second {
		t.Fatalf("raised Deadline = %v, want 15s", d)
	}
	if d := (Limits{Deadline: time.Second}).withDefaults().Deadline; d != time.Second {
		t.Fatalf("lowered Deadline = %v, want 1s", d)
	}
}

func TestLimitErrorIs(t *testing.T) {
	err := error(&LimitError{What: "typeset layout exceeds 15 seconds"})
	if !errors.Is(err, ErrLimit) {
		t.Fatal("errors.Is(err, ErrLimit) = false")
	}
	if err.Error() != "typeset layout exceeds 15 seconds" {
		t.Fatalf("message = %q", err.Error())
	}
	if errors.Is(err, errors.New("other")) {
		t.Fatal("matched an unrelated error")
	}
}

func TestBoundsFit(t *testing.T) {
	if w, h := (Bounds{Width: 100}).Fit(400, 200); w != 100 || h != 50 {
		t.Fatalf("Fit = %d,%d, want 100,50", w, h)
	}
	if w, h := (Bounds{}).Fit(400, 200); w != 400 || h != 200 {
		t.Fatalf("zero Fit = %d,%d, want 400,200", w, h)
	}
	if w, h := (Bounds{Height: 50}).Fit(400, 200); w != 100 || h != 50 {
		t.Fatalf("Height Fit = %d,%d, want 100,50", w, h)
	}
	if w, h := (Bounds{Width: 1000}).Fit(400, 200); w != 400 || h != 200 {
		t.Fatalf("enlarging Fit = %d,%d, want 400,200", w, h)
	}
	if o := (Bounds{Align: "center"}).Offset(300, 100); o != 100 {
		t.Fatalf("center Offset = %d, want 100", o)
	}
	if o := (Bounds{Align: "right"}).Offset(300, 100); o != 200 {
		t.Fatalf("right Offset = %d, want 200", o)
	}
	if o := (Bounds{Align: "right"}).Offset(50, 100); o != 0 {
		t.Fatalf("overflow Offset = %d, want 0", o)
	}
	if o := (Bounds{}).Offset(300, 100); o != 0 {
		t.Fatalf("left Offset = %d, want 0", o)
	}
}
