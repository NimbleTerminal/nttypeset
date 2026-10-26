package nttypeset

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"sort"
)

// Insets are nonnegative layout pixels on each edge (at most 65,536).
type Insets struct{ Top, Right, Bottom, Left int }

// Border is one solid rectangular edge. A transparent Color reserves Width
// without painting. Width has the same limits as Insets.
type Border struct {
	Width int
	Color color.RGBA
}

// Section decorates blocks [Start, End). Ranges must be nonempty, nested or
// disjoint, never crossing. Equal ranges nest in input order (outer first).
// Sections do not add selectable text. Padding and borders reduce content width.
// Background includes padding; borders are inside the outer rectangle.
// SpaceBefore/SpaceAfter are outside the decoration and do not collapse.
// There are at most Limits.Blocks sections and at most 64 nesting levels.
type Section struct {
	Start, End               int
	SpaceBefore, SpaceAfter  int
	SourceID                 string
	Padding                  Insets
	Background               color.RGBA
	Top, Right, Bottom, Left Border
}

// BlockFormat supplies resolved block styling in layout pixels. A nonnil value
// replaces the kind's default before/after spacing, code inset and code fill.
// Size and span inks/styles still come from Block.Size and Block.Spans.
// LineHeight zero keeps default leading; positive leading is enlarged if needed
// to fit font metrics. Tables retain their cell padding, header fill and rules.
// Spacing does not collapse. Values must be between zero and 65,536.
type BlockFormat struct {
	LineHeight, SpaceBefore, SpaceAfter int
	Padding                             Insets
	Background                          color.RGBA
}

type anchorRef struct {
	index   int
	section bool
}
type sectionSegment struct {
	index, left, right, top, bottom int
	first, last                     bool
}

func validSpacing(values ...int) error {
	for _, v := range values {
		if v < 0 || v > 65536 {
			return &LimitError{What: "typeset spacing outside 0..65,536 pixels", Limit: 65536}
		}
	}
	return nil
}
func validInsets(v Insets) error { return validSpacing(v.Top, v.Right, v.Bottom, v.Left) }

func (l *Layout) initSections() error {
	if len(l.doc.Sections) > l.e.limits.Blocks {
		return &LimitError{What: "typeset section count exceeds block limit", Limit: int64(l.e.limits.Blocks)}
	}
	l.anchors = make(map[string]anchorRef)
	add := func(id string, ref anchorRef) error {
		if id == "" {
			return nil
		}
		if _, ok := l.anchors[id]; ok {
			return fmt.Errorf("nttypeset: duplicate source ID %q", id)
		}
		l.anchors[id] = ref
		return nil
	}
	for i, b := range l.doc.Blocks {
		if err := add(b.SourceID, anchorRef{index: i}); err != nil {
			return err
		}
		if f := b.Format; f != nil {
			if err := validSpacing(f.LineHeight, f.SpaceBefore, f.SpaceAfter); err != nil {
				return err
			}
			if err := validInsets(f.Padding); err != nil {
				return err
			}
		}
	}
	if len(l.doc.Sections) == 0 {
		return nil
	}
	l.sectionStarts = make([][]int, len(l.doc.Blocks))
	l.sectionBounds = make([]image.Rectangle, len(l.doc.Sections))
	order := make([]int, len(l.doc.Sections))
	for i, s := range l.doc.Sections {
		order[i] = i
		if s.Start < 0 || s.End <= s.Start || s.End > len(l.doc.Blocks) {
			return fmt.Errorf("nttypeset: invalid section range [%d,%d)", s.Start, s.End)
		}
		if err := validInsets(s.Padding); err != nil {
			return err
		}
		if err := validSpacing(s.SpaceBefore, s.SpaceAfter, s.Top.Width, s.Right.Width, s.Bottom.Width, s.Left.Width); err != nil {
			return err
		}
		if err := add(s.SourceID, anchorRef{index: i, section: true}); err != nil {
			return err
		}
	}
	sort.SliceStable(order, func(i, j int) bool {
		a, b := l.doc.Sections[order[i]], l.doc.Sections[order[j]]
		if a.Start != b.Start {
			return a.Start < b.Start
		}
		return a.End > b.End
	})
	var stack []int
	for _, i := range order {
		s := l.doc.Sections[i]
		for len(stack) > 0 && l.doc.Sections[stack[len(stack)-1]].End <= s.Start {
			stack = stack[:len(stack)-1]
		}
		if len(stack) > 0 && s.End > l.doc.Sections[stack[len(stack)-1]].End {
			return fmt.Errorf("nttypeset: crossing section ranges")
		}
		stack = append(stack, i)
		if len(stack) > 64 {
			return &LimitError{What: "typeset section depth exceeds 64", Limit: 64}
		}
		l.sectionStarts[s.Start] = append(l.sectionStarts[s.Start], i)
	}
	return nil
}

func (l *Layout) openSections() ([]sectionSegment, int, int) {
	segs := append([]sectionSegment(nil), l.active...)
	left, right := 0, 0
	for i := range segs {
		segs[i].top = l.y
		segs[i].first = false
		s := l.doc.Sections[segs[i].index]
		left += s.Left.Width + s.Padding.Left
		right += s.Right.Width + s.Padding.Right
	}
	if len(l.sectionStarts) > 0 {
		for _, i := range l.sectionStarts[l.next] {
			s := l.doc.Sections[i]
			l.y += s.SpaceBefore
			segs = append(segs, sectionSegment{index: i, left: Margin + left, right: max(Margin+left, l.width-Margin-right), top: l.y, first: true})
			left += s.Left.Width + s.Padding.Left
			right += s.Right.Width + s.Padding.Right
			l.y += s.Top.Width + s.Padding.Top
		}
	}
	return segs, left, right
}

