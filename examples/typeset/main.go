// Command typeset is the nttypeset demo. It lays out a Markdown or text file
// with the ntfonts faces and shows the page in the terminal through ntpicture,
// scrolling and restyling from the keyboard, or writes the top of the page as
// a PNG with -o.
//
//	go -C examples run ./typeset [flags] [FILE]
//
// Without FILE it shows the embedded sample page.
package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"image"
	_ "image/gif"  // decode GIF stills referenced by Markdown
	_ "image/jpeg" // decode JPEG stills referenced by Markdown
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	gm "github.com/yuin/goldmark"
	emoji "github.com/yuin/goldmark-emoji"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
	"nimbleterminal.dev/ntfonts"
	"nimbleterminal.dev/nttypeset"
	ntgoldmark "nimbleterminal.dev/nttypeset/goldmark"
)

//go:embed sample.md landscape.png
var sample embed.FS

// options are the command line.
type options struct {
	font, mono, lang, sel, out string
	dark, numbers, chop        bool
	width, height              int
	scale                      float64
}

func main() {
	var o options
	flag.StringVar(&o.font, "font", "readable", "look: readable, editorial, technical or compact")
	flag.StringVar(&o.mono, "mono", "", `code font: "jetbrains", or empty for the look's own`)
	flag.BoolVar(&o.dark, "dark", false, "paint with the dark palette")
	flag.BoolVar(&o.numbers, "numbers", false, "a gutter of line numbers (text files)")
	flag.BoolVar(&o.chop, "chop", false, "unwrapped lines (text files)")
	flag.StringVar(&o.sel, "select", "", "highlight a selection, BLOCK.RUNE-BLOCK.RUNE (for example 1.0-2.12)")
	flag.StringVar(&o.lang, "lang", "", "chroma language of a text file; by default from its name")
	flag.StringVar(&o.out, "o", "", "write the top of the page to this PNG instead of showing it")
	flag.IntVar(&o.width, "width", 800, "page width in layout pixels")
	flag.IntVar(&o.height, "height", 1125, "PNG height in layout pixels (with -o)")
	flag.Float64Var(&o.scale, "scale", 1, "raster pixels per layout pixel (with -o)")
	flag.Parse()
	if err := run(o, flag.Arg(0)); err != nil {
		fmt.Fprintln(os.Stderr, "typeset:", err)
		os.Exit(1)
	}
}

