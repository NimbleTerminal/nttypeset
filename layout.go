package nttypeset

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"
)

// operation is one entry of the display list: a text run, a fill or an
// image, in page coordinates.
type operation struct {
	token          chroma.TokenType
	href           string
	rect           image.Rectangle
	run            *shaping.Output
	style          Style
	size, baseline int
	ink            color.RGBA
	role           Role
	asset          image.Image
	destination    string
	animated       bool
	fill           fillRole
	// A text run's place: the paragraph's runes, and which paragraph of
	// which block it is, the same at every width.
	text        []rune
	block, para int
	gutter      bool // A line number in the gutter: drawn muted, and not text to select.
}

// LayoutOptions are the layout choices that apply to Lines documents only;
// on any other document they are ignored.
type LayoutOptions struct {
	Numbers bool // a gutter of line numbers left of the lines
	Chop    bool // each line on one row, unwrapped, instead of wrapped
}

// Layout is an incremental layout of a Document at one width. It lays out
// blocks as far as it is asked to, and keeps them for later requests. Like
// its Engine it is not safe for concurrent use, and the Document must not be
// changed while the Layout is in use.
type Layout struct {
	starts         []int
	bounds         []image.Rectangle // Around what each laid block drew.
	glyphs         int
	spent          time.Duration
	e              *Engine
	doc            *Document
	width, y, next int
	ops            []operation
	para           int  // Paragraphs laid out of the current block.
	numbers        bool // Lay a gutter of line numbers left of the lines.
	gutter         int  // Its width in page pixels, once measured.
	chop           bool // Lay each line on one row, however long, instead of wrapping it.
	extentX        int  // The right edge of the widest operation, for panning a chopped page.
	fontGeneration int  // The engine's font generation when the layout was made.
}

// errStale is returned by a Layout made before its engine's latest font change.
var errStale = errors.New("nttypeset: layout predates a font change")

// stale returns errStale when the engine's fonts changed after the layout was
// made. Every exported method that can lay out or raster checks it first.
func (l *Layout) stale() error {
	if l.fontGeneration != l.e.fontGeneration {
		return errStale
	}
	return nil
}

// maxBlockSize is the largest Block.Size, in layout pixels.
const maxBlockSize = 1024

// noStill reports whether an Image.Still is absent: a nil interface, a nil
// pointer or slice stored in the interface, or an image with empty bounds.
func noStill(im image.Image) bool {
	if im == nil {
		return true
	}
	switch v := reflect.ValueOf(im); v.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Map, reflect.Func, reflect.Interface:
		if v.IsNil() {
			return true
		}
	}
	return im.Bounds().Empty()
}

// Layout starts an incremental layout of doc at width layout pixels. It lays
// out nothing yet. More blocks than Limits.Blocks is a *LimitError, and a
// width below 1 is an error.
func (e *Engine) Layout(doc *Document, width int, opts LayoutOptions) (*Layout, error) {
	if doc == nil {
		return nil, errors.New("nttypeset: nil document")
	}
	if len(doc.Blocks) > e.limits.Blocks {
		return nil, &LimitError{What: fmt.Sprintf("typeset document exceeds %s blocks", groupDigits(e.limits.Blocks)), Limit: int64(e.limits.Blocks)}
	}
	if width < 1 {
		return nil, errors.New("nttypeset: width must be positive")
	}
	return &Layout{e: e, doc: doc, width: width, y: Margin, numbers: opts.Numbers, chop: opts.Chop, fontGeneration: e.fontGeneration}, nil
}

// guard is deferred by every exported method that can lay out or raster,
// with ctx the request's context. It recovers the panic that gives up the
// request and makes it the method's error: ctx's error (context.Canceled or
// context.DeadlineExceeded) or a *LimitError as is, anything else wrapped.
// The method defers l.e.enter(ctx)() after it, so the engine checks ctx
// meanwhile and gives back the outer context before guard runs.
func (l *Layout) guard(ctx context.Context, err *error) {
	p := recover()
	if p == nil {
		return
	}
	if e, ok := p.(error); ok {
		var le *LimitError
		if errors.As(e, &le) || errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded) {
			*err = e
			return
		}
	}
	*err = fmt.Errorf("nttypeset: %v", p)
}

