package goldmark

import (
	"context"
	"fmt"
	"strings"

	"github.com/alecthomas/chroma/v2"
	emojiast "github.com/yuin/goldmark-emoji/ast"
	"github.com/yuin/goldmark/ast"
	ext "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/util"
	"nimbleterminal.dev/nttypeset"
)

// Options carry what the Markdown tree cannot: the images, their bounds and
// the syntax style.
type Options struct {
	// Images by destination, exactly as written in the Markdown. A destination
	// that is absent, or whose Still is nil, lays out as the placeholder
	// "[image unavailable: dest]" in italic. Key, Still and Animated are copied;
	// Bounds always comes from the Bounds hook.
	Images map[string]nttypeset.Image
	// Bounds returns the bounds for an image node, from attributes the caller's
	// normalization set. Nil means natural size, left.
	Bounds func(*ast.Image) nttypeset.Bounds
	// Syntax decides bold and italic for fenced code. Nil leaves code plain.
	Syntax *chroma.Style
}

const (
	maxBlocks    = 32768
	maxCodeLines = 8192
)

// walker turns a goldmark tree into the blocks nttypeset lays out.
type walker struct {
	ctx    context.Context
	opts   Options
	blocks []nttypeset.Block
}

// walkAbort carries an error out of the recursive walk.
type walkAbort struct{ err error }

// Build walks a parsed tree into a Document. It does not parse. Headings are
// sized 50 down to 29 by level; lists and quotes indent 24 layout pixels.
// More than 32,768 blocks, or a code block of more than 8,192 lines, is a
// *nttypeset.LimitError (it matches nttypeset.ErrLimit). A cancelled ctx
// returns ctx.Err() unwrapped. Build never panics.
//
// The walk recurses with the tree's depth, and Build sets no depth limit:
// callers must bound the nesting of the Markdown they parse (gloss rejects
// deeply nested input before parsing).
func Build(ctx context.Context, root ast.Node, source []byte, opts Options) (doc *nttypeset.Document, err error) {
	w := &walker{ctx: ctx, opts: opts}
	defer func() {
		if p := recover(); p != nil {
			doc = nil
			if a, ok := p.(walkAbort); ok {
				err = a.err
			} else {
				err = fmt.Errorf("goldmark: %v", p)
			}
		}
	}()
	w.walk(root, source, 0)
	w.checkBlocks(len(w.blocks) > maxBlocks)
	return &nttypeset.Document{Blocks: w.blocks}, nil
}

func (w *walker) checkBlocks(exceeded bool) {
	if exceeded {
		panic(walkAbort{&nttypeset.LimitError{What: "typeset document exceeds 32,768 blocks", Limit: maxBlocks}})
	}
}

func (w *walker) walk(n ast.Node, data []byte, indent int) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if w.ctx != nil {
			if err := w.ctx.Err(); err != nil {
				panic(walkAbort{err})
			}
		}
		w.checkBlocks(len(w.blocks) > maxBlocks)
		switch v := c.(type) {
		case *ast.Heading:
			heading := inline(v, data, nttypeset.Bold)
			for i := range heading {
				heading[i].Role = nttypeset.RoleAccent
			}
			w.blocks = append(w.blocks, nttypeset.Block{Kind: nttypeset.KindHeading, Spans: heading, Size: []int{50, 46, 42, 38, 33, 29}[v.Level-1], Indent: indent})
		case *ast.Paragraph, *ast.TextBlock:
			if c.ChildCount() == 1 {
				if im := standaloneImage(c.FirstChild()); im != nil {
					w.blocks = append(w.blocks, w.imageBlock(im, indent))
					continue
				}
			}
			w.blocks = append(w.blocks, nttypeset.Block{Kind: nttypeset.KindParagraph, Spans: inline(c, data, 0), Indent: indent})
		case *ast.FencedCodeBlock, *ast.CodeBlock:
			if c.Lines().Len() > maxCodeLines {
				panic(walkAbort{&nttypeset.LimitError{What: "typeset code block exceeds 8,192 lines", Limit: maxCodeLines}})
			}
			var b strings.Builder
			for i := 0; i < c.Lines().Len(); i++ {
				s := c.Lines().At(i)
				b.Write(s.Value(data))
			}
			language := ""
			if fence, ok := c.(*ast.FencedCodeBlock); ok {
				language = string(fence.Language(data))
			}
			w.blocks = append(w.blocks, nttypeset.Block{Kind: nttypeset.KindCode, Spans: nttypeset.Highlight(w.ctx, strings.TrimSuffix(b.String(), "\n"), language, w.opts.Syntax), Size: 25, Indent: indent})
		case *ast.Blockquote:
			start := len(w.blocks)
			w.walk(c, data, indent+24)
			for i := start; i < len(w.blocks); i++ {
				w.blocks[i].Quotes = append(w.blocks[i].Quotes, nttypeset.Quote{Indent: indent + 24, Last: i == len(w.blocks)-1})
				if w.blocks[i].Kind == nttypeset.KindParagraph {
					w.blocks[i].Kind = nttypeset.KindQuote
				}
			}
		case *ast.List:
			index := v.Start
			if index < 1 {
				index = 1
			}
			for li := v.FirstChild(); li != nil; li = li.NextSibling() {
				start := len(w.blocks)
				w.walk(li, data, indent+24)
				if start < len(w.blocks) {
					marker := "• "
					if v.IsOrdered() {
						marker = fmt.Sprintf("%d. ", index)
					}
					w.blocks[start].Spans = append([]nttypeset.Span{{Text: marker}}, w.blocks[start].Spans...)
				}
				index++
			}
		case *ext.Table:
			b := nttypeset.Block{Kind: nttypeset.KindTable, Indent: indent}
			for row := v.FirstChild(); row != nil; row = row.NextSibling() {
				var cells [][]nttypeset.Span
				var style nttypeset.Style
				if _, ok := row.(*ext.TableHeader); ok {
					style = nttypeset.Bold
				}
				for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
					cells = append(cells, inline(cell, data, style))
				}
				b.Rows = append(b.Rows, cells)
			}
			w.blocks = append(w.blocks, b)
		case *ast.ThematicBreak:
			w.blocks = append(w.blocks, nttypeset.Block{Kind: nttypeset.KindRule})
		case *ast.HTMLBlock: // no raw HTML layout
		default:
			w.walk(c, data, indent)
		}
	}
}