func (l *Layout) solid(r image.Rectangle, c color.RGBA) {
	if c.A != 0 && !r.Empty() {
		l.ops = append(l.ops, operation{rect: r, ink: c, explicitFill: true})
	}
}

func (l *Layout) closeSections(segs []sectionSegment, at int) {
	for i := len(segs) - 1; i >= 0; i-- {
		s := l.doc.Sections[segs[i].index]
		segs[i].last = s.End == l.next
		if segs[i].last {
			l.y += s.Padding.Bottom + s.Bottom.Width
		}
		segs[i].bottom = l.y
		if segs[i].last {
			l.y += s.SpaceAfter
		}
	}
	if len(segs) == 0 {
		return
	}
	content := append([]operation(nil), l.ops[at:]...)
	l.ops = l.ops[:at]
	for _, seg := range segs {
		s := l.doc.Sections[seg.index]
		r := image.Rect(seg.left, seg.top, seg.right, seg.bottom)
		l.solid(r, s.Background)
		l.solid(image.Rect(r.Min.X, r.Min.Y, min(r.Max.X, r.Min.X+s.Left.Width), r.Max.Y), s.Left.Color)
		l.solid(image.Rect(max(r.Min.X, r.Max.X-s.Right.Width), r.Min.Y, r.Max.X, r.Max.Y), s.Right.Color)
		if seg.first {
			l.solid(image.Rect(r.Min.X, r.Min.Y, r.Max.X, min(r.Max.Y, r.Min.Y+s.Top.Width)), s.Top.Color)
		}
		if seg.last {
			l.solid(image.Rect(r.Min.X, max(r.Min.Y, r.Max.Y-s.Bottom.Width), r.Max.X, r.Max.Y), s.Bottom.Color)
		}
	}
	l.ops = append(l.ops, content...)
}
func (l *Layout) commitSections(segs []sectionSegment) {
	l.active = nil
	for _, seg := range segs {
		l.sectionBounds[seg.index] = l.sectionBounds[seg.index].Union(image.Rect(seg.left, seg.top, seg.right, seg.bottom))
		if !seg.last {
			l.active = append(l.active, seg)
		}
	}
}
func blockGap(b *Block, fallback int) int {
	if b.Format != nil {
		return b.Format.SpaceAfter
	}
	return fallback
}
func placeholderSize(b *Block) int {
	if b.Size != 0 {
		return b.Size
	}
	return 20
}
func tableSize(b *Block) int {
	if b.Size != 0 {
		return b.Size
	}
	return 25
}

// Anchor is the geometry currently laid for a source ID, in page pixels.
// Complete says the entire block or section is laid. An unlaid anchor has an
// empty Rect and Complete false. Geometry is a bounding box, including gaps.
type Anchor struct {
	Rect     image.Rectangle
	Complete bool
}

// AnchorBounds looks up an opaque ID without laying anything out. With ensure,
// it lays through the identified block or section end. Absent or empty IDs
// return found=false and never lay anything out. Hosts omit hidden content and
// its IDs. Duplicate nonempty IDs across blocks and sections are rejected by
// Engine.Layout. Reflow by making a new Layout of the same Document.
func (l *Layout) AnchorBounds(ctx context.Context, id string, ensure bool) (a Anchor, found bool, err error) {
	if err := l.stale(); err != nil {
		return a, false, err
	}
	defer l.guard(ctx, &err)
	defer l.e.enter(ctx)()
	ref, ok := l.anchors[id]
	if !ok {
		return a, false, nil
	}
	end := ref.index + 1
	if ref.section {
		end = l.doc.Sections[ref.index].End
	}
	if ensure {
		for l.next < end {
			l.ensure(l.y + 1)
		}
	}
	a.Complete = l.next >= end
	if ref.section {
		a.Rect = l.sectionBounds[ref.index]
	} else if ref.index < len(l.bounds) {
		a.Rect = l.bounds[ref.index]
	}
	return a, true, nil
}

// SourceAt resolves a selection Place (including Selected.Places) to the block's
// source ID, falling back to its innermost identified section. Point places
// resolve to nearest laid text as Select does. It may lay through a placed
// block. Empty IDs return found=false. TextAt hits can be passed as placed
// Block/Para/Rune coordinates. Inline IDs are represented by separate blocks.
func (l *Layout) SourceAt(ctx context.Context, p Place) (id string, found bool, err error) {
	if err := l.stale(); err != nil {
		return "", false, err
	}
	defer l.guard(ctx, &err)
	defer l.e.enter(ctx)()
	block := p.Block
	if p.Placed {
		if block < 0 || block >= len(l.doc.Blocks) {
			return "", false, nil
		}
		for l.next <= block {
			l.ensure(l.y + 1)
		}
	} else {
		c, ok := l.place(p)
		if !ok {
			return "", false, nil
		}
		block = l.ops[c.op].block
	}
	id = l.doc.Blocks[block].SourceID
	if id != "" {
		return id, true, nil
	}
	bestStart, bestEnd := -1, len(l.doc.Blocks)+1
	for _, s := range l.doc.Sections {
		if s.SourceID != "" && s.Start <= block && block < s.End && s.Start >= bestStart && s.End <= bestEnd {
			id = s.SourceID
			bestStart, bestEnd = s.Start, s.End
		}
	}
	return id, id != "", nil
}