func run(o options, name string) error {
	ctx := context.Background()
	p, err := open(ctx, name, o)
	if err != nil {
		return err
	}
	eng, err := nttypeset.NewEngine(nttypeset.WithFonts(ntfonts.Embedded), nttypeset.WithLook(o.font, o.mono))
	if err != nil {
		return err
	}
	if o.out == "" {
		return show(ctx, p, eng, o)
	}
	lay, err := eng.Layout(p.doc, o.width, p.opts)
	if err != nil {
		return err
	}
	img, err := p.render(ctx, lay, nttypeset.View{Width: o.width, Height: o.height, Scale: o.scale}, o.dark, p.sel, nil)
	if err != nil {
		return err
	}
	f, err := os.Create(o.out)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// page is a document ready to lay out, with the layout options and the
// selection the flags asked for.
type page struct {
	title string
	doc   *nttypeset.Document
	opts  nttypeset.LayoutOptions
	sel   *nttypeset.Selection
}

// open reads name, or the embedded sample when name is empty, into a page.
// Markdown images are resolved against the file's directory.
func open(ctx context.Context, name string, o options) (*page, error) {
	var (
		src []byte
		dir fs.FS
		err error
	)
	if name == "" {
		name = "sample.md"
		src, err = sample.ReadFile(name)
		dir = sample
	} else {
		src, err = os.ReadFile(name)
		dir = os.DirFS(filepath.Dir(name))
	}
	if err != nil {
		return nil, err
	}
	p := &page{title: filepath.Base(name), opts: nttypeset.LayoutOptions{Numbers: o.numbers, Chop: o.chop}}
	if o.sel != "" {
		s, err := parseSelection(o.sel)
		if err != nil {
			return nil, err
		}
		p.sel = &s
	}
	p.doc, err = build(ctx, name, src, dir, o.lang)
	return p, err
}

// build turns src into a Document: a Markdown file through the goldmark
// adapter, with the local images it references decoded from dir; anything
// else as a Lines document highlighted as lang, or as the language chroma
// matches to the file name.
func build(ctx context.Context, name string, src []byte, dir fs.FS, lang string) (*nttypeset.Document, error) {
	if ext := strings.ToLower(filepath.Ext(name)); lang == "" && (ext == ".md" || ext == ".markdown") {
		md := gm.New(gm.WithExtensions(extension.GFM, emoji.Emoji))
		root := md.Parser().Parse(text.NewReader(src))
		return ntgoldmark.Build(ctx, root, src, ntgoldmark.Options{Images: images(root, dir), Syntax: styles.Get("github")})
	}
	if lang == "" {
		if lexer := lexers.Match(name); lexer != nil {
			lang = lexer.Config().Name
		}
	}
	return nttypeset.Lines(ctx, string(src), lang, styles.Get("github"))
}

// images decodes every image the tree references, by destination, from dir.
// One that is missing or undecodable is left out and lays out as the
// adapter's placeholder.
func images(root ast.Node, dir fs.FS) map[string]nttypeset.Image {
	out := map[string]nttypeset.Image{}
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		img, ok := n.(*ast.Image)
		if !ok || !entering {
			return ast.WalkContinue, nil
		}
		dest := string(img.Destination)
		if _, seen := out[dest]; seen {
			return ast.WalkContinue, nil
		}
		if still := decode(dir, dest); still != nil {
			out[dest] = nttypeset.Image{Key: dest, Still: still}
		}
		return ast.WalkContinue, nil
	})
	return out
}

// decode returns the image at name in dir, or nil.
func decode(dir fs.FS, name string) image.Image {
	if dir == nil {
		return nil
	}
	f, err := dir.Open(name)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	still, _, err := image.Decode(f)
	if err != nil {
		return nil
	}
	return still
}

// render lays out as far as v needs and paints it with the light or dark
// palette, sel, if its places land on text, and marks behind their text.
func (p *page) render(ctx context.Context, lay *nttypeset.Layout, v nttypeset.View, dark bool, sel *nttypeset.Selection, marks []nttypeset.Mark) (*image.RGBA, error) {
	opts := nttypeset.RasterOptions{Palette: nttypeset.LightPalette(), Marks: marks}
	if dark {
		opts.Palette = nttypeset.DarkPalette()
	}
	if sel != nil {
		selected, err := lay.Select(ctx, *sel)
		if err != nil {
			return nil, err
		}
		opts.Selection = selected
	}
	return lay.Raster(ctx, v, opts)
}

// parseSelection reads BLOCK.RUNE-BLOCK.RUNE into a Selection whose ends are
// placed in the first paragraph of each block.
func parseSelection(s string) (nttypeset.Selection, error) {
	var sel nttypeset.Selection
	anchor, focus, ok := strings.Cut(s, "-")
	if !ok {
		return sel, fmt.Errorf("selection %q: want BLOCK.RUNE-BLOCK.RUNE", s)
	}
	var err error
	if sel.Anchor, err = parsePlace(anchor); err != nil {
		return sel, err
	}
	if sel.Focus, err = parsePlace(focus); err != nil {
		return sel, err
	}
	return sel, nil
}

// parsePlace reads BLOCK.RUNE, two non-negative integers.
func parsePlace(s string) (nttypeset.Place, error) {
	block, r, ok := strings.Cut(s, ".")
	b, err1 := strconv.Atoi(block)
	n, err2 := strconv.Atoi(r)
	if !ok || err1 != nil || err2 != nil || b < 0 || n < 0 {
		return nttypeset.Place{}, fmt.Errorf("place %q: want BLOCK.RUNE", s)
	}
	return nttypeset.Place{Placed: true, Block: b, Rune: n}, nil
}
