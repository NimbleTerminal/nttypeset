package nttypeset

import (
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
	"nimbleterminal.dev/nttypeset/internal/testfonts"
)

var testLook = FontLook{Name: testfonts.Name, Body: testfonts.Body, Code: testfonts.Code}

type countingSource struct {
	inner FaceSource
	calls map[string]int
}

func (c *countingSource) Face(family, variant string) ([]byte, error) {
	if c.calls == nil {
		c.calls = map[string]int{}
	}
	c.calls[family+"/"+variant]++
	return c.inner.Face(family, variant)
}

type funcSource func(family, variant string) ([]byte, error)

func (f funcSource) Face(family, variant string) ([]byte, error) { return f(family, variant) }

func TestResolveFont(t *testing.T) {
	look, err := ResolveFont("", "")
	if err != nil || look.Name != "readable" || look.Body != "Literata" || look.Code != "Iosevka" {
		t.Fatalf("default = %+v, %v", look, err)
	}
	look, err = ResolveFont("readable", "jetbrains")
	if err != nil || look.Code != "JetBrains Mono" || look.Body != "Literata" {
		t.Fatalf("jetbrains = %+v, %v", look, err)
	}
	_, err = ResolveFont("missing", "")
	if err == nil || err.Error() != `unknown font look "missing"` {
		t.Fatalf("unknown look err = %v", err)
	}
	_, err = ResolveFont("readable", "other")
	if err == nil || err.Error() != `mono override must be "jetbrains"` {
		t.Fatalf("bad override err = %v", err)
	}
	if got := FontKey("", ""); got != "readable/" {
		t.Fatalf("FontKey = %q", got)
	}
	if got := FontKey("compact", "jetbrains"); got != "compact/jetbrains" {
		t.Fatalf("FontKey = %q", got)
	}
}

func TestFontLooksTable(t *testing.T) {
	want := []FontLook{
		{"readable", "Literata", "Iosevka"},
		{"editorial", "Newsreader", "Iosevka"},
		{"technical", "IBM Plex Sans", "IBM Plex Mono"},
		{"compact", "Source Serif 4", "JetBrains Mono"},
	}
	got := FontLooks()
	if len(got) != len(want) {
		t.Fatalf("len = %d", len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("look %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	got[0].Body = "changed"
	if FontLooks()[0].Body != "Literata" {
		t.Error("FontLooks returned the internal table")
	}
}

func TestStyleVariant(t *testing.T) {
	cases := []struct {
		s    Style
		want string
	}{
		{0, VariantRegular},
		{Bold | Mono, VariantBold},
		{Italic, VariantItalic},
		{Bold | Italic | Strike, VariantBoldItalic},
	}
	for _, c := range cases {
		if got := c.s.Variant(); got != c.want {
			t.Errorf("Style(%d).Variant() = %q, want %q", c.s, got, c.want)
		}
	}
	if VariantRegular != "Regular" || VariantBold != "Bold" || VariantItalic != "Italic" || VariantBoldItalic != "BoldItalic" {
		t.Error("variant constants changed")
	}
}

func TestLoadFacesLoadsEightFaces(t *testing.T) {
	src := &countingSource{inner: testfonts.Source}
	faces, err := loadFaces(src, testLook)
	if err != nil {
		t.Fatal(err)
	}
	for i, f := range faces {
		if f == nil {
			t.Errorf("face %d is nil", i)
		}
	}
	if src.calls["Go Mono/BoldItalic"] == 0 || src.calls["Go/Regular"] == 0 {
		t.Errorf("calls = %v", src.calls)
	}
}

func TestLoadFacesNamesMissingFamily(t *testing.T) {
	look, err := ResolveFont("", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = loadFaces(testfonts.Source, look)
	if err == nil || !strings.Contains(err.Error(), "Literata") {
		t.Fatalf("err = %v, want one naming Literata", err)
	}
	if !strings.Contains(err.Error(), VariantRegular) {
		t.Errorf("err = %v, want it to name the variant", err)
	}
}

func TestLoadFacesRejectsBadBytes(t *testing.T) {
	src := funcSource(func(family, variant string) ([]byte, error) { return []byte("not a font"), nil })
	_, err := loadFaces(src, testLook)
	if err == nil || !strings.Contains(err.Error(), "font Go "+VariantRegular+":") {
		t.Fatalf("err = %v, want one naming the family and variant", err)
	}
}

// corruptSource serves bad bytes for every face of the built-in families.
type corruptSource []byte

func (c corruptSource) Face(family, variant string) ([]byte, error) { return c, nil }

func TestLoadFacesRejectsCorruptBytes(t *testing.T) {
	half := goregular.TTF[:len(goregular.TTF)/2]
	flipped := xorEvery(goregular.TTF, 0, 50, 0xff)
	// This corruption makes go-text's ParseTTF panic (slice bounds out of
	// range); the two above are returned as errors by ParseTTF itself.
	panicky := xorEvery(goregular.TTF, 10, 1, 0x01)
	for name, data := range map[string][]byte{"truncated": half, "flipped": flipped, "panicky": panicky} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if p := recover(); p != nil {
					t.Fatalf("NewEngine panicked: %v", p)
				}
			}()
			e, err := NewEngine(WithFonts(corruptSource(data)))
			if err == nil || e != nil {
				t.Fatalf("NewEngine = %v, %v; want an error", e, err)
			}
			if !strings.Contains(err.Error(), "font Literata "+VariantRegular+":") {
				t.Fatalf("err = %v, want one naming the family and variant", err)
			}
		})
	}
}

