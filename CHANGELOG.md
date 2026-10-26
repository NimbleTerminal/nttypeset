# nttypeset CHANGELOG

## Unreleased

- Nested section backgrounds, padding, solid borders and spacing; resolved block formatting; source IDs with incremental anchor bounds and hit/selection identity lookup.

## v0.1.3 - 2026-10-23

- `RasterOptions.Marks` tints stretches of text behind their glyphs, in `Palette.Mark`, or when that is left zero the selection's color halfway to the page, as for the link under a pointer; `Link.Mark` gives a link line's `Mark`.
- `LightPalette` and `DarkPalette` are written with field names; a palette written by position needs `Mark` before `Syntax`.
- `examples/typeset` tints the link under the pointer, every line of it, and turns the mouse off a moment before quitting, so no mouse report is left at the shell prompt.

## v0.1.2 - 2026-10-22

- `Link` says where it comes from: its block, paragraph and runes, the same at every width, so a caller can tell the lines of one wrapped link from two links to one URL.
- `Layout.TextAt` gives the text under a page point, its paragraph and rune, or nothing off the text, as for finding a web address written out in a paragraph.
- `Layout.Bounds` gives the page rectangle around what each laid block drew, so a caller can find a table or a heading without knowing the spacing between blocks.

## v0.1.1 - 2026-10-19

- `View.Unproject` maps a raster pixel back to the page point it was projected from, the inverse of `Project`.
- `examples/typeset` selects text with a left drag, copies it with `y` and clears it with `Esc` or a click, natively and on the site.
- The site runs on go-booba v0.10.2, whose browser runtime copies OSC 52 writes from wasm programs and carries the ghostty-web fixes for wheel scrolling under mouse tracking, Ctrl+V forwarding and runtime cursor shapes.
- `examples/specimen`, a font specimen from `Preview`, one row per look.
- A browser demo site at nimbleterminal.github.io/nttypeset: both demos compiled to WebAssembly and run through booba and ghostty-web, built and checked by `task ci` and deployed by GitHub Pages. The demos publish their keys as booba actions and gain a `t` glyph/Kitty toggle. `THIRD_PARTY_NOTICES.md` now credits the demos' dependencies and the browser runtime.
- `examples/typeset`, a demo in its own module: a Markdown or text file in the terminal through ntpicture, with keys for scrolling, looks, the code face and the palette, or the top of the page as a PNG with `-o`.
- `THIRD_PARTY_NOTICES.md` credits the modules linked on every supported platform, not only the generating machine's: it now includes `golang.org/x/sys`, which `x/image/vector` uses on amd64.

## v0.1.0 - 2026-10-18

- Extracted from gloss's `--typeset` view: the block and span model with layout limits, font looks and glyph fallback over a `FaceSource`, the shaping engine, incremental layout, and the glyph-mask rasterizer (`raster`).
- Logical selection and caret placement, link and image hit areas, chroma highlighting, `Lines` documents and the `Preview` font chooser picture.
- `nttypeset/goldmark` turns a parsed goldmark tree into a Document.
- Hardened the public boundary: corrupt font bytes are an error, not a panic, in `NewEngine`, `SetFonts` and glyph fallback; `Limits.Em` (4,096 raster pixels per em) and a 1,024-pixel `Block.Size` cap bound glyph tiles; a `Layout` made before a font change returns an error.
- `LimitError.Limit` is an `int64`, so the layout deadline in nanoseconds fits on 32-bit and wasm targets.
- An `Image.Still` holding a nil pointer is a placeholder like a nil interface, not a layout error; a negative `Block.Size` is a `*LimitError`; `ResolveFont` errors carry the `nttypeset:` prefix; `Lines` documents that a cancelled context disables highlighting and still returns the document.
- Renamed the `Kind` constants uniformly: `KindParagraph`, `KindHeading`, `KindQuote`, `KindCode`, `KindLine`, `KindRule`, `KindImage`, `KindTable`.

### Known limitations

- Only the four built-in looks are accepted; a `FaceSource` serves their family names. `WithFontLook`, for custom looks, is planned for a later release.
