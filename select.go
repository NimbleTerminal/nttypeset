package nttypeset

import (
	"context"
	"image"
	"strings"

	"golang.org/x/image/math/fixed"
)

// Place is one end of a selection. A viewer gives a point on the page, in
// layout pixels; the result gives it back placed in the text, as a rune of a
// paragraph of a block, which is the same place at every width, zoom and
// theme. A Placed end is used as a place and its Point is ignored, but kept.
type Place struct {
	Point             image.Point // a page point in layout pixels, used when not Placed
	Block, Para, Rune int         // a rune of a paragraph of a block, used when Placed
	Placed            bool
}

// Selection is the text between two places, in either order.
type Selection struct {
	Anchor, Focus Place
}

// caret is a place in a layout: a text operation and a rune offset in its run.
type caret struct{ op, rune int }

func (c caret) before(d caret) bool { return c.op < d.op || (c.op == d.op && c.rune < d.rune) }

// Selected is a Selection resolved against one Layout: its ends in reading
// order. It is valid for that layout only, and is drawn by passing it in
// RasterOptions.Selection.
type Selected struct {
	layout     *Layout
	start, end caret
	places     Selection
}

// Select places both ends of s in the laid-out text, laying out as far as a
// placed end asks. It returns nil, nil when a place is not in the document, or
// when a point has no text to land on. Gutter line numbers are never text.
func (l *Layout) Select(ctx context.Context, s Selection) (sel *Selected, err error) {
	if err := l.stale(); err != nil {
		return nil, err
	}
	defer l.guard(ctx, &err)
	defer l.e.enter(ctx)()
	a, ok := l.place(s.Anchor)
	if !ok {
		return nil, nil
	}
	b, ok := l.place(s.Focus)
	if !ok {
		return nil, nil
	}
	out := &Selected{layout: l, start: a, end: b, places: Selection{Anchor: l.placed(s.Anchor, a), Focus: l.placed(s.Focus, b)}}
	if b.before(a) {
		out.start, out.end = b, a
	}
	return out, nil
}

// Places returns both ends as places in the text, in the order they were
// given, with the points they were given kept. A nil Selected has none.
func (s *Selected) Places() Selection {
	if s == nil {
		return Selection{}
	}
	return s.places
}

// Text returns the selected text in reading order: runs of a line joined, a
// paragraph's original text across visual wraps, paragraphs of a block by a
// line break (cells of a table row by a tab), and blocks by a blank line.
// A nil Selected has no text.
func (s *Selected) Text() string {
	if s == nil {
		return ""
	}
	return s.layout.selected(s)
}

func (l *Layout) placed(p Place, c caret) Place {
	op := &l.ops[c.op]
	return Place{Point: p.Point, Block: op.block, Para: op.para, Rune: op.run.Runes.Offset + c.rune, Placed: true}
}

func (l *Layout) place(p Place) (caret, bool) {
	if !p.Placed {
		return l.caretAt(p.Point)
	}
	for l.next <= p.Block && l.next < len(l.doc.Blocks) {
		l.ensure(l.y + 1)
	}
	last := -1
	for i := range l.ops {
		op := &l.ops[i]
		if op.run == nil || op.gutter || op.block != p.Block || op.para != p.Para {
			continue
		}
		if r := op.run.Runes; p.Rune >= r.Offset && p.Rune < r.Offset+r.Count {
			return caret{i, p.Rune - r.Offset}, true
		}
		last = i
	}
	if last < 0 {
		return caret{}, false
	}
	return caret{last, l.ops[last].run.Runes.Count}, true
}

// caretAt is the text place nearest a point: on the line the point is on,
// or else the nearest line, before the glyph the point is in or after it,
// or at the line's start or end when the point is beside it.
func (l *Layout) caretAt(p image.Point) (caret, bool) {
	best, dist := -1, 0
	for i := range l.ops {
		op := &l.ops[i]
		if op.run == nil || op.gutter {
			continue
		}
		d := max(op.rect.Min.Y-p.Y, p.Y-op.rect.Max.Y+1, 0)
		if best < 0 || d < dist {
			best, dist = i, d
		}
	}
	if best < 0 {
		return caret{}, false
	}
	line := l.ops[best].rect
	first, last := best, best
	for i := range l.ops {
		op := &l.ops[i]
		if op.run == nil || op.gutter || op.rect.Min.Y != line.Min.Y || op.rect.Max.Y != line.Max.Y {
			continue
		}
		if op.rect.Min.X <= p.X && p.X < op.rect.Max.X {
			return caret{i, runRune(op, p.X)}, true
		}
		if op.rect.Min.X < l.ops[first].rect.Min.X {
			first = i
		}
		if op.rect.Max.X > l.ops[last].rect.Max.X {
			last = i
		}
	}
	if p.X < l.ops[first].rect.Min.X {
		return caret{first, 0}, true
	}
	if p.X >= l.ops[last].rect.Max.X {
		return caret{last, l.ops[last].run.Runes.Count}, true
	}
	// In a gap between runs on the line: the end of the one before it.
	gap := first
	for i := range l.ops {
		op := &l.ops[i]
		if op.run != nil && !op.gutter && op.rect.Min.Y == line.Min.Y && op.rect.Max.Y == line.Max.Y && op.rect.Max.X <= p.X && op.rect.Max.X > l.ops[gap].rect.Max.X {
			gap = i
		}
	}
	return caret{gap, l.ops[gap].run.Runes.Count}, true
}

