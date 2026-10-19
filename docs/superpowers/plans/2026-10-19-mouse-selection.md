# Mouse Selection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The typeset demo selects text with a left drag, copies it with `y` and clears it with `Esc` or a click, natively and on the Pages site, the way gloss's typeset view does.

**Architecture:** One pure method on the library's `View` inverts `Project`. The examples viewer gains selection state and mouse handling ported from gloss's `typeset_select.go` with the async request queue removed, since the demo rasters inside `Update`. The site passes OSC 52 through and the wasm smoke test drives one drag and one copy.

**Tech Stack:** Go 1.27.2 (module minimum 1.26), Bubble Tea v2.1.0 natively and the pinned fork under `wasm.work`, ntpicture v0.1.0, go-booba v0.9.2, Node 24 for the site checks.

**Spec:** `docs/superpowers/specs/2026-10-19-mouse-selection-design.md`

## Global Constraints

- All of `AGENTS.md` holds: the core imports only go-text, chroma and x/image; no sibling `replace` directives; no panic crosses the public boundary; `task ci` before finishing.
- Mouse messages are `tea.MouseClickMsg`, `tea.MouseMotionMsg`, `tea.MouseReleaseMsg`; the viewer's `View` sets `MouseMode: tea.MouseModeCellMotion`; copying is `tea.SetClipboard`.
- A cell (x, y) under raster geometry `g` is the raster pixel `((2x+1)*g.Pixels.X/(2*g.Cells.X), (2y+1)*g.Pixels.Y/(2*g.Cells.Y))`; a cell outside `g.Cells` is outside the picture; a drag clamps to `g.Pixels - (1, 1)`.
- Mouse messages are the page's only while `m.lay != nil && m.err == nil && m.pic.RasterGeometry() == m.geom`.
- Shift and Alt drags, and buttons other than left, are never the page's.
- The booba status stays under 2048 bytes (`demorun.Check`), with `Copy` and `Clear selection` always enabled.
- The status line is cut to the terminal width with `ansi.Truncate(status, width, "…")`.
- Counts in the status and the note are runes (`utf8.RuneCountInString`).
- Links, double-click, Shift-click and the specimen demo are out of scope.
- `wasm.work` and `examples/go.mod` dependencies do not change.

## Review Focus

- A drag that leaves the picture: the focus clamps to the raster's edge and the selection keeps growing; it does not end or jump. Test: `TestViewerDragClampsAtEdge` in Task 2.
- Mouse events between a resize or `t` toggle and the redraw: ignored, and a drag in progress ends, instead of mapping through a stale view. Test: `TestViewerMouseIgnoredWhileStale` in Task 2.
- A change of look or palette during a selection: the placed ends survive the new layout and the text is the same. Test: `TestViewerSelectionSurvivesLook` in Task 2.
- A narrow terminal with a long status: the status is one row and the help row stays visible. Test: `TestViewerStatusFitsWidth` in Task 2.
- `Esc` with no selection: nothing happens and the program does not quit. Test: part of `TestViewerKeyDispatch` in Task 2.

---

### Task 1: `View.Unproject`

**Files:**
- Modify: `view.go`
- Create: `view_test.go`
- Modify: `CHANGELOG.md` (Unreleased)

**Interfaces:**
- Produces: `func (v View) Unproject(p image.Point) image.Point`.

- [ ] **Step 1: Write the failing test**

