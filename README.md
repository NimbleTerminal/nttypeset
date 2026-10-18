# nttypeset — Typesetting for Nimble Terminal

<p>
    <a href="https://pkg.go.dev/nimbleterminal.dev/nttypeset"><img src="https://pkg.go.dev/badge/nimbleterminal.dev/nttypeset.svg" alt="Go Reference"></a>
    <a href="https://github.com/NimbleTerminal/nttypeset/actions/workflows/ci.yml"><img src="https://github.com/NimbleTerminal/nttypeset/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
    <a href="./CODE_OF_CONDUCT.md"><img src="https://img.shields.io/badge/Contributor%20Covenant-2.1-4baaaa.svg" alt="Code Of Conduct"></a>
</p>

nttypeset is a Go library that lays out a `Document` of blocks and styled spans
and rasters viewports of it into `*image.RGBA`. It lays out incrementally, shapes
text with an engine that keeps bounded caches, draws through a glyph-mask
rasterizer, and reports logical selections along with link and image hit areas.
It was extracted from the `--typeset` view of
[Gloss](https://github.com/NimbleMarkets/gloss).

It draws images and nothing else. It writes nothing to a terminal; show its
output with [ntpicture](https://nimbleterminal.dev/ntpicture). The core does not
parse Markdown: the `nttypeset/goldmark` subpackage takes an already parsed
goldmark tree. It ships no fonts either: faces come from a `FaceSource`, and the
separate module `nimbleterminal.dev/ntfonts` supplies seven embedded OFL families
as `ntfonts.Embedded`.

```sh
go get nimbleterminal.dev/nttypeset nimbleterminal.dev/ntfonts
```

## Quick start

```go
ctx := context.Background()

// ntfonts.Embedded is the separate ntfonts module's embedded OFL subsets.
eng, _ := nttypeset.NewEngine(nttypeset.WithFonts(ntfonts.Embedded))

// A Lines document: highlighted source, one block per line.
doc, _ := nttypeset.Lines(ctx, src, "go", styles.Get("github"))

// Lay it out at 400 layout pixels wide. Nothing is laid out yet.
lay, _ := eng.Layout(doc, 400, nttypeset.LayoutOptions{})

// Raster lays out as far as the view needs, then draws it.
img, _ := lay.Raster(ctx, nttypeset.View{Width: 400, Height: 100, Scale: 1},
    nttypeset.RasterOptions{Palette: nttypeset.LightPalette()})
```

The engine offers four built-in looks (`FontLooks`: readable, editorial,
technical, compact) and asks its `FaceSource` for their families by name:
Literata, Newsreader, IBM Plex Sans, Source Serif 4, Iosevka, IBM Plex Mono and
JetBrains Mono. Any FaceSource that serves these family names works; to use
other fonts, serve them under these names. Custom looks are not supported yet.

`img` is an `*image.RGBA` of 400x100 pixels. The runnable form, with test fonts
standing in for `ntfonts.Embedded`, is `ExampleLines` in `example_test.go`.
Handle the errors; they are dropped here for brevity.

## Markdown

The `nimbleterminal.dev/nttypeset/goldmark` subpackage walks a tree you parsed
yourself, so you keep goldmark's parser, extensions and any normalization. Only
this subpackage imports goldmark.

```go
md := goldmark.New(goldmark.WithExtensions(extension.GFM, emoji.Emoji))
root := md.Parser().Parse(text.NewReader(src))

// Images are looked up by destination; the adapter reads no files.
doc, _ := ntgoldmark.Build(ctx, root, src, ntgoldmark.Options{
    Images: map[string]nttypeset.Image{"logo.png": {Key: "logo.png", Still: logo}},
    Syntax: styles.Get("github"),
})
lay, _ := eng.Layout(doc, 800, nttypeset.LayoutOptions{})
img, _ := lay.Raster(ctx, nttypeset.View{Width: 800, Height: 600, Scale: 1},
    nttypeset.RasterOptions{Palette: nttypeset.LightPalette()})
```

`ntgoldmark` is the import `nimbleterminal.dev/nttypeset/goldmark`. A missing
image lays out as the text `[image unavailable: dest]`; `Options.Bounds` sets an
image's size and alignment from its node.

## Demo

`examples/typeset` shows a Markdown or text file in the terminal through
ntpicture, with keys to scroll, cycle the looks, swap the code face and switch
palettes, or writes the top of the page as a PNG:

```sh
task run-typeset                                        # the embedded sample page
task run-typeset -- -dark -font technical README.md
task run-typeset -- -numbers -select 3.0-5.10 -o page.png layout.go
```

## What you get

- Incremental layout: `Layout.Ensure`, `EnsureBlock` and `Complete` lay out only
  what a view needs, and `Raster` does so itself.
- Hit areas: `Links` and `Images` for a view, and `Find` for a link target.
- Selection: `Select` turns two points on the page into logical text and a
  highlight that `Raster` draws.
- `Highlight` and `Lines`: chroma tokens as styled spans, and source as a
  document, with an optional gutter of line numbers or unwrapped lines through
  `LayoutOptions`.
- `Preview`: a font chooser picture of sample rows, one per `FontLook`.
- `nttypeset/goldmark`: a parsed goldmark tree as a `Document`.

Layout is bounded. A document may hold 32,768 blocks, a paragraph 16,384 runes,
a layout 262,144 glyphs and 32,768 display-list entries, and a raster 32 Mi
pixels with text at no more than 4,096 pixels per em; a block's size is at most
1,024 layout pixels and never negative, and layout stops after 15 s. Limits can be lowered with `WithLimits`, never
raised, and exceeding one is a `*LimitError` that matches `ErrLimit`. The shaped
run cache and the glyph-mask cache are each capped at 32 MiB, and no panic
crosses the public API, including on corrupt font bytes.

## License

MIT; see [LICENSE](./LICENSE) and [NOTICE.md](./NOTICE.md).