// runRune is the rune boundary of a run nearest to page x: before the glyph
// cluster x is in the left half of, after it otherwise.
// Hit is the text under a page point: a rune of a paragraph of a block, and
// the whole of that paragraph's text, the same at every width.
type Hit struct {
	Block, Para, Rune int
	Text              string
}

// TextAt returns the text under page point p, false when no text is drawn
// there: in a margin, between lines or runs, on a picture or a line number.
// Unlike a selection's place, it does not take the nearest text.
func (l *Layout) TextAt(p image.Point) (Hit, bool) {
	for i := range l.ops {
		op := &l.ops[i]
		if op.run == nil || op.gutter || !p.In(op.rect) {
			continue
		}
		pos, at := fixed.I(op.rect.Min.X), fixed.I(p.X)
		for _, g := range op.run.Glyphs {
			if at < pos+g.Advance {
				return Hit{Block: op.block, Para: op.para, Rune: g.TextIndex(), Text: string(op.text)}, true
			}
			pos += g.Advance
		}
	}
	return Hit{}, false
}

func runRune(op *operation, x int) int {
	pos, at := fixed.I(op.rect.Min.X), fixed.I(x)
	for _, g := range op.run.Glyphs {
		if at < pos+g.Advance/2 {
			return max(0, min(op.run.Runes.Count, g.TextIndex()-op.run.Runes.Offset))
		}
		pos += g.Advance
	}
	return op.run.Runes.Count
}

// runX is the page x of a rune boundary of a run: the left edge of the
// cluster that begins at it, or the run's right edge.
func runX(op *operation, r int) int {
	if r <= 0 {
		return op.rect.Min.X
	}
	pos := fixed.I(op.rect.Min.X)
	for _, g := range op.run.Glyphs {
		if g.TextIndex()-op.run.Runes.Offset >= r {
			return pos.Floor()
		}
		pos += g.Advance
	}
	return op.rect.Max.X
}

// span is the page x range of the selected part of text operation i. A nil
// selection, or one of another layout, selects nothing.
func (s *Selected) span(l *Layout, i int, op *operation) (x0, x1 int, ok bool) {
	if s == nil || s.layout != l || op.run == nil || op.gutter || i < s.start.op || i > s.end.op {
		return 0, 0, false
	}
	from, to := 0, op.run.Runes.Count
	if i == s.start.op {
		from = s.start.rune
	}
	if i == s.end.op {
		to = s.end.rune
	}
	if from >= to {
		return 0, 0, false
	}
	x0, x1 = op.rect.Min.X, op.rect.Max.X
	if from > 0 {
		x0 = runX(op, from)
	}
	if to < op.run.Runes.Count {
		x1 = runX(op, to)
	}
	return x0, x1, true
}

// selected is the selection's text in reading order.
func (l *Layout) selected(s *Selected) string {
	var b strings.Builder
	var prev *operation
	prevEnd := 0
	for i := s.start.op; i <= s.end.op && i < len(l.ops); i++ {
		op := &l.ops[i]
		if op.run == nil || op.gutter {
			continue
		}
		from, to := 0, op.run.Runes.Count
		if i == s.start.op {
			from = s.start.rune
		}
		if i == s.end.op {
			to = s.end.rune
		}
		if from >= to {
			continue
		}
		text := string(op.text[op.run.Runes.Offset+from : op.run.Runes.Offset+to])
		if prev != nil {
			switch {
			case op.block != prev.block:
				b.WriteString("\n\n")
			case op.para != prev.para && op.rect.Min.Y == prev.rect.Min.Y:
				b.WriteByte('\t')
			case op.para != prev.para:
				b.WriteByte('\n')
			case op.run.Runes.Offset+from > prevEnd:
				// Wrapping may omit whitespace from the drawing runs. Recover
				// it from logical text; a break inside a token adds nothing.
				b.WriteString(string(op.text[prevEnd : op.run.Runes.Offset+from]))
			}
		}
		b.WriteString(text)
		prev = op
		prevEnd = op.run.Runes.Offset + to
	}
	return b.String()
}
