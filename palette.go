package nttypeset

import (
	"image/color"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
)

// Palette is every color the raster uses. The layout records roles and token
// types, never colors, so a different palette repaints the same layout
// without reshaping or laying it out again. Syntax colors chroma tokens; nil
// paints them in Foreground.
type Palette struct {
	Background, Foreground, Muted, Border, Code, Header, Accent, Selection color.RGBA
	Syntax                                                                 *chroma.Style
}

func rgb(n uint32) color.RGBA {
	return color.RGBA{R: uint8(n >> 16), G: uint8(n >> 8), B: uint8(n), A: 255}
}

// LightPalette returns gloss's light base palette with chroma's "github"
// syntax style.
func LightPalette() Palette {
	return Palette{rgb(0xffffff), rgb(0x232a32), rgb(0x596575), rgb(0xd2d2d2), rgb(0xf5f7f9), rgb(0xedf1f5), rgb(0x55408c), rgb(0xb4d5fe), styles.Get("github")}
}

// DarkPalette returns gloss's dark base palette with chroma's "github-dark"
// syntax style.
func DarkPalette() Palette {
	return Palette{rgb(0x181a1e), rgb(0xe6edf3), rgb(0x9da7b3), rgb(0x424a55), rgb(0x242830), rgb(0x303641), rgb(0xb9a5ff), rgb(0x264f78), styles.Get("github-dark")}
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