```go
package nttypeset

import (
	"image"
	"testing"
)

func TestUnprojectInvertsProject(t *testing.T) {
	r := image.Rect(130, 2040, 530, 2100)
	for _, scale := range []float64{1, 1.5} {
		v := View{Top: 2000, Left: 100, Width: 800, Height: 600, Scale: scale}
		if got := v.Unproject(v.Project(r).Min); got != r.Min {
			t.Errorf("scale %v: unproject %v, want %v", scale, got, r.Min)
		}
	}
	v := View{Top: 2000, Left: 100, Width: 800, Height: 600, Scale: 0.125}
	last := v.Size().Sub(image.Pt(1, 1))
	got := v.Unproject(last)
	if got.X < v.Left || got.X >= v.Left+v.Width || got.Y < v.Top || got.Y >= v.Top+v.Height {
		t.Errorf("last pixel %v unprojects to %v, outside the region", last, got)
	}
	if got := (View{Top: 7, Left: 3, Width: 10, Height: 10}).Unproject(image.Pt(5, 5)); got != image.Pt(3, 7) {
		t.Errorf("scale 0: %v", got)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `GOWORK=off go test -mod=readonly -run TestUnprojectInvertsProject .`
Expected: FAIL, `v.Unproject undefined`.

- [ ] **Step 3: Implement `Unproject` in `view.go`**

Doc comment from the spec. Each coordinate `int(math.Round(float64(c)/v.Scale))` plus `Left` or `Top`; when `!(v.Scale > 0) || math.IsInf(v.Scale, 0)` return `image.Pt(v.Left, v.Top)`.

- [ ] **Step 4: Run the test to verify it passes**

Run: `GOWORK=off go test -mod=readonly -run TestUnprojectInvertsProject .`
Expected: PASS.

- [ ] **Step 5: CHANGELOG**

Under `## Unreleased`, first bullet: `` - `View.Unproject` maps a raster pixel back to the page point it was projected from, the inverse of `Project`. ``

- [ ] **Step 6: Commit**

```bash
git add view.go view_test.go CHANGELOG.md
git commit -m "Add View.Unproject"
```

---

### Task 2: Selection in the viewer

**Files:**
- Modify: `examples/typeset/viewer.go`
- Modify: `examples/typeset/main.go` (`page.render`, the `-o` call)
- Modify: `examples/typeset/main_test.go` (`TestRenderPaintsAndSelectionShows` passes the selection)
- Modify: `examples/typeset/viewer_test.go`

**Interfaces:**
- Consumes: `View.Unproject` from Task 1; `demorun.Binding(label, shortcuts...)`; `nt.Picture.RasterGeometry()` with `Cells` and `Pixels`.
- Produces: `func (p *page) render(ctx context.Context, lay *nttypeset.Layout, v nttypeset.View, dark bool, sel *nttypeset.Selection) (*image.RGBA, error)`; on `viewer` the fields `sel *nttypeset.Selection`, `selected string`, `dragging bool`, `dragFrom image.Point`, `view nttypeset.View`, `note string`; bindings `copy` (`"Copy"`, `"y"`) and `clear` (`"Clear selection"`, `"esc"`) in `More` after `chop`, before `keep`.

- [ ] **Step 1: Change `render` to take the selection**

`page.render` drops its read of `p.sel` and takes `sel *nttypeset.Selection`; `run` passes `p.sel` for the PNG; `TestRenderPaintsAndSelectionShows` passes `nil` then `&sel` instead of setting `p.sel`. Run `go -C examples test -mod=readonly ./typeset` and commit `Pass the selection to render`.

- [ ] **Step 2: Write the failing viewer tests**

A helper builds a viewer with a real engine and the sample page at a glyph geometry, then draws once:

```go
func newTestViewer(t *testing.T) *viewer {
	t.Helper()
	ctx := context.Background()
	eng, err := nttypeset.NewEngine(nttypeset.WithFonts(ntfonts.Embedded), nttypeset.WithLook("readable", ""))
	if err != nil {
		t.Fatal(err)
	}
	p, err := open(ctx, "", options{})
	if err != nil {
		t.Fatal(err)
	}
	pic := nt.New(nt.Options{AutoMode: true})
	t.Cleanup(func() { _ = pic.Close() })
	pic.SetSize(100, 38)
	m := &viewer{ctx: ctx, page: p, eng: eng, pic: pic, o: options{font: "readable", width: 800}, width: 100}
	m.redraw()
	if m.err != nil || m.last == nil {
		t.Fatalf("first draw: %v", m.err)
	}
	return m
}

func mouse(m *viewer, msg tea.Msg) tea.Cmd {
	_, cmd := m.Update(msg)
	return cmd
}

func click(x, y int) tea.MouseClickMsg     { return tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft} }
func motion(x, y int) tea.MouseMotionMsg   { return tea.MouseMotionMsg{X: x, Y: y, Button: tea.MouseLeft} }
func release(x, y int) tea.MouseReleaseMsg { return tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft} }
```

