// Command specimen is the nttypeset font specimen: one row per look from
// nttypeset.Preview, shown in the terminal through ntpicture, or written as a
// PNG with -o.
//
//	go -C examples run ./specimen [flags]
package main

import (
	"context"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"

	"nimbleterminal.dev/ntfonts"
	"nimbleterminal.dev/nttypeset"
)

// body and code are the sample lines of every row: body text with accents and
// an em dash, and a line of code.
const (
	body = "Sphinx of black quartz, judge my vow. Café, naïve, résumé — 0123456789"
	code = "func (e *Engine) Layout(doc *Document, width int) (*Layout, error)"
)

// options are the command line.
type options struct {
	mono, out     string
	dark          bool
	width, height int
}

func main() {
	var o options
	flag.StringVar(&o.mono, "mono", "", `code font: "jetbrains", or empty for each look's own`)
	flag.BoolVar(&o.dark, "dark", false, "paint with the dark palette")
	flag.StringVar(&o.out, "o", "", "write the specimen to this PNG instead of showing it")
	flag.IntVar(&o.width, "width", 800, "PNG width in pixels (with -o)")
	flag.IntVar(&o.height, "height", 600, "PNG height in pixels (with -o)")
	flag.Parse()
	if err := run(o); err != nil {
		fmt.Fprintln(os.Stderr, "specimen:", err)
		os.Exit(1)
	}
}

func run(o options) error {
	ctx := context.Background()
	if o.out == "" {
		return show(ctx, o)
	}
	img, err := render(ctx, o.mono, o.dark, o.width, o.height)
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

// render draws one sample row per built-in look, in the light or dark
// palette, with override as the code face ("" for each look's own).
func render(ctx context.Context, override string, dark bool, width, height int) (*image.RGBA, error) {
	palette := nttypeset.LightPalette()
	if dark {
		palette = nttypeset.DarkPalette()
	}
	return nttypeset.Preview(ctx, nttypeset.FontLooks(), override, body, code, width, height, palette, nttypeset.WithFonts(ntfonts.Embedded))
}

// fit clamps each axis of p to the 160 to 1200 pixels that Preview accepts.
func fit(p image.Point) (width, height int) {
	return min(max(p.X, 160), 1200), min(max(p.Y, 160), 1200)
}
