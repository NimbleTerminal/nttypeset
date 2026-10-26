# Sections and source identity

`Document.Sections []Section` decorates nonempty half-open block ranges
`[Start, End)`. This keeps the existing flat reading order and logical
`Place{Block, Para, Rune}` selection coordinates. A section may contain other
sections; crossing ranges are rejected. Input order breaks ties for equal
ranges, with the first outermost. Sections are sorted internally, without
changing the document. Do not mutate a document during layout.

`Section` supplies `Padding Insets`, `Background color.RGBA`, four independent
`Border{Width, Color}` edges (`Top`, `Right`, `Bottom`, `Left`), and
`SpaceBefore`/`SpaceAfter`. All dimensions are integer layout pixels. Spacing
is outside the decoration; padding is inside the border. Spacing is additive,
without CSS margin collapse. Transparent borders reserve width. Colors use
Go's premultiplied RGBA convention; transparent backgrounds paint nothing.

Layout opens sections before their first block and closes them after their
last block. Each laid block contributes one segment of every enclosing
section, painted outermost first behind its content. Only the actual first
and last segment paint top and bottom borders. Side borders and backgrounds
continue through the spaces between blocks. Raster clips these rectangles to
the viewport, including horizontal pan and zoom: it does not invent a border
at a viewport edge. A section extending below the viewport remains open;
`Raster` only lays blocks through the viewport bottom. A single block is still
the atomic layout unit, as before. Content that exceeds the available width
can overflow; section borders do not impose a new content clipping region.

Zero decoration adds no space and emits no paint operations. Existing callers
with nil new fields retain their output. Use keyed struct literals when adding
fields to public models. There are at most `Limits.Blocks` sections and 64
nesting levels; each spacing, padding, border width and line height is in
0..65,536. Painted rectangles count against `Limits.Operations`. Existing
height, time, glyph, raster, font and cache limits still apply. Cancellation
and limits roll back the entire block, including its section state.

## Resolved typography and spacing

Set `Block.Size` to the resolved font size (1..1,024). The built-in face flags
remain `Span.Style`; set resolved colors in `Span.Ink` with `RoleText` and
`Token == 0` to avoid palette overrides. `Span.Href` preserves link areas.
Font sizes are block-level; split blocks for different sizes. Font families
remain the built-in looks, with the monospace flag selecting the code face.

A nonnil `Block.Format *BlockFormat` replaces automatic heading-before space,
paragraph-after space, and the code block's default inset/background. It
supplies `LineHeight`, `SpaceBefore`, `SpaceAfter`, `Padding` and `Background`.
Zero line height keeps the original leading; positive line height is enlarged
to fit font metrics. A heading's `Block.Size` is used directly. Tables now
honor nonzero `Block.Size` as well; zero preserves 25. Missing image
placeholders honor nonzero size; zero preserves 20. The library never applies
CSS heading-size rules. A nil format preserves all existing defaults.

This is a block-flow subset, not a CSS engine. The bridge resolves inheritance,
units and cascade before building this model. Inline font-size changes,
custom families, rounded/dashed borders, shadows, width/height CSS constraints,
flex/grid, floats and margin collapse are not modeled. Tables keep their
existing cell padding, palette header fill, horizontal rules and width
heuristic; per-cell CSS decoration is an approximation. Rules keep their
2-pixel thickness. Page margins remain `Margin` (50). The host chooses the
page palette and can crop the page margin in its `View`. There is no browser
pixel-parity claim.

## Bridge recipe

1. Flatten visible content into blocks in reading order. Record the block
   count on entering each visible container, then emit a `Section` with that
   start and the final count on leaving. Omit containers with no blocks.
2. Map resolved container background, padding, solid edge colors/widths and
   vertical margins to a section. Map resolved leaf typography, color and
   spacing to `Block.Size`, spans and `Block.Format`. A preformatted block can
   use `KindCode` with explicit format/padding/background and `Mono` spans.
3. Store opaque element IDs in `Section.SourceID` or `Block.SourceID`. IDs are
   exact strings, without fragment parsing or URL decoding. All nonempty IDs
   must be unique across blocks and sections; duplicates fail `Engine.Layout`.
   A wrapper and its leaf must not both repeat the same ID. Inline IDs need
   separate blocks; inline identity ranges are not modeled.
4. `AnchorBounds(ctx, id, false)` returns `(Anchor, found, error)` without
   laying more content. An unlaid ID is found with an empty rectangle and
   `Complete == false`. An open section reports its currently laid bounds.
   `AnchorBounds(ctx, id, true)` lays through that block or section's end;
   request this explicitly for navigation needing full geometry. Missing or
   empty IDs return `found == false` and never force layout. Hidden content
   and its IDs are omitted by the host. There is no hidden registry.
5. `SourceAt(ctx, place)` returns the block's ID, otherwise its innermost
   identified section. Pass either `Selected.Places()` end, a point place for
   nearest-text behavior, or a placed block/paragraph/rune constructed from
   `TextAt`'s `Hit`. A placed block can resolve image or empty-block identity
   too. Point resolution searches already laid text, just as selection does.
6. Reflow with `Engine.Layout` at the new width, keeping the same source IDs.
   Logical selection coordinates survive width changes. Rebuild the model
   and resolve IDs again after host filtering or collapse changes block indices.

`Anchor.Rect` is a page bounding box, not a list of line rectangles. Block
bounds surround everything that block drew, including its section segment;
section bounds include decoration and internal spaces, excluding the section's
own outside spacing. Overlapping/equal-range wrappers can have identical
geometry. Anchor lookups and source resolution honor stale-font errors and
context cancellation just like layout and selection.

## Example and visual check

`ExampleSection` is executable without filesystem or network access, natively
and under js/wasm. Run `go run ./internal/tools/sections` to reproduce these
illustrations. Their generic content and palette are adapted from
`go-thinkt/internal/conversation/testdata/conversation.html` (read 2026-10-26);
no fixture or code in that repository is modified. Both images use identical
resolved text styling; the second adds nested section ranges. These images
are demonstration artifacts, not test goldens.

[Before](sections-before.png) · [After](sections-after.png)

Regression tests independently check border/background pixels, progressive
layout, cropped viewport consistency, nested spacing, reflow, source lookup,
selection, cancellation, resource-limit rollback and zero-decoration parity.
Automated pixel checks establish those invariants, not visual quality.