// Ensure lays out blocks until the laid height reaches bottom or the document
// ends. A cancelled ctx or a limit leaves the layout as it was before the
// block in progress, and returns the error; the next call takes it up again.
// After a font change (see Engine.SetFonts) it and every other method that
// can lay out or raster returns an error.
func (l *Layout) Ensure(ctx context.Context, bottom int) (err error) {
	if err := l.stale(); err != nil {
		return err
	}
	defer l.guard(ctx, &err)
	defer l.e.enter(ctx)()
	l.ensure(bottom)
	return nil
}

// EnsureBlock lays out blocks until block index is laid, or the document
// ends.
func (l *Layout) EnsureBlock(ctx context.Context, index int) (err error) {
	if err := l.stale(); err != nil {
		return err
	}
	defer l.guard(ctx, &err)
	defer l.e.enter(ctx)()
	for l.next <= index && l.next < len(l.doc.Blocks) {
		l.ensure(l.y + 1)
	}
	return nil
}

// Complete lays out to the end of the document, within Limits.Height.
func (l *Layout) Complete(ctx context.Context) error {
	return l.Ensure(ctx, l.e.limits.Height)
}

// Done reports whether every block is laid.
func (l *Layout) Done() bool { return l.next == len(l.doc.Blocks) }

// Laid returns how many blocks are laid so far.
func (l *Layout) Laid() int { return l.next }

// Starts returns the top y of each laid block, a copy.
func (l *Layout) Starts() []int { return append([]int(nil), l.starts...) }

// Bounds returns, for each laid block, the page rectangle around all it
// drew, without the space after it, a copy. A block that drew nothing has
// an empty one.
func (l *Layout) Bounds() []image.Rectangle { return append([]image.Rectangle(nil), l.bounds...) }

// Extent returns the laid height plus the bottom margin.
func (l *Layout) Extent() int { return l.y + Margin }

// ExtentX returns the right edge of the widest text run laid so far, for
// panning a chopped page.
func (l *Layout) ExtentX() int { return l.extentX }

// Width returns the layout's width in layout pixels.
func (l *Layout) Width() int { return l.width }

// LineAt is the line at the top of a view whose top is top: the block at the
// reading top, the page margin below the view's edge, where the first line
// sits when the view is at the top of the page. Lines are numbered from 1.
func (l *Layout) LineAt(top int) int {
	at := 0
	for i, y := range l.starts {
		if y > top+Margin {
			break
		}
		at = i
	}
	return at + 1
}

// Find lays out until the first run or image whose Href is href, or the end,
// and returns its page rectangle. The empty href finds nothing and lays out
// nothing.
func (l *Layout) Find(ctx context.Context, href string) (r image.Rectangle, found bool, err error) {
	if err := l.stale(); err != nil {
		return image.Rectangle{}, false, err
	}
	if href == "" {
		return image.Rectangle{}, false, nil
	}
	defer l.guard(ctx, &err)
	defer l.e.enter(ctx)()
	at := 0
	for {
		for ; at < len(l.ops); at++ {
			if op := l.ops[at]; op.href == href {
				return op.rect, true, nil
			}
		}
		if l.next == len(l.doc.Blocks) {
			return image.Rectangle{}, false, nil
		}
		l.ensure(l.y + 1)
	}
}

