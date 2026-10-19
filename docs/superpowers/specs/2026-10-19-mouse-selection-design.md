# Mouse selection in the typeset demo

The typeset demo takes mouse selection the way gloss's `--typeset` view has
it: a left drag over the page highlights the text between two page points,
`y` copies it through the terminal's clipboard, and `Esc` or a plain click
clears it. The selection is logical, so it keeps through scrolling, a change
of look or palette, and the glyph/Kitty toggle. The same code runs natively
and on the GitHub Pages site.

## Why

nttypeset already has logical selection (`Select`, `Selected`,
`RasterOptions.Selection`), and gloss already drives it from the mouse. The
demo only shows a selection given on the command line, so nothing a visitor
can touch proves that the library's selection works. Porting gloss's
behaviour into the demo closes that gap and leaves a worked example for any
viewer built on nttypeset. Gloss's own code is the reference: it works well,
and its decisions are kept, not redesigned.

## Goals

- In the terminal and in the browser, a left drag over the page selects text,
  which is highlighted in the palette's selection colour as the drag moves.
- `y` copies the selected text through OSC 52 and the status line says how
  many characters were copied. On the site, the clipboard write reaches the
  browser's clipboard.
- `Esc` clears the selection. A plain left click clears it. A click with no
  drag, or a drag that closes on no text, leaves no selection.
- The selection survives `j`/`k`/`G`/`g`, `d`, `f`, `m`, `n`, `c` and `t`,
  because both ends are placed in the text once the first result is in.
- Shift and Alt drags are left to the terminal, so the terminal's own
  selection still works over the page.
- `View.Unproject` in the library maps a raster pixel back to a page point,
  the inverse of `Project`, so the demo and gloss stop computing it by hand.
- Unit tests pin the behaviour through Bubble Tea messages, and the wasm
  smoke test drives one drag and one copy through the booba bridge.

## Non-goals

- Link clicks and link hover. Gloss opens links through a host opener the
  demo does not have. Links stay reported by `Links` and nothing more.
- Double-click word selection, triple-click paragraph selection, Shift-click
  extension and keyboard selection. Gloss has none of these; the demo
  matches gloss.
- Selection in the specimen demo, which has no text to select.
- Any other change to the library's public API, limits or invariants.

## Anti-goals

- The library importing Bubble Tea or ntpicture. The mouse plumbing lives in
  the examples module; the library gains only pure geometry.
- A drag that blocks on layout. Gloss follows a drag rather than waits for
  it; the demo lays out synchronously in `Update` but must not call
  `Complete` on a drag.
- Browser-only or native-only behaviour. One viewer, both targets.

## Constraints

- All of `AGENTS.md` holds. The examples module may not add sibling
  `replace` directives; `wasm.work` stays as it is.
- Bubble Tea v2.1.0 natively and the pinned fork in `wasm.work` for the
  browser. Mouse messages are `tea.MouseClickMsg`, `tea.MouseMotionMsg` and
  `tea.MouseReleaseMsg`; the view asks for `tea.MouseModeCellMotion`;
  copying is `tea.SetClipboard`.
- ntpicture v0.1.0. The demo rasters at `RasterGeometry().Pixels` exactly
  and shows the picture at the top left of the screen, so a cell maps to a
  raster pixel without letterboxing: gloss's `typesetPixel` fitting math is
  not needed and not ported.
- The booba action-bar status stays under its 2048-byte cap
  (`demorun.Check`), with the two new bindings included.
- go-booba's terminal denies OSC 52 unless `allowOSC52: true` is passed, so
  `web/site.js` must pass it.

## The library change

`View` gains one method:

```go
// Unproject maps a raster pixel to the page point it was projected from:
// each coordinate divided by Scale and rounded, then offset by the view's
// top left. It inverts Project for the top left of a projected rectangle at
// a Scale of 1 or more; below 1 a raster pixel covers several page pixels
// and the nearest is returned. A Scale that is not positive and finite
// returns the view's top left.
func (v View) Unproject(p image.Point) image.Point
```

With `Pixels` set, `Scale` is still what the caller rastered at, so the same
formula holds. `TestUnprojectInvertsProject` projects a rectangle and
unprojects its `Min` back exactly at scales 1 and 1.5; at scale 0.125 it
checks `Unproject` of the raster's last pixel (`Size() - (1, 1)`) lies in the
half-open region `[Left, Left+Width) × [Top, Top+Height)`; and it checks
scale 0 returns `(Left, Top)`.

## The viewer

`examples/typeset/viewer.go` gains the selection state and the mouse
handling; the behaviour is gloss's `typeset_select.go` with the asynchronous
request plumbing removed, since the demo rasters in `Update`.

State on `viewer`:

- `sel *nttypeset.Selection`: the selection as asked, nil for none. After a
  raster, both ends are the placed ends the result gave back (gloss adopts
  them so the anchor is stable across reflow).
- `selected string`: the text of the last drawn selection, empty for none.
- `dragging bool` and `dragFrom image.Point`: a drag in progress and the
  page point it began at.
- `view nttypeset.View`: the view of the last raster, for mapping a cell to
  the page.
- `note string`: a one-shot status note (`copied N characters`), cleared by
  the next key press other than `y` and by the next mouse click; motion and
  release leave it.

Mouse events are the page's only while the picture shows the last raster:
when `m.lay` is nil, `m.err` is set, or `m.pic.RasterGeometry()` differs from
`m.geom` (between a resize or toggle and its redraw), every mouse message
falls through to ntpicture and a drag in progress ends. This is gloss's
`typesetViewMatches` guard.

