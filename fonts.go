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
type FaceSource interface {
	Face(family, variant string) ([]byte, error)
}

// FontLook pairs a body family with a code family, by name. The family names
// are what the engine asks its FaceSource for.
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

// FontLooks returns the built-in looks: readable, editorial, technical, compact.
func FontLooks() []FontLook { return append([]FontLook(nil), fontLooks...) }

// ResolveFont returns the look called name; the empty name is readable. The
// override is empty or "jetbrains", which swaps the code family to JetBrains Mono.
func ResolveFont(name, override string) (FontLook, error) {
	if name == "" {
		name = "readable"
	}
	if override != "" && override != "jetbrains" {
		return FontLook{}, fmt.Errorf(`mono override must be "jetbrains"`)
	}
	for _, look := range fontLooks {
		if look.Name == name {
			if override == "jetbrains" {
				look.Code = "JetBrains Mono"
			}
			return look, nil
		}
	}
	return FontLook{}, fmt.Errorf("unknown font look %q", name)
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

// loadFace fetches and parses one family and variant.
func loadFace(src FaceSource, family, variant string) (*font.Face, error) {
	data, err := src.Face(family, variant)
	if err != nil {
		return nil, fmt.Errorf("font %s %s: %w", family, variant, err)
	}
	face, err := font.ParseTTF(bytes.NewReader(data))
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
	missing        map[rune]bool
	fallbackUsed   map[string]bool
	src            FaceSource
}

// newFaces loads a look's faces from src with empty fallback state.
func newFaces(src FaceSource, look FontLook) (*faces, error) {
	loaded, err := loadFaces(src, look)
	if err != nil {
		return nil, err
	}
	return &faces{
		faces:        loaded,
		missing:      map[rune]bool{},
		fallbackUsed: map[string]bool{},
		src:          src,
	}, nil
}

// fallbackMap resolves runes for one style, falling back to Literata or
// Iosevka when the primary face lacks the rune. It implements shaping.Fontmap.
type fallbackMap struct {
	f     *faces
	style Style
}

var _ shaping.Fontmap = fallbackMap{}

// ResolveFace returns the face to shape r with. A fallback family the source
// cannot supply is remembered per style; the rune then counts as missing and
// the primary face is returned.
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
	if len(f.missing) < maxMissing {
		f.missing[r] = true
	}
	return primary
}