func (l *Layout) text(spans []Span, size, x, y, width int) ([]operation, int) {
	// go-text wraps one paragraph at a time. Preserve explicit line breaks,
	// including empty code lines, rather than feeding paragraph separators to it.
	if len(spans) > l.e.limits.Runs {
		panic(&LimitError{What: fmt.Sprintf("typeset block exceeds %s style runs", groupDigits(l.e.limits.Runs)), Limit: int64(l.e.limits.Runs)})
	}
	var paragraphs [][]Span
	current := []Span{}
	for _, sp := range spans {
		parts := strings.Split(sp.Text, "\n")
		for i, part := range parts {
			if i > 0 {
				paragraphs = append(paragraphs, current)
				current = nil
			}
			current = append(current, Span{Text: part, Token: sp.Token, Style: sp.Style, Ink: sp.Ink, Role: sp.Role, Href: sp.Href})
		}
	}
	if len(paragraphs) > 0 {
		paragraphs = append(paragraphs, current)
		var ops []operation
		start := y
		for _, p := range paragraphs {
			o, h := l.text(p, size, x, y, width)
			ops = append(ops, o...)
			y += h
		}
		return ops, y - start
	}
	s := l.e.shape(spans, size)
	// Link destinations do not affect shaping. Keep them out of the cache and
	// project the caller's spans onto run offsets, avoiding repeated URL copies.
	hrefs := make([]string, 0, len(s.text))
	for _, sp := range spans {
		for range []rune(sp.Text) {
			hrefs = append(hrefs, sp.Href)
		}
	}
	para := l.para
	l.para++
	if len(s.text) == 0 {
		return nil, size * 3 / 2
	}
	var wrapper shaping.LineWrapper
	// Wrapping trims whitespace glyph advances in-place. Keep the cached
	// shaping immutable across different widths.
	runsForWrap := append([]shaping.Output(nil), s.runs...)
	for i := range runsForWrap {
		runsForWrap[i].Glyphs = append([]shaping.Glyph(nil), runsForWrap[i].Glyphs...)
	}
	wrapper.Prepare(shaping.WrapConfig{Direction: di.DirectionLTR}, s.text, shaping.NewSliceIterator(runsForWrap))
	var ops []operation
	start := y
	for {
		l.e.check()
		line, done := wrapper.WrapNextLine(max(1, width))
		runs := append(shaping.Line(nil), line.Line...)
		sort.SliceStable(runs, func(i, j int) bool { return runs[i].VisualIndex < runs[j].VisualIndex })
		ascent, descent := 0, 0
		for _, r := range runs {
			ascent = max(ascent, r.LineBounds.Ascent.Ceil())
			descent = max(descent, (-r.LineBounds.Descent).Ceil())
		}
		height := max(size*3/2, ascent+descent)
		baseline := y + (height-ascent-descent)/2 + ascent
		advance := fixed.I(x)
		for _, r := range runs {
			run := r
			l.glyphs += len(r.Glyphs)
			if l.glyphs > l.e.limits.Glyphs {
				panic(&LimitError{What: fmt.Sprintf("typeset layout exceeds %s glyphs", groupDigits(l.e.limits.Glyphs)), Limit: int64(l.e.limits.Glyphs)})
			}
			run.Glyphs = append([]shaping.Glyph(nil), r.Glyphs...)
			var style Style
			var ink color.RGBA
			var rl Role
			href := ""
			var token chroma.TokenType
			if r.Runes.Offset < len(s.styles) {
				style = s.styles[r.Runes.Offset]
				token = s.tokens[r.Runes.Offset]
				href = hrefs[r.Runes.Offset]
				ink = s.inks[r.Runes.Offset]
				rl = s.roles[r.Runes.Offset]
			}
			ops = append(ops, operation{token: token, rect: image.Rect(advance.Floor(), y, (advance + r.Advance).Ceil(), y+height), run: &run, href: href, size: size, style: style, ink: ink, role: rl, baseline: baseline, text: s.text, block: l.next - 1, para: para})
			advance += r.Advance
		}
		y += height
		if done {
			break
		}
	}
	return ops, y - start
}