The tests:

```go
func TestViewerDragSelectsAndCopies(t *testing.T) {
	m := newTestViewer(t)
	plain := m.last // the *image.RGBA of the last raster; add the field
	mouse(m, click(4, 1))
	mouse(m, motion(40, 12))
	if !strings.Contains(m.selected, "nttypeset") {
		t.Fatalf("drag selected %q", m.selected)
	}
	if m.sel == nil || !m.sel.Anchor.Placed || !m.sel.Focus.Placed {
		t.Fatalf("selection not placed: %+v", m.sel)
	}
	if bytes.Equal(m.last.Pix, plain.Pix) {
		t.Fatal("highlight not drawn")
	}
	grown := m.selected
	mouse(m, motion(70, 20))
	if len(m.selected) <= len(grown) {
		t.Fatalf("selection did not grow: %q then %q", grown, m.selected)
	}
	mouse(m, release(70, 20))
	if !strings.Contains(m.View().Content, "selected · y copies") {
		t.Fatalf("status lacks the selection:\n%s", m.View().Content)
	}
	before := m.selected
	press(m, 'j')
	if m.selected != before {
		t.Fatalf("scroll changed the selection %q", m.selected)
	}
	cmd := press(m, 'y')
	if cmd == nil || fmt.Sprint(cmd()) != before {
		t.Fatal("y did not copy the selection")
	}
	if !strings.Contains(m.View().Content, fmt.Sprintf("copied %d characters", utf8.RuneCountInString(before))) {
		t.Fatalf("note missing:\n%s", m.View().Content)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.sel != nil || m.selected != "" {
		t.Fatal("esc kept the selection")
	}
	if press(m, 'y') != nil {
		t.Fatal("y copied nothing")
	}
}

func TestViewerClickAndShiftDragSelectNothing(t *testing.T) {
	m := newTestViewer(t)
	mouse(m, click(4, 1))
	mouse(m, release(4, 1))
	if m.sel != nil {
		t.Fatal("click selected")
	}
	mouse(m, tea.MouseClickMsg{X: 4, Y: 1, Button: tea.MouseLeft, Mod: tea.ModShift})
	mouse(m, tea.MouseMotionMsg{X: 40, Y: 12, Button: tea.MouseLeft, Mod: tea.ModShift})
	if m.sel != nil {
		t.Fatal("shift-drag selected")
	}
	mouse(m, click(4, 38)) // the status row, just below the 38-row picture
	mouse(m, motion(40, 12))
	if m.sel != nil || m.dragging {
		t.Fatal("drag from outside the picture selected")
	}
}

func TestViewerClickClearsSelection(t *testing.T) {
	m := newTestViewer(t)
	mouse(m, click(4, 1))
	mouse(m, motion(40, 12))
	mouse(m, release(40, 12))
	if m.selected == "" {
		t.Fatal("no selection to clear")
	}
	mouse(m, click(60, 20))
	mouse(m, release(60, 20))
	if m.sel != nil || m.selected != "" {
		t.Fatal("click did not clear the selection")
	}
}

func TestViewerDragClampsAtEdge(t *testing.T) {
	m := newTestViewer(t)
	mouse(m, click(4, 1))
	mouse(m, motion(40, 12))
	inside := m.selected
	mouse(m, motion(150, 60)) // past the picture on both axes
	if !m.dragging || len(m.selected) <= len(inside) {
		t.Fatalf("drag past the edge: dragging %v, %q then %q", m.dragging, inside, m.selected)
	}
}

func TestViewerMouseIgnoredWhileStale(t *testing.T) {
	m := newTestViewer(t)
	mouse(m, click(4, 1))
	m.pic.SetSize(80, 30) // geometry changes before any redraw
	mouse(m, motion(40, 12))
	if m.dragging || m.sel != nil {
		t.Fatal("stale geometry still mapped the mouse")
	}
}

func TestViewerSelectionSurvivesLook(t *testing.T) {
	m := newTestViewer(t)
	mouse(m, click(4, 1))
	mouse(m, motion(40, 12))
	mouse(m, release(40, 12))
	before := m.selected
	press(m, 'f')
	press(m, 'd')
	if m.selected != before {
		t.Fatalf("look or palette changed the selection: %q then %q", before, m.selected)
	}
}

func TestViewerStatusFitsWidth(t *testing.T) {
	m := newTestViewer(t)
	mouse(m, click(4, 1))
	mouse(m, motion(40, 12))
	mouse(m, release(40, 12))
	m.width = 40
	rows := strings.Split(m.View().Content, "\n")
	status := rows[len(rows)-2]
	if ansi.StringWidth(status) > 40 || !strings.HasSuffix(status, "…") {
		t.Fatalf("status %q not cut to 40", status)
	}
}
```

