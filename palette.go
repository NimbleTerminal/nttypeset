package nttypeset

import (
	"image/color"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
)

// Palette is every color the raster uses. The layout records roles and token
// types, never colors, so a different palette repaints the same layout
// without reshaping or laying it out again. Syntax colors chroma tokens; nil
// paints them in Foreground. Mark tints a Mark; left zero, it is the
// selection's color halfway to the page.
type Palette struct {
	Background, Foreground, Muted, Border, Code, Header, Accent, Selection color.RGBA
	Mark                                                                   color.RGBA
	Syntax                                                                 *chroma.Style
}

// mark is the tint of a Mark: Mark when set, else the selection's color
// halfway to the page.
func (p Palette) mark() color.RGBA {
	if p.Mark != (color.RGBA{}) {
		return p.Mark
	}
	mid := func(a, b uint8) uint8 { return uint8((int(a) + int(b)) / 2) }
	s, b := p.Selection, p.Background
	return color.RGBA{mid(s.R, b.R), mid(s.G, b.G), mid(s.B, b.B), 255}
}

func rgb(n uint32) color.RGBA {
	return color.RGBA{R: uint8(n >> 16), G: uint8(n >> 8), B: uint8(n), A: 255}
}

// LightPalette returns gloss's light base palette with chroma's "github"
// syntax style.
func LightPalette() Palette {
	return Palette{Background: rgb(0xffffff), Foreground: rgb(0x232a32), Muted: rgb(0x596575), Border: rgb(0xd2d2d2), Code: rgb(0xf5f7f9), Header: rgb(0xedf1f5), Accent: rgb(0x55408c), Selection: rgb(0xb4d5fe), Syntax: styles.Get("github")}
}

// DarkPalette returns gloss's dark base palette with chroma's "github-dark"
// syntax style.
func DarkPalette() Palette {
	return Palette{Background: rgb(0x181a1e), Foreground: rgb(0xe6edf3), Muted: rgb(0x9da7b3), Border: rgb(0x424a55), Code: rgb(0x242830), Header: rgb(0x303641), Accent: rgb(0xb9a5ff), Selection: rgb(0x264f78), Syntax: styles.Get("github-dark")}
}

// A fillRole is a semantic fill color, resolved against the palette at paint time.
type fillRole uint8

const (
	fillNone fillRole = iota
	fillBorder
	fillCode
	fillHeader
	fillMuted
)

func (p Palette) fillColor(f fillRole) color.RGBA {
	switch f {
	case fillBorder:
		return p.Border
	case fillCode:
		return p.Code
	case fillHeader:
		return p.Header
	case fillMuted:
		return p.Muted
	}
	return p.Background
}

// runInk is the color a text run is painted in. A syntax token wins over the
// span's own ink, an accent role over both, and a gutter number is muted.
func (p Palette) runInk(op *operation) color.RGBA {
	ink := p.Foreground
	if op.ink.A != 0 {
		ink = op.ink
	}
	if op.token != 0 {
		ink = p.Foreground
		if p.Syntax != nil {
			if c := p.Syntax.Get(op.token).Colour; c.IsSet() {
				ink = color.RGBA{c.Red(), c.Green(), c.Blue(), 255}
			}
		}
	}
	if op.role == RoleAccent {
		ink = p.Accent
	}
	if op.gutter {
		ink = p.Muted
	}
	return ink
}
