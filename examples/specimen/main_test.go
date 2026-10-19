package main

import (
	"bytes"
	"context"
	"image"
	"testing"

	"github.com/NimbleMarkets/go-booba/actions/bubblehelp"
	nt "nimbleterminal.dev/ntpicture"
	"nimbleterminal.dev/nttypeset"
	"nimbleterminal.dev/nttypeset/examples/internal/demorun"
	"nimbleterminal.dev/nttypeset/examples/internal/pixels"
)

func TestFit(t *testing.T) {
	for _, tc := range []struct {
		in   image.Point
		w, h int
	}{
		{image.Pt(800, 600), 800, 600},
		{image.Pt(100, 100), 160, 160},
		{image.Pt(4000, 50), 1200, 160},
		{image.Pt(0, 0), 160, 160},
	} {
		if w, h := fit(tc.in); w != tc.w || h != tc.h {
			t.Errorf("fit(%v) = %d×%d, want %d×%d", tc.in, w, h, tc.w, tc.h)
		}
	}
}

func TestRenderPaintsBothPalettes(t *testing.T) {
	ctx := context.Background()
	light, err := render(ctx, "", false, 800, 600)
	if err != nil {
		t.Fatal(err)
	}
	if got := light.Bounds().Size(); got != image.Pt(800, 600) {
		t.Fatalf("size %v", got)
	}
	if pixels.CountNot(light, nttypeset.LightPalette().Background) == 0 {
		t.Fatal("light specimen has no ink")
	}
	dark, err := render(ctx, "", true, 800, 600)
	if err != nil {
		t.Fatal(err)
	}
	if pixels.CountNot(dark, nttypeset.DarkPalette().Background) == 0 {
		t.Fatal("dark specimen has no ink")
	}
	mono, err := render(ctx, "jetbrains", false, 800, 600)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(mono.Pix, light.Pix) {
		t.Fatal("the mono override changed no pixel")
	}
}

func TestSpecimenKeysFollowKitty(t *testing.T) {
	m := &viewer{pic: nt.New(nt.Options{AutoMode: true})}
	snapshot, diagnostics, err := bubblehelp.Snapshot(m.HelpKeys(), "nttypeset", "specimen", 1, bubblehelp.Options{})
	if err != nil || len(diagnostics) != 0 {
		t.Fatal(err, diagnostics)
	}
	labels := map[string]bool{}
	for _, a := range snapshot.Actions {
		labels[a.Label] = a.Enabled
	}
	for _, want := range []string{"Dark / Light", "Mono", "Keep screen & quit", "Quit"} {
		if !labels[want] {
			t.Errorf("%s not enabled", want)
		}
	}
	if labels["Glyph / Kitty"] {
		t.Fatal("toggle advertised without Kitty support")
	}
}

func TestSpecimenKeysFitBooba(t *testing.T) {
	m := &viewer{pic: nt.New(nt.Options{AutoMode: true})}
	if err := demorun.Check(stubModel{m, newBindings(true).keys()}, "specimen", "Font specimen"); err != nil {
		t.Fatal(err)
	}
}

// stubModel is a viewer whose help is fixed.
type stubModel struct {
	*viewer
	k demorun.Keys
}

func (s stubModel) HelpKeys() demorun.Keys { return s.k }

func TestRedrawBeforeGeometry(t *testing.T) {
	m := &viewer{ctx: context.Background(), pic: nt.New(nt.Options{AutoMode: true})}
	if cmd := m.redraw(); cmd != nil || m.err != nil {
		t.Fatalf("cmd %v err %v", cmd, m.err)
	}
}