// xorEvery returns a copy of data with width bytes XORed by x at every
// 1,000 bytes from offset.
func xorEvery(data []byte, offset, width int, x byte) []byte {
	out := append([]byte(nil), data...)
	for i := offset; i < len(out); i += 1000 {
		for j := i; j < min(i+width, len(out)); j++ {
			out[j] ^= x
		}
	}
	return out
}

func TestLoadFaceRecoversSourcePanic(t *testing.T) {
	src := funcSource(func(family, variant string) ([]byte, error) { panic("source broke") })
	_, err := loadFace(src, "Go", VariantBold)
	if err == nil || err.Error() != "font Go Bold: source broke" {
		t.Fatalf("err = %v", err)
	}
}

func TestFallbackRecoversCorruptBytes(t *testing.T) {
	// The primary faces load; the fallback family's bytes make go-text panic.
	panicky := xorEvery(goregular.TTF, 10, 1, 0x01)
	src := funcSource(func(family, variant string) ([]byte, error) {
		if family == "Literata" {
			return panicky, nil
		}
		return testfonts.Source.Face(family, variant)
	})
	f, err := newFaces(src, testLook)
	if err != nil {
		t.Fatal(err)
	}
	m := fallbackMap{f: f, style: 0}
	if got := m.ResolveFace('漢'); got != f.faces[0] {
		t.Fatal("did not return the primary face")
	}
	if !f.fallbackFailed[0] || !f.missing['漢'] {
		t.Fatalf("failed=%v missing=%v", f.fallbackFailed, f.missing)
	}
}

func TestFallbackFailureCountsMissing(t *testing.T) {
	src := &countingSource{inner: testfonts.Source}
	f, err := newFaces(src, testLook)
	if err != nil {
		t.Fatal(err)
	}
	m := fallbackMap{f: f, style: 0}
	primary := f.faces[0]
	if got := m.ResolveFace('漢'); got != primary {
		t.Fatal("did not return the primary face")
	}
	if len(f.fallbackUsed) != 0 {
		t.Errorf("fallbackUsed = %v", f.fallbackUsed)
	}
	if !f.missing['漢'] {
		t.Errorf("missing = %v", f.missing)
	}
	if !f.fallbackFailed[0] {
		t.Error("fallbackFailed[0] not set")
	}
	n := src.calls["Literata/Regular"]
	if n != 1 {
		t.Fatalf("Literata calls = %d, want 1", n)
	}
	m.ResolveFace('漢')
	m.ResolveFace('字')
	if src.calls["Literata/Regular"] != n {
		t.Errorf("Face called again for a failed fallback: %v", src.calls)
	}
	if !f.missing['字'] {
		t.Error("second missing rune not recorded")
	}
	// Mono style asks for Iosevka and remembers its own failure.
	mm := fallbackMap{f: f, style: Mono}
	mm.ResolveFace('漢')
	if src.calls["Iosevka/Regular"] != 1 || !f.fallbackFailed[Mono] {
		t.Errorf("mono fallback: %v", src.calls)
	}
}

func TestFallbackControlAndSpaceUsePrimary(t *testing.T) {
	src := &countingSource{inner: testfonts.Source}
	f, err := newFaces(src, testLook)
	if err != nil {
		t.Fatal(err)
	}
	m := fallbackMap{f: f, style: Bold}
	for _, r := range []rune{'\t', '\n', ' ', ' ', 0x1f} {
		if m.ResolveFace(r) != f.faces[Bold] {
			t.Errorf("%U did not resolve to primary", r)
		}
	}
	if len(f.missing) != 0 || f.fallbackFailed[Bold] || len(src.calls) != 8 {
		t.Errorf("missing=%v failed=%v calls=%v", f.missing, f.fallbackFailed, src.calls)
	}
}

func TestFallbackLoadsOncePerStyleWhenSourceHasFamily(t *testing.T) {
	// A source that serves Literata and Iosevka lets the fallback load. The Go
	// fonts share one glyph set, so the rune is still missing, but the fallback
	// face is loaded once and kept.
	src := &countingSource{inner: funcSource(func(family, variant string) ([]byte, error) {
		switch family {
		case "Literata":
			family = testfonts.Body
		case "Iosevka":
			family = testfonts.Code
		}
		return testfonts.Source.Face(family, variant)
	})}
	f, err := newFaces(src, testLook)
	if err != nil {
		t.Fatal(err)
	}
	m := fallbackMap{f: f, style: Italic}
	m.ResolveFace('漢')
	m.ResolveFace('字')
	if f.fallback[Italic] == nil || f.fallbackFailed[Italic] {
		t.Error("fallback should have loaded")
	}
	if src.calls["Literata/Italic"] != 1 {
		t.Errorf("calls = %v", src.calls)
	}
	if !f.missing['漢'] || len(f.fallbackUsed) != 0 {
		t.Errorf("missing=%v used=%v", f.missing, f.fallbackUsed)
	}
}

func TestMissingCappedAt32(t *testing.T) {
	f, err := newFaces(testfonts.Source, testLook)
	if err != nil {
		t.Fatal(err)
	}
	m := fallbackMap{f: f, style: 0}
	for r := rune(0x4e00); r < 0x4e00+100; r++ {
		m.ResolveFace(r)
	}
	if len(f.missing) != 32 {
		t.Errorf("missing = %d, want 32", len(f.missing))
	}
}

func TestNoLigatures(t *testing.T) {
	if len(noLigatures) != 4 {
		t.Fatalf("len = %d", len(noLigatures))
	}
	for _, f := range noLigatures {
		if f.Value != 0 {
			t.Errorf("%v enabled", f.Tag)
		}
	}
}
