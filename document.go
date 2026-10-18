package nttypeset

import (
	"image"
	"image/color"

	"github.com/alecthomas/chroma/v2"
)

// Margin is the layout pixels around the page; the first block starts at y = Margin.
const Margin = 50

// Style is a set of face flags. Bold|Italic|Mono|Strike; the low three bits
// select one of the eight loaded faces.
type Style uint8

// The Style flags.
const (
	// Bold selects the bold face.
	Bold Style = 1 << iota
	// Italic selects the italic face.
	Italic
	// Mono selects the monospace face.
	Mono
	// Strike draws a line through the text; it does not select a face.
	Strike
)

// Role names a semantic ink the palette resolves at paint time.
type Role uint8

// The Role values.
const (
	// RoleText is the palette's Foreground.
	RoleText Role = iota
	// RoleAccent is the ink for headings and links.
	RoleAccent
)

// Span is a run of text with one style, ink and optional link.
type Span struct {
	Text  string
	Style Style
	Role  Role
	Ink   color.RGBA       // explicit ink; zero defers to Role and Token
	Token chroma.TokenType // nonzero paints from Palette.Syntax
	Href  string           // link destination; not part of shaping
}

// Kind says what a Block is and how it is laid out.
type Kind uint8

// The Kind values. The quote and image kinds are KindQuote and KindImage
// because Quote and Image name types.
const (
	// Paragraph is a wrapped run of spans.
	Paragraph Kind = iota
	// Heading is a paragraph set in a heading size.
	Heading
	// KindQuote is a paragraph inside a blockquote.
	KindQuote
	// Code is a code block: inset, code-background fill.
	Code
	// Line is one line of a Lines document: no gap after it.
	Line
	// Rule is a horizontal rule.
	Rule
	// KindImage is an image block; its Block.Image is set.
	KindImage
	// Table is a table; its cells are in Block.Rows.
	Table
)

// Quote is one blockquote bar beside a block: its indent, and whether this is
// the last block of that quote (the bar stops at the content, not the gap).
type Quote struct {
	Indent int
	Last   bool
}

// Bounds are optional HTML-style limits on an image: fit without enlarging,
// align left, center or right. Zero means natural size, left.
type Bounds struct {
	Width, Height int
	Align         string
}

// Fit applies the bounds to a natural size without stretching or enlarging
// the image, keeping its aspect ratio.
func (s Bounds) Fit(w, h int) (int, int) {
	if s.Width > 0 && w > s.Width {
		h = max(1, h*s.Width/w)
		w = s.Width
	}
	if s.Height > 0 && h > s.Height {
		w = max(1, w*s.Height/h)
		h = s.Height
	}
	return w, h
}

// Offset returns the horizontal offset of an image of the given width within
// the available width, for the "center" and "right" alignments; any other
// alignment returns 0.
func (s Bounds) Offset(available, width int) int {
	switch s.Align {
	case "center":
		return max(0, (available-width)/2)
	case "right":
		return max(0, available-width)
	}
	return 0
}

// Image is a reserved rectangle. Still is drawn by Raster unless Animated and
// the raster omits animated images; nil Still draws the block's Spans as a
// placeholder. Key is reported back by Layout.Images.
type Image struct {
	Key      string
	Still    image.Image
	Bounds   Bounds
	Animated bool
}

// Block is one laid-out unit of a Document: a paragraph, heading, quote line,
// code block, rule, image or table.
type Block struct {
	Kind   Kind
	Spans  []Span
	Rows   [][][]Span // Table: rows of cells of spans; the first row is the header
	Size   int        // font size in layout pixels; zero is 25
	Indent int        // layout pixels left of the margin
	Quotes []Quote
	Image  *Image // Kind == KindImage
	Href   string // Image: the link around it
}

// Document is what the layout consumes: blocks in reading order. Lines says
// the blocks are a file's lines, one each, which enables gutter numbers and
// chop in LayoutOptions.
type Document struct {
	Blocks []Block
	Lines  bool
}