Cell to page: for a mouse message at cell (x, y) with the last raster
geometry `g`, the raster pixel is the cell's centre,
`(( 2x+1) * g.Pixels.X / (2 * g.Cells.X), (2y+1) * g.Pixels.Y / (2 *
g.Cells.Y))`; a cell outside `g.Cells` is outside the picture. During a drag
the pixel is clamped to the raster instead, so a drag that leaves the picture
selects to the edge, as in gloss. The page point is `m.view.Unproject(pixel)`.

Mouse, following gloss exactly:

- Click: dragging stops. A button other than left, or a Shift or Alt
  modifier, is not the page's. Otherwise any selection, including one from
  `-select`, is cleared and the page redrawn, whether or not the cell is in
  the picture; if it is, a drag starts there, on text or on blank space
  alike.
- Motion while dragging: the point is clamped to the raster's last pixel.
  If it is the same page point as the last motion, nothing happens. Otherwise
  the selection becomes `{Anchor: placed anchor if there is one, else the
  drag start point; Focus: the point}`, and the page is redrawn with it. The
  anchor stays a bare point until a result places both ends, which `Select`
  does together or not at all.
- Release while dragging: dragging stops; if the drawn selection has no text
  the selection is cleared without a redraw, since nothing was drawn.
- Any mouse message that is not the page's falls through to ntpicture's
  `Update` as before.

A redraw per motion is accepted: glyph rasters are small, and Kitty rasters
are what a scroll key costs. The same-point check is the only coalescing.

Keys:

- `y`, labelled `Copy`: with text selected, sets the note to
  `copied N characters` (N in runes) and returns `tea.SetClipboard(text)`.
  With nothing selected it does nothing.
- `esc`, labelled `Clear selection`: clears the selection and redraws; with
  no selection it is not handled.
- Both go in `More`, after `chop` and before `keep`, so the help row still
  ends with Quit. Both are always enabled, like gloss's `y`, so the help row
  and the booba snapshot list them from the start, and the existing
  enabled-label tests add them.

Raster: `page.render` takes the selection as a parameter instead of reading
`page.sel`; the PNG path passes the `-select` flag's selection, the viewer
passes its own. The viewer starts with the flag's selection as `sel`, so
`-select` still shows in the terminal. After `Select`, `sel` becomes the
result's `Places()` and `selected` its `Text()`; a nil result (no text at the
point) leaves `sel` as asked while dragging and clears it when not, matching
gloss's "a drag that has closed on nothing is no selection".

Status: when text is selected the status line ends with
`· N selected · y copies`, N in runes like the note; a note replaces that
suffix while it is set. The status line is cut to the terminal width with
`ansi.Truncate(status, width, "…")`, so it never wraps and never pushes the
help row off the screen.

Mouse mode: `View` sets `MouseMode: tea.MouseModeCellMotion`.

## The site

- `web/site.js` passes `allowOSC52: true` to `BoobaTerminal` so `y` reaches
  the browser clipboard.
- The typeset help line adds `drag selects · y copies · Esc clears`.
- `scripts/smoke-demo.mjs`, for the typeset demo only: after the scroll
  check, writes an SGR press at a cell in the page, an SGR motion while held
  to a cell further down, and a release; waits for output; then writes `y`
  and asserts the output contains `\x1b]52;c;` with a non-empty base64
  payload. The press/motion/release sequences are
  `\x1b[<0;COL;ROWM`, `\x1b[<32;COL;ROWM` and `\x1b[<0;COL;ROWm`, 1-based.
  It also asserts the snapshot taken at startup advertises `Copy` enabled.
  The pinned browser fork reads the bridge through ultraviolet's parser,
  which decodes SGR mouse sequences whatever the terminal's mouse mode, so
  the raw sequences reach `Update` without a mode handshake.

## Documentation

- `examples/typeset/sample.md`: one sentence that a drag selects and `y`
  copies.
- `README.md`: the typeset demo paragraph mentions mouse selection; the
  Selection bullet mentions `Unproject`.
- `DEVELOP.md`: the manual acceptance check adds a drag and a copy, natively
  and in the browser.
- `AGENTS.md`: `view.go` row mentions `Unproject`; the examples row mentions
  selection.
- `CHANGELOG.md` Unreleased: `View.Unproject`; mouse selection and `y` copy
  in the typeset demo.

## Tests

- `view_test.go`: `TestUnprojectInvertsProject`.
- `examples/typeset/viewer_test.go`, driving a viewer over the embedded
  sample with a fixed geometry (the viewer's `pic` is real; geometry comes
  from a `WindowSizeMsg` after `pic.Init` as the existing tests do, or from
  a stub if the picture reports no pixels under test; the builder decides
  and records which):
  - `TestViewerDragSelectsAndCopies`: click, motion, release selects text
    containing a word from the sample, the raster differs from the plain one,
    both ends are placed, `y` returns a command whose message carries the
    same text and the status says copied, `j` keeps the text, `Esc` clears.
  - `TestViewerClickAndShiftDragSelectNothing`: a click without motion, a
    Shift-drag, and a drag that starts outside the picture all leave `sel`
    nil.
  - `TestViewerClickClearsSelection`: after a drag, a plain click elsewhere
    clears.
  - `TestViewerKeysFitBooba` and `TestViewerKeyDispatch` extend to the new
    bindings.
- Smoke as above. `task ci` green locally and in Actions.

## Branch and delivery

Branch `feat/mouse-selection` in a sibling worktree, as the site was done.
Pushing main is Evan's word, as before.

## Left to the builder

- How the viewer test obtains a raster geometry without a terminal.
- The exact cells the smoke test presses, chosen from the 100×40 resize so
  they land on the sample's first paragraph.
