package nttypeset

import (
	"nimbleterminal.dev/nttypeset/internal/testfonts"
	"slices"
	"testing"
)

type extraSource struct {
	*countingSource
	names []string
}

func (s extraSource) Fallbacks() []string { return s.names }

func TestExtraFallbackLoadsAndFailures(t *testing.T) {
	for style := Style(0); style < 8; style++ {
		t.Run(style.Variant()+string(rune('0'+style)), func(t *testing.T) {
			var order []string
			src := extraSource{countingSource: &countingSource{inner: funcSource(func(family, variant string) ([]byte, error) {
				order = append(order, family)
				if family == "available" {
					family = testLook.Body
				}
				return testfonts.Source.Face(family, variant)
			})}, names: []string{"unavailable", "available", "unavailable"}}
			f, err := newFaces(src, testLook)
			if err != nil {
				t.Fatal(err)
			}
			order = nil
			m := fallbackMap{f, style}
			for _, r := range "漢字漢" {
				if got := m.ResolveFace(r); got != f.faces[style] {
					t.Fatal("missing rune must return primary")
				}
			}
			builtin := fallbackBody
			if style&Mono != 0 {
				builtin = fallbackCode
			}
			if !slices.Equal(order, []string{builtin, "unavailable", "available"}) {
				t.Fatalf("load order = %v", order)
			}
			if len(f.extra) != 2 || !f.extra[0].failed[style] || f.extra[1].faces[style] == nil {
				t.Fatal("fallback load state not cached")
			}
			if len(f.missing) != 2 || len(f.fallbackUsed) != 0 {
				t.Fatalf("missing=%v used=%v", f.missing, f.fallbackUsed)
			}
			for _, name := range []string{"unavailable", "available"} {
				if src.calls[name+"/"+style.Variant()] != 1 {
					t.Fatalf("wrong variant or repeated load: %v", src.calls)
				}
			}
		})
	}
}

type panickingFallbackSource struct{ FaceSource }

func (panickingFallbackSource) Fallbacks() []string { panic("broken fallback list") }

func TestFallbackSourcePanic(t *testing.T) {
	if _, err := newFaces(panickingFallbackSource{testfonts.Source}, testLook); err == nil {
		t.Fatal("expected source error")
	}
}
