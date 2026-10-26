// Command hackernews typesets the Hacker News front page and shows it in the
// terminal through ntpicture, or writes it as a PNG with -o.
//
//	go -C examples run ./hackernews [flags]
package main

import (
	"context"
	"flag"
	"fmt"
	"image/png"
	"net/http"
	"os"
	"time"

	"nimbleterminal.dev/ntfonts"
	"nimbleterminal.dev/nttypeset"
)

type options struct {
	out           string
	count         int
	width, height int
	scale         float64
}

func main() {
	var o options
	flag.IntVar(&o.count, "n", 30, "number of top stories")
	flag.StringVar(&o.out, "o", "", "write the front page to this PNG instead of showing it")
	flag.IntVar(&o.width, "width", 800, "page width in layout pixels")
	flag.IntVar(&o.height, "height", 1125, "PNG height in layout pixels (with -o)")
	flag.Float64Var(&o.scale, "scale", 1, "raster pixels per layout pixel (with -o)")
	flag.Parse()
	if err := run(o); err != nil {
		fmt.Fprintln(os.Stderr, "hackernews:", err)
		os.Exit(1)
	}
}

func run(o options) error {
	if o.count < 1 || o.count > 100 {
		return fmt.Errorf("story count %d: want 1 to 100", o.count)
	}
	ctx := context.Background()
	client := &hnClient{
		baseURL: "https://hacker-news.firebaseio.com/v0",
		http:    &http.Client{Timeout: 20 * time.Second},
	}
	eng, err := nttypeset.NewEngine(nttypeset.WithFonts(ntfonts.Embedded), nttypeset.WithLook("technical", ""))
	if err != nil {
		return err
	}
	if o.out == "" {
		return show(ctx, client, eng, nil, o)
	}
	stories, err := client.stories(ctx, o.count)
	if err != nil {
		return err
	}
	doc := buildDocument(stories, time.Now())
	lay, err := eng.Layout(doc, o.width, nttypeset.LayoutOptions{})
	if err != nil {
		return err
	}
	img, err := lay.Raster(ctx, nttypeset.View{Width: o.width, Height: o.height, Scale: o.scale}, nttypeset.RasterOptions{Palette: hnPalette()})
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