`TestViewerKeysFollowKitty` adds `"Copy", "Clear selection"` to its wanted labels. `TestViewerKeyDispatch` adds: `m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})` with no selection returns a nil command and leaves `m.top` as it was; `press(m, 'y')` with no selection returns nil. `TestViewerStatusNamesMode` is unchanged (its width is 200). The test file imports `bytes`, `context`, `fmt`, `unicode/utf8`, `github.com/charmbracelet/x/ansi` and `nimbleterminal.dev/ntfonts` in addition to what it has.

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go -C examples test -mod=readonly ./typeset`
Expected: FAIL to compile on `m.sel`, `m.selected`, `m.last`.

- [ ] **Step 4: Implement the viewer**

In `viewer.go`:
- Fields `sel`, `selected`, `dragging`, `dragFrom`, `view`, `note`, `last *image.RGBA` (the raster `redraw` last submitted).
- `show` seeds `sel` from `p.sel`.
- Bindings `copy` and `clear` in `bindings`, `newBindings` and `keys()` as the Interfaces block says.
- `func (m *viewer) pageAt(x, y int, clamp bool) (image.Point, bool)`: the Global Constraints formula with `m.geom`, clamped to `g.Pixels - (1,1)` when `clamp`, else false outside `g.Cells`; returns `m.view.Unproject(pixel)`.
- `func (m *viewer) live() bool`: the stale guard from Global Constraints.
- `func (m *viewer) mouse(msg tea.Msg) (tea.Cmd, bool)`: gloss's `typesetSelectMouse` with the spec's rules; when `!m.live()` set `dragging = false` and return `nil, false`. Motion: compute the point; if `dragging` and the point equals the last focus point (keep `dragTo image.Point`), return `nil, true`; else set `sel` and return `m.redraw(), true`. Release: if `m.selected == ""` set `m.sel = nil`.
- `Update`: `case tea.MouseClickMsg, tea.MouseMotionMsg, tea.MouseReleaseMsg:` call `m.mouse`; on `true` return its command; otherwise fall through to the existing picture path. A `KeyPressMsg` other than `y` clears `note` before dispatch; a click clears it.
- `key`: `copy` → if `m.selected == ""` return `m, nil`; else set the note and return `tea.SetClipboard(m.selected)`. `clear` → if `m.sel == nil` return `m, nil`; else `m.sel, m.selected = nil, ""` and redraw.
- `redraw`: after `Ensure`, if `m.sel != nil` call `lay.Select(ctx, *m.sel)`; on error set `m.err` and return nil; a non-nil result sets `m.sel` to `Places()` (as a new pointer) and `m.selected` to `Text()`, and goes into `RasterOptions.Selection` through `render`; a nil result sets `m.selected = ""` and, when `!m.dragging`, `m.sel = nil`. Store `m.view = v` and `m.last = img` before `SetImageFor`.
- `View`: build the status, append `· N selected · y copies` when `m.selected != ""`, replaced by `· ` + note when `note != ""`; `ansi.Truncate(status, m.width, "…")` when `m.width > 0`; set `v.MouseMode = tea.MouseModeCellMotion`.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go -C examples test -mod=readonly ./typeset ./internal/...`
Expected: PASS, including `TestViewerKeysFitBooba`. If the 2048-byte check fails, shorten only the two new labels (`Copy`, `Clear`) and record it.