// gutterWidth is the room the line numbers take: the widest number the
// document has, in the monospace face at the lines' size, and a gap.
func (l *Layout) gutterWidth(size int) int {
	if l.gutter == 0 {
		digits := strings.Repeat("0", len(strconv.Itoa(len(l.doc.Blocks))))
		var advance fixed.Int26_6
		for _, run := range l.e.shape([]Span{{Text: digits, Style: Mono}}, size).runs {
			advance += run.Advance
		}
		l.gutter = advance.Ceil() + 20
	}
	return l.gutter
}

// number lays a block's line number in the gutter, at the block's top. It is
// not a paragraph of the block: places in the text stay the same with or
// without numbers.
func (l *Layout) number(n, size int) {
	digits := len(strconv.Itoa(len(l.doc.Blocks)))
	para := l.para
	ops, _ := l.text([]Span{{Text: fmt.Sprintf("%*d", digits, n), Style: Mono}}, size, Margin, l.y, l.gutter)
	l.para = para
	for i := range ops {
		ops[i].gutter, ops[i].para = true, -1
	}
	l.ops = append(l.ops, ops...)
}

func (l *Layout) rect(r image.Rectangle, f fillRole) {
	l.ops = append(l.ops, operation{rect: r, fill: f})
}

// displayListLimit is the panic for a display list over Limits.Operations
// operations or Limits.Height layout pixels.
func (l *Layout) displayListLimit() {
	limit := l.e.limits.Operations
	if len(l.ops) <= limit {
		limit = l.e.limits.Height
	}
	panic(&LimitError{What: "typeset layout exceeds display-list limit", Limit: int64(limit)})
}

// ensure lays out blocks until the laid height reaches bottom or the
// document ends. It panics to give up; the exported methods' guard recovers.
func (l *Layout) ensure(bottom int) {
	start := time.Now()
	defer func() { l.spent += time.Since(start) }()

	for l.next < len(l.doc.Blocks) && l.y < bottom {
		l.e.check()
		if l.spent+time.Since(start) > l.e.limits.Deadline {
			d := l.e.limits.Deadline
			panic(&LimitError{What: "typeset layout exceeds " + strconv.FormatFloat(d.Seconds(), 'f', -1, 64) + " seconds", Limit: int64(d)})
		}
		if len(l.ops) > l.e.limits.Operations || l.y > l.e.limits.Height {
			l.displayListLimit()
		}
		l.block()
	}
}

