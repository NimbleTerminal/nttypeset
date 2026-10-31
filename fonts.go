package nttypeset

import (
	"bytes"
	"fmt"
	"unicode"

	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/shaping"
)

// The face variants a FaceSource is asked for. They are plain strings so a
// face source need not import this package.
const (
	// VariantRegular is the upright regular weight.
	VariantRegular = "Regular"
	// VariantBold is the upright bold weight.
	VariantBold = "Bold"
	// VariantItalic is the italic regular weight.
	VariantItalic = "Italic"
	// VariantBoldItalic is the italic bold weight.
	VariantBoldItalic = "BoldItalic"
)

// Variant names the face variant the Bold and Italic bits select. Mono and
// Strike do not change the variant.
func (s Style) Variant() string {
	switch {
	case s&Bold != 0 && s&Italic != 0:
		return VariantBoldItalic
	case s&Bold != 0:
		return VariantBold
	case s&Italic != 0:
		return VariantItalic
	}
	return VariantRegular
}

// FaceSource supplies TrueType or OpenType bytes for a family and variant. It
// uses strings only, so a font package implements it without importing
// nttypeset.
//
// The engine asks for the families of the built-in looks: Literata,
// Newsreader, IBM Plex Sans, Source Serif 4, Iosevka, IBM Plex Mono and
// JetBrains Mono, each in the four variants. Literata and Iosevka are also the
// glyph fallback. A source serves its own faces under these names; any family
// it does not supply makes the looks that use it fail to load. Sources may
// additionally implement FallbackSource to supply more glyph-fallback families.
type FaceSource interface {
	Face(family, variant string) ([]byte, error)
}

// FallbackSource optionally lists additional glyph-fallback families in preference
// order. A FaceSource implementing it is queried when loading a look. Each family
// is tried after Literata (body) or Iosevka (mono), in the run's variant. Families
// that cannot load are skipped and remembered until the look changes.
type FallbackSource interface {
	Fallbacks() []string
}

// FontLook pairs a body family with a code family, by name. The family names
// are what the engine asks its FaceSource for. The engine accepts only the
// built-in looks, chosen by Name; a FontLook made by hand is not a new look.
type FontLook struct {
	Name, Body, Code string
}

var fontLooks = []FontLook{
	{"readable", "Literata", "Iosevka"},
	{"editorial", "Newsreader", "Iosevka"},
	{"technical", "IBM Plex Sans", "IBM Plex Mono"},
	{"compact", "Source Serif 4", "JetBrains Mono"},
}

// The families glyph fallback asks the FaceSource for.
const (
	fallbackBody = "Literata"
	fallbackCode = "Iosevka"
)

// FontLooks returns the built-in looks: readable, editorial, technical,
// compact. They are the only looks WithLook and SetFonts accept.
func FontLooks() []FontLook { return append([]FontLook(nil), fontLooks...) }

// ResolveFont returns the look called name; the empty name is readable. The
// override is empty or "jetbrains", which swaps the code family to JetBrains Mono.
func ResolveFont(name, override string) (FontLook, error) {
	if name == "" {
		name = "readable"
	}
	if override != "" && override != "jetbrains" {
		return FontLook{}, fmt.Errorf(`nttypeset: mono override must be "jetbrains"`)
	}
	for _, look := range fontLooks {
		if look.Name == name {
			if override == "jetbrains" {
				look.Code = "JetBrains Mono"
			}
			return look, nil
		}
	}
	return FontLook{}, fmt.Errorf("nttypeset: unknown font look %q", name)
}

// FontKey is the cache key for a look and override: name + "/" + override,
// with the empty name standing for readable.
func FontKey(name, override string) string {
	if name == "" {
		name = "readable"
	}
	return name + "/" + override
}

// noLigatures turns off the ligature and contextual features so text keeps
// one glyph per character.
var noLigatures = []shaping.FontFeature{
	{Tag: opentype.MustNewTag("liga"), Value: 0},
	{Tag: opentype.MustNewTag("clig"), Value: 0},
	{Tag: opentype.MustNewTag("dlig"), Value: 0},
	{Tag: opentype.MustNewTag("calt"), Value: 0},
}

