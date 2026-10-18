# nttypeset CHANGELOG

## Unreleased

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