// block lays out the next block. If that is given up part-way, by a
// cancelled request or a limit, the layout is left as it was before the
// block, so that the next request can take it up again.
func (l *Layout) block() {
	starts, bounds, ops, y, next, glyphs := len(l.starts), len(l.bounds), len(l.ops), l.y, l.next, l.glyphs
	defer func() {
		if p := recover(); p != nil {
			l.starts, l.bounds, l.ops, l.y, l.next, l.glyphs = l.starts[:starts], l.bounds[:bounds], l.ops[:ops], y, next, glyphs
			panic(p)
		}
	}()
	{
		l.starts = append(l.starts, l.y)
		b := &l.doc.Blocks[l.next]
		l.next++
		l.para = 0
		x := Margin + b.Indent
		size := b.Size
		if size == 0 {
			size = 25
		}
		if size < 0 {
			panic(&LimitError{What: "typeset block size is negative", Limit: 0})
		}
		if size > maxBlockSize {
			panic(&LimitError{What: fmt.Sprintf("typeset block size exceeds %s pixels", groupDigits(maxBlockSize)), Limit: maxBlockSize})
		}
		if l.numbers && l.doc.Lines {
			x += l.gutterWidth(size)
			l.number(l.next, size)
		}
		width := max(32, l.width-Margin-x)
		if l.chop && l.doc.Lines {
			width = 1 << 24
		}
		switch b.Kind {
		case KindRule:
			l.rect(image.Rect(x, l.y, x+width, l.y+2), fillBorder)
			l.y += 20
		case KindImage:
			if b.Image == nil || noStill(b.Image.Still) {
				ops, h := l.text(b.Spans, 20, x, l.y, width)
				l.ops = append(l.ops, ops...)
				l.y += h + 20
				break
			}
			im := b.Image
			bounds := im.Still.Bounds()
			iw, ih := im.Bounds.Fit(bounds.Dx(), bounds.Dy())
			w := min(width, iw)
			h := max(1, ih*w/iw)
			x += im.Bounds.Offset(width, w)
			l.ops = append(l.ops, operation{rect: image.Rect(x, l.y, x+w, l.y+h), asset: im.Still, destination: im.Key, animated: im.Animated, href: b.Href})
			l.y += h + 20
		case KindTable:
			l.table(b, x, width)
		default:
			if b.Kind == KindHeading && l.next > 1 {
				l.y += 16
			}
			inset := 0
			if b.Kind == KindCode {
				inset = 16
			}
			ops, h := l.text(b.Spans, size, x+inset, l.y+inset, max(1, width-inset*2))
			if b.Kind == KindCode {
				l.rect(image.Rect(x, l.y, x+width, l.y+h+32), fillCode)
			}
			l.ops = append(l.ops, ops...)
			l.y += h + 2*inset
			if b.Kind != KindLine {
				l.y += 16
			}
		}
		// Border segments meet at block boundaries, including paragraph gaps.
		// Only the final segment stops at the content's bottom. Appending per
		// block preserves progressive layout and cancellation rollback.
		contentBottom := y
		for _, op := range l.ops[ops:] {
			contentBottom = max(contentBottom, op.rect.Max.Y)
		}
		for _, quote := range b.Quotes {
			bottom := l.y
			if quote.Last {
				bottom = contentBottom
			}
			x := Margin + quote.Indent - 18
			l.rect(image.Rect(x, y, x+4, bottom), fillMuted)
		}
		if l.y+Margin > l.e.limits.Height || len(l.ops) > l.e.limits.Operations {
			l.displayListLimit()
		}
		var drawn image.Rectangle
		for _, op := range l.ops[ops:] {
			if op.run != nil {
				l.extentX = max(l.extentX, op.rect.Max.X)
			}
			drawn = drawn.Union(op.rect)
		}
		l.bounds = append(l.bounds, drawn)
	}
}

func (l *Layout) table(b *Block, x, width int) {
	if len(b.Rows) == 0 {
		return
	}
	cols := len(b.Rows[0])
	if cols == 0 {
		return
	}
	weights := make([]int, cols)
	sum := 0
	for _, row := range b.Rows {
		for i, cell := range row {
			if i >= cols {
				break
			}
			s := l.e.shape(cell, 25)
			advance := fixed.Int26_6(0)
			for _, r := range s.runs {
				advance += r.Advance
			}
			weights[i] = max(weights[i], min(width/2, max(80, advance.Ceil()+24)))
		}
	}
	for _, w := range weights {
		sum += w
	}
	if sum == 0 {
		return
	}
	for rowIndex, row := range b.Rows {
		l.e.check()
		var ops []operation
		xx, height := x, 0
		for i, cell := range row {
			if i >= cols {
				break
			}
			w := width * weights[i] / sum
			if i == cols-1 {
				w = x + width - xx
			}
			cellOps, h := l.text(cell, 25, xx+12, l.y+10, max(1, w-24))
			ops = append(ops, cellOps...)
			height = max(height, h+20)
			xx += w
		}
		if rowIndex == 0 {
			l.rect(image.Rect(x, l.y, x+width, l.y+height), fillHeader)
		}
		l.ops = append(l.ops, ops...)
		l.rect(image.Rect(x, l.y+height-1, x+width, l.y+height), fillBorder)
		l.y += height
	}
	l.y += 20
}