- [ ] **Step 6: Lint and vet**

Run: `task lint vet`
Expected: 0 issues.

- [ ] **Step 7: Commit**

```bash
git add examples/typeset
git commit -m "Select text with the mouse in the typeset demo"
```

---

### Task 3: The site and the smoke test

**Files:**
- Modify: `web/site.js`
- Modify: `scripts/smoke-demo.mjs`

**Interfaces:**
- Consumes: the viewer's mouse handling and `Copy` action from Task 2.

- [ ] **Step 1: Site**

In `web/site.js`, the `BoobaTerminal` options gain `allowOSC52: true`. The typeset help text gains ` · drag selects · y copies · Esc clears` before ` · Q keeps the screen`.

- [ ] **Step 2: Smoke test**

In `scripts/smoke-demo.mjs`, inside `if (demo === 'typeset')`, after the scroll check:

```js
    assert.ok(enabled('Copy'), 'copy action');
    bubbletea_write('\x1b[<0;5;2M');
    bubbletea_write('\x1b[<32;40;12M');
    assert.ok((await outputFollows()).length > 0, 'drag produces output');
    bubbletea_write('\x1b[<0;40;12m');
    bubbletea_write('y');
    let copied = '';
    for (let n = 0; n < 100 && !/\x1b\]52;c;[A-Za-z0-9+/=]+/.test(copied); n++) {
      await delay(25);
      copied += bubbletea_read();
    }
    assert.ok(/\x1b\]52;c;[A-Za-z0-9+/=]+/.test(copied), 'y writes the selection to the clipboard');
```

- [ ] **Step 3: Build and run the site checks**

Run: `task build-wasm-site check-site test-wasm`
Expected: both smoke runs pass; the typeset line reports the copy.

- [ ] **Step 4: Commit**

```bash
git add web/site.js scripts/smoke-demo.mjs
git commit -m "Copy a mouse selection in the browser demo"
```

---

### Task 4: Documentation and the full gate

**Files:**
- Modify: `examples/typeset/sample.md`
- Modify: `README.md`, `DEVELOP.md`, `AGENTS.md`, `CHANGELOG.md`

- [ ] **Step 1: sample.md**

After the sentence ending `` and `j`/`k` to scroll. `` in the first paragraph, add: `Drag with the mouse to select text, and press `y` to copy it.`

- [ ] **Step 2: README**

The Demo paragraph: `with keys to scroll, cycle the looks, swap the code face and switch palettes, a mouse drag to select text and `y` to copy it, or writes the top of the page as a PNG:`. The Selection bullet gains `; `View.Unproject` maps a raster pixel back to the page`.

- [ ] **Step 3: DEVELOP**

The manual check paragraph: after `toggle dark and mono,` add `drag over the page and press `y` and check the clipboard,`.

- [ ] **Step 4: AGENTS**

`view.go` row: `` `View`: the region of the page and the raster it maps to; `Project` and `Unproject`. `` The `examples/typeset` row: `` ... through ntpicture, with mouse selection and `y` to copy, or as a PNG with `-o`. ``

- [ ] **Step 5: CHANGELOG**

Under `## Unreleased`, after the `View.Unproject` bullet: `` - `examples/typeset` selects text with a left drag, copies it with `y` and clears it with `Esc` or a click, natively and on the site. ``

- [ ] **Step 6: Full gate**

Run: `task ci`
Expected: exit 0 (vulncheck needs network).

- [ ] **Step 7: Commit**

```bash
git add examples/typeset/sample.md README.md DEVELOP.md AGENTS.md CHANGELOG.md
git commit -m "Document mouse selection in the typeset demo"
```