// loadFace fetches and parses one family and variant. A panic in the source
// or in the parser, which go-text's ParseTTF raises on some truncated or
// corrupt fonts, is returned as an error naming the family and variant.
func loadFace(src FaceSource, family, variant string) (face *font.Face, err error) {
	defer func() {
		if p := recover(); p != nil {
			face, err = nil, fmt.Errorf("font %s %s: %v", family, variant, p)
		}
	}()
	data, err := src.Face(family, variant)
	if err != nil {
		return nil, fmt.Errorf("font %s %s: %w", family, variant, err)
	}
	face, err = font.ParseTTF(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("font %s %s: %w", family, variant, err)
	}
	return face, nil
}

// loadFaces loads the eight faces of a look. Index i is a Style whose low
// three bits pick the family (Mono for code) and the variant.
func loadFaces(src FaceSource, look FontLook) ([8]*font.Face, error) {
	var faces [8]*font.Face
	for i := range faces {
		family := look.Body
		if Style(i)&Mono != 0 {
			family = look.Code
		}
		face, err := loadFace(src, family, Style(i).Variant())
		if err != nil {
			return faces, err
		}
		faces[i] = face
	}
	return faces, nil
}

// maxMissing caps how many distinct missing runes are remembered.
const maxMissing = 32

// faces holds a look's primary faces and the lazily loaded fallbacks.
type faces struct {
	faces          [8]*font.Face
	fallback       [8]*font.Face
	fallbackFailed [8]bool
	extra          []fallbackFamily
	missing        map[rune]bool
	fallbackUsed   map[string]bool
	src            FaceSource
}

// fallbackFamily caches each extra family's faces, including failed loads.
// Mono and body styles have separate slots, just like the built-in fallback.
type fallbackFamily struct {
	name   string
	faces  [8]*font.Face
	failed [8]bool
}

// sourceFallbacks also contains panics from optional source implementations.
func sourceFallbacks(src FallbackSource) (names []string, err error) {
	defer func() {
		if p := recover(); p != nil {
			names, err = nil, fmt.Errorf("font fallbacks: %v", p)
		}
	}()
	return src.Fallbacks(), nil
}

// newFaces loads a look's faces from src with empty fallback state.
func newFaces(src FaceSource, look FontLook) (*faces, error) {
	loaded, err := loadFaces(src, look)
	if err != nil {
		return nil, err
	}
	var extra []fallbackFamily
	if provider, ok := src.(FallbackSource); ok {
		names, err := sourceFallbacks(provider)
		if err != nil {
			return nil, err
		}
		seen := map[string]bool{}
		for _, name := range names {
			if !seen[name] {
				extra = append(extra, fallbackFamily{name: name})
				seen[name] = true
			}
		}
	}
	return &faces{
		faces:        loaded,
		extra:        extra,
		missing:      map[rune]bool{},
		fallbackUsed: map[string]bool{},
		src:          src,
	}, nil
}

// fallbackMap resolves runes for one style, falling back to Literata or
// Iosevka, then the source's extra families. It implements shaping.Fontmap.
type fallbackMap struct {
	f     *faces
	style Style
}

var _ shaping.Fontmap = fallbackMap{}

// ResolveFace returns the face to shape r with. A fallback family the source
// cannot supply is remembered per style. Only when the entire chain fails does
// the rune count as missing and resolve to the primary face.
func (m fallbackMap) ResolveFace(r rune) *font.Face {
	i := int(m.style & 7)
	f := m.f
	primary := f.faces[i]
	if _, ok := primary.NominalGlyph(r); ok {
		return primary
	}
	if unicode.IsControl(r) || unicode.IsSpace(r) {
		return primary
	}
	family := fallbackBody
	if i&int(Mono) != 0 {
		family = fallbackCode
	}
	if f.fallback[i] == nil && !f.fallbackFailed[i] {
		face, err := loadFace(f.src, family, Style(i).Variant())
		if err != nil {
			f.fallbackFailed[i] = true
		} else {
			f.fallback[i] = face
		}
	}
	if fb := f.fallback[i]; fb != nil {
		if _, ok := fb.NominalGlyph(r); ok {
			f.fallbackUsed[family] = true
			return fb
		}
	}
	for j := range f.extra {
		extra := &f.extra[j]
		if extra.faces[i] == nil && !extra.failed[i] {
			face, err := loadFace(f.src, extra.name, Style(i).Variant())
			if err != nil {
				extra.failed[i] = true
			} else {
				extra.faces[i] = face
			}
		}
		if fb := extra.faces[i]; fb != nil {
			if _, ok := fb.NominalGlyph(r); ok {
				f.fallbackUsed[extra.name] = true
				return fb
			}
		}
	}
	if len(f.missing) < maxMissing {
		f.missing[r] = true
	}
	return primary
}
