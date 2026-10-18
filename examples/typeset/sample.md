# Typesetting for the terminal

nttypeset lays out a document of blocks and styled spans and rasters any view
of it into an image. This page is laid out with the embedded ntfonts faces and
shown through ntpicture. Press `f` to cycle the four looks, `d` for the dark
palette, `m` to swap the code face for JetBrains Mono, and `j`/`k` to scroll.

Words such as café, naïve and résumé keep their accents; **bold**, *italic*,
~~struck~~ and `inline code` keep their styles across line wraps. A
[link](https://nimbleterminal.dev/nttypeset) is recorded as a hit area that
`Links` reports for the view.

> A quotation has room to breathe, and a second line keeps the same bar
> beside it when the paragraph wraps.

## Code

Fenced code is highlighted by chroma at layout time; the palette colors the
tokens when the page is painted, so the same layout repaints in dark.

```go
// Layout lays out as far as the view needs, then Raster draws it.
func page(ctx context.Context, eng *nttypeset.Engine, doc *nttypeset.Document) (*image.RGBA, error) {
	lay, err := eng.Layout(doc, 800, nttypeset.LayoutOptions{})
	if err != nil {
		return nil, err
	}
	return lay.Raster(ctx, nttypeset.View{Width: 800, Height: 600, Scale: 1},
		nttypeset.RasterOptions{Palette: nttypeset.LightPalette()})
}
```

## Tables and lists

| Look      | Body            | Code           |
| --------- | --------------- | -------------- |
| readable  | Literata        | Iosevka        |
| editorial | Newsreader      | IBM Plex Mono  |
| technical | IBM Plex Sans   | IBM Plex Mono  |
| compact   | Source Serif 4  | Iosevka        |

1. Blocks are the input, not an AST; the goldmark adapter built this page.
2. Colors are roles on spans, resolved from a palette at paint time.
3. Images are reserved rectangles, decoded by the caller.

- [x] Lay out incrementally
- [ ] Reach the end of the page

## Pictures

A local image is decoded by the demo and handed to the adapter by destination:

![A landscape](landscape.png)

One that cannot be found lays out as a placeholder:

![A missing picture](missing.png)

---

The rule above is a block of its own. Scroll with `G` to reach the end of the
page, and `g` to return to the top.