// imageBlock is the block for an image that stands alone in its paragraph.
// The placeholder spans are laid out when Still is nil.
func (w *walker) imageBlock(im *ast.Image, indent int) nttypeset.Block {
	dest := string(im.Destination)
	img := nttypeset.Image{Key: dest}
	if supplied, ok := w.opts.Images[dest]; ok {
		img = supplied
		if img.Key == "" {
			img.Key = dest
		}
	}
	img.Bounds = nttypeset.Bounds{}
	if w.opts.Bounds != nil {
		img.Bounds = w.opts.Bounds(im)
	}
	return nttypeset.Block{
		Kind:   nttypeset.KindImage,
		Image:  &img,
		Href:   imageLink(im),
		Spans:  []nttypeset.Span{{Text: "[image unavailable: " + dest + "]", Style: nttypeset.Italic}},
		Indent: indent,
	}
}

func inline(n ast.Node, data []byte, style nttypeset.Style) []nttypeset.Span {
	var out []nttypeset.Span
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch v := c.(type) {
		case *ast.Text:
			raw := v.Segment.Value(data)
			if style&nttypeset.Mono == 0 {
				raw = util.ResolveEntityNames(util.ResolveNumericReferences(util.UnescapePunctuations(raw)))
			}
			s := string(raw)
			if v.HardLineBreak() {
				s += "\n"
			} else if v.SoftLineBreak() {
				s += " "
			}
			out = append(out, nttypeset.Span{Text: s, Style: style})
		case *ast.String:
			out = append(out, nttypeset.Span{Text: string(v.Value), Style: style})
		case *emojiast.Emoji:
			out = append(out, nttypeset.Span{Text: string(v.Value.Unicode), Style: style})
		case *ast.Emphasis:
			flag := nttypeset.Italic
			if v.Level == 2 {
				flag = nttypeset.Bold
			}
			out = append(out, inline(v, data, style|flag)...)
		case *ast.CodeSpan:
			out = append(out, inline(v, data, style|nttypeset.Mono)...)
		case *ext.Strikethrough:
			out = append(out, inline(v, data, style|nttypeset.Strike)...)
		case *ext.TaskCheckBox:
			s := "[ ] "
			if v.IsChecked {
				s = "[x] "
			}
			out = append(out, nttypeset.Span{Text: s, Style: style | nttypeset.Mono})
		case *ast.Link:
			linked := inline(v, data, style)
			for i := range linked {
				linked[i].Href = string(v.Destination)
				linked[i].Role = nttypeset.RoleAccent
			}
			out = append(out, linked...)
		case *ast.AutoLink:
			out = append(out, nttypeset.Span{Text: string(v.Label(data)), Href: string(v.URL(data)), Style: style, Role: nttypeset.RoleAccent})
		case *ast.Image:
			out = append(out, nttypeset.Span{Text: "[image: " + string(v.Destination) + "]", Style: style})
		default:
			out = append(out, inline(c, data, style)...)
		}
	}
	return out
}

// standaloneImage returns the image when n is an image, or an image-only
// link. A link around an image changes its destination when clicked, not its
// visual content; mixed prose keeps its inline layout.
func standaloneImage(n ast.Node) *ast.Image {
	for n != nil {
		switch v := n.(type) {
		case *ast.Image:
			return v
		case *ast.Link:
			if n.ChildCount() != 1 {
				return nil
			}
			n = n.FirstChild()
		default:
			return nil
		}
	}
	return nil
}

func imageLink(im *ast.Image) string {
	if link, ok := im.Parent().(*ast.Link); ok {
		return string(link.Destination)
	}
	return ""
}
