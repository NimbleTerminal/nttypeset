# Cell-style contract

`Block.Table == nil` preserves existing content-weighted table layout: 12px
horizontal and 10px vertical cell insets, first-row palette fill and row rules.
Existing Markdown and other callers do not need to change.

A nonnil `TableFormat` opts into explicit cell styles:

- `Cells` must match a nonempty rectangular `Block.Rows`, one `CellFormat` per
  cell. Invalid kind, dimensions or alignment return an error from `Layout`.
- `Padding` and border widths use layout pixels, each 0..65,536. Invalid sizes
  and excessive cell counts return `ErrLimit`. The total styled cell count in a
  document cannot exceed `Limits.Blocks`, including lowered budgets.
- Background fills the cell box behind text. Solid borders are inside that
  box; transparent borders reserve space. Horizontal edges paint over vertical
  edges at corners. Padding and borders reduce the text's available width.
- Empty/left, center and right alignment are physical horizontal directions,
  applied to each wrapped line. Text selection and links move with their runs.
- Rows use their tallest cell; shorter cells stay top-aligned and their paint
  fills the row height. Background and border colors are explicit RGBA values.
- No implicit header fill or row rules are added. Use `Block.Format` to control
  spacing after the table and inherited line height; nil retains its existing
  block spacing. Font size and spans continue to come from `Block`/`Rows`.
- Keep the document and format slices immutable for the lifetime of a layout.
  Engine/layout serialization and cancellation rules are unchanged.

Columns remain content-weighted. This model does not implement CSS column sizing,
cell spans, border collapse, cell spacing, vertical alignment, bidi-aware logical
alignment or percentage dimensions. A host must not claim CSS table conformance
merely because properties can be stored in these fields.

Tests use calculated reference coordinates for fill/border painting and explicit
alignment invariants, not regenerated screenshots. They check legacy rendering,
selection across reflow, links, cancellation, bounded metadata and rollback on
operation-limit failure. Independent browser comparison belongs in the HTML
adapter when that adapter adopts this published API.
