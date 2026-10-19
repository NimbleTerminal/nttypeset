# Example Site Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A GitHub Pages site that runs the nttypeset demos in the browser through booba and ghostty-web, built and checked by `task ci`, with a new font-specimen demo beside the existing typeset viewer.

**Architecture:** The examples module gains a `demorun` package that publishes each demo's key bindings to booba and runs the program; both demos are compiled to js/wasm through a `wasm.work` workspace that swaps in the browser Bubble Tea fork, and `web/` holds a committed page plus generated assets. Node scripts validate the built site and drive each wasm binary through booba's bridge. The notices generator grows to credit the demos and the browser runtime.

**Tech Stack:** Go 1.27.2, Bubble Tea v2 (upstream natively, `github.com/neomantra/bubbletea/v2` fork for wasm), bubbles v2.2.1, go-booba v0.9.2, ntpicture v0.1.0, ntfonts v0.1.0, Node 24 (CI) for site checks, GitHub Actions Pages.

**Spec:** `docs/superpowers/specs/2026-10-18-example-site-design.md`

## Global Constraints

- The library (`nimbleterminal.dev/nttypeset`, every package outside `examples/` and `internal/tools`) is not changed. `git diff main --stat -- '*.go' ':!examples' ':!internal/tools'` is empty at the end.
- `AGENTS.md` holds: core imports only go-text, chroma, x/image; `examples/go.mod` replaces nttypeset with `..` and never adds sibling `replace` directives; no panic crosses the public boundary.
- Versions, exact: `github.com/NimbleMarkets/go-booba v0.9.2`, `charm.land/bubbles/v2 v2.2.1`, `charm.land/bubbletea/v2 v2.1.0` natively; wasm replace `charm.land/bubbletea/v2 => github.com/neomantra/bubbletea/v2 v2.0.0-20261009201241-c397c32a5074`; `nimbleterminal.dev/ntpicture v0.1.0`; `nimbleterminal.dev/ntfonts v0.1.0`. Node `24` in CI.
- Booba app name is `nttypeset`. Demo contexts are `typeset` and `specimen`; summaries `Typeset a page` and `Font specimen`.
- Toggle key is `t`, label `Glyph / Kitty`, enabled only while `pic.KittySupported() == nt.KittyCapabilitySupported`.
- Generated site assets are never committed: `*.wasm`, `/web/demos/`, `/web/booba/`, `/web/ghostty-web/`, `/web/wasm_exec.js`, `/web/LICENSE`, `/web/THIRD_PARTY_NOTICES.md` are ignored.
- Wasm builds use `-trimpath -ldflags='-s -w'`, `CGO_ENABLED=0`, `GOWORK={{.ROOT_DIR}}/wasm.work`.
- Every task runs through `Taskfile.yml`; `task ci` passes locally before any task is reported complete, and the final branch passes on both GitHub runners.
- `CHANGELOG.md` entries go under `## Unreleased`.

## Review Focus

1. A terminal narrower than the help row: the help must truncate on one line, never wrap into the picture. Pinned in Task 1 (`TestKeysViewTruncates`).
2. A terminal without Kitty graphics: `t` must be harmless and the button must not be advertised. Pinned in Task 2 and Task 3 (`...FollowKitty` tests) and the smoke scripts in Task 4.
3. A picture smaller than 160 pixels in either dimension: `Preview` rejects it, so the specimen must clamp and fall back to `SetImage`. Pinned in Task 3 (`TestFit`).
4. The first `redraw` before the picture knows its device pixels: both demos must draw nothing and set no error. Pinned in Task 3 (`TestRedrawBeforeGeometry`); the typeset viewer already guards this.
5. A wasm build whose Bubble Tea is upstream rather than the fork: it fails to compile, so `task build-wasm` must set `GOWORK` itself and `test-wasm` must prove the fork runs. Pinned in Task 4 (`task test-wasm` in `ci`).

---

### Task 1: Browser workspace and the demorun package

**Files:**
- Create: `wasm.work`, `wasm.work.sum` (generated)
- Create: `examples/internal/demorun/run.go`, `examples/internal/demorun/run_test.go`
- Modify: `examples/go.mod`, `examples/go.sum`, `.gitignore`

**Interfaces:**
- Produces: package `demorun` with
  - `type Keys struct { Primary, More []key.Binding }`
  - `func (k Keys) ShortHelp() []key.Binding` (Primary) and `func (k Keys) FullHelp() [][]key.Binding` (`{Primary, More}`)
  - `func (k Keys) View(width int) string`: `help.New()` with `Width = width`, `ShortHelpView` of Primary followed by More, so the row truncates with bubbles' ellipsis at `width`.
  - `func Binding(label string, shortcuts ...string) key.Binding`: `key.WithKeys(shortcuts...)`, `key.WithHelp(shortcuts[0], label)`.
  - `type Model interface { tea.Model; HelpKeys() Keys }`
  - `func Run(m Model, context, summary string) error`: wraps with `bubblehelp.NewModel(m, func(inner tea.Model) bubblehelp.Description { return bubblehelp.Description{Context: context, Keys: inner.(Model).HelpKeys(), Status: actions.ProgramStatus{App: "nttypeset", Summary: summary}} }, bubblehelp.ModelOptions{Adapter: bubblehelp.Options{Customize: func(_ key.Binding, a *actions.Action) error { first := 0; a.Preferred = &first; return nil }}, OnError: func(err error) { fmt.Fprintln(os.Stderr, err) }})` then `booba.Run(wrapped)`. Copy ntpicture's `examples/internal/demohelp/help.go` `Run` for the exact calls.

- [ ] **Step 1: Add dependencies and the booba-assets tool**

In `examples/`: `GOWORK=off go get github.com/NimbleMarkets/go-booba@v0.9.2 charm.land/bubbles/v2@v2.2.1` and add the line `tool github.com/NimbleMarkets/go-booba/cmd/booba-assets` to `examples/go.mod` (then `GOWORK=off go mod tidy` there). Confirm `charm.land/bubbletea/v2 v2.1.0` is unchanged.

- [ ] **Step 2: Write `wasm.work`**

```
// Browser-only Bubble Tea fork; validate the demo bridge when updating.
// Native consumers use the upstream dependency in examples/go.mod.
go 1.26.0

use (
	.
	./examples
)

replace charm.land/bubbletea/v2 => github.com/neomantra/bubbletea/v2 v2.0.0-20261009201241-c397c32a5074
```

Run `GOWORK=$PWD/wasm.work go work sync` to create `wasm.work.sum`; commit both.

- [ ] **Step 3: Extend `.gitignore`**

Append `*.wasm`, `/web/demos/`, `/web/booba/`, `/web/ghostty-web/`, `/web/wasm_exec.js`, `/web/LICENSE`, `/web/THIRD_PARTY_NOTICES.md`.

- [ ] **Step 4: Write the failing tests** `examples/internal/demorun/run_test.go`

```go
func TestBindingUsesFirstShortcutForHelp(t *testing.T) {
	b := Binding("Down", "j", "down")
	if b.Help().Key != "j" || b.Help().Desc != "Down" || !key.Matches(tea.KeyPressMsg{Code: tea.KeyDown}, b) { t.Fatalf("%+v", b.Help()) }
}

func TestKeysViewTruncates(t *testing.T) {
	k := Keys{Primary: []key.Binding{Binding("Down", "j"), Binding("Up", "k"), Binding("Page down", "space")}, More: []key.Binding{Binding("Quit", "q")}}
	wide := k.View(200)
	if !strings.Contains(wide, "Quit") || strings.Contains(wide, "\n") { t.Fatalf("wide: %q", wide) }
	narrow := k.View(16)
	if lipgloss.Width(narrow) > 16 || strings.Contains(narrow, "\n") { t.Fatalf("narrow: %q", narrow) }
}

func TestKeysSnapshotHasNoDiagnostics(t *testing.T) {
	k := Keys{Primary: []key.Binding{Binding("Down", "j", "down")}, More: []key.Binding{Binding("Quit", "q", "ctrl+c")}}
	snapshot, diagnostics, err := bubblehelp.Snapshot(k, "nttypeset", "demo", 1, bubblehelp.Options{})
	if err != nil || len(diagnostics) != 0 { t.Fatal(err, diagnostics) }
	if len(snapshot.Actions) != 2 || snapshot.Actions[1].Label != "Quit" { t.Fatalf("%+v", snapshot.Actions) }
}
```

Use `charm.land/lipgloss/v2` only if it is already an indirect dependency after Step 1; otherwise measure width with `ansi.StringWidth` from `github.com/charmbracelet/x/ansi`, which the module already has.

- [ ] **Step 5: Run the tests to verify they fail**

Run: `GOWORK=off go -C examples test -mod=readonly ./internal/demorun/`
Expected: FAIL, package does not exist.

- [ ] **Step 6: Implement `examples/internal/demorun/run.go`** with the interfaces above. Package comment: "Package demorun publishes a demo's key bindings to booba hosts and runs it."

- [ ] **Step 7: Run the tests and the two builds**

Run: `GOWORK=off go -C examples test -mod=readonly ./internal/demorun/`
Expected: PASS.
Run: `GOWORK=$PWD/wasm.work GOOS=js GOARCH=wasm CGO_ENABLED=0 go -C examples build -o /dev/null ./typeset`
Expected: builds (the fork makes Bubble Tea compile for wasm).
Run: `task go-tidy-check test vet lint`
Expected: pass.

- [ ] **Step 8: Commit**

```bash
git add wasm.work wasm.work.sum .gitignore examples/go.mod examples/go.sum examples/internal/demorun
git commit -m "Add the browser workspace and the demorun package"
```

### Task 2: Typeset viewer on booba bindings

**Files:**
- Modify: `examples/typeset/viewer.go`, `examples/typeset/main.go`
- Create: `examples/typeset/viewer_test.go`

**Interfaces:**
- Consumes: `demorun.Keys`, `demorun.Binding`, `demorun.Run`, `demorun.Model` from Task 1.
- Produces: `func (m *viewer) HelpKeys() demorun.Keys` and the viewer satisfying `demorun.Model`. Behavior otherwise as today.

- [ ] **Step 1: Write the failing tests** `examples/typeset/viewer_test.go`

```go
func TestViewerKeysFollowKitty(t *testing.T) {
	m := &viewer{pic: nt.New(nt.Options{AutoMode: true}), o: options{font: "readable"}}
	// A fresh picture has not resolved Kitty support: the toggle is hidden.
	snapshot, diagnostics, err := bubblehelp.Snapshot(m.HelpKeys(), "nttypeset", "typeset", 1, bubblehelp.Options{})
	if err != nil || len(diagnostics) != 0 { t.Fatal(err, diagnostics) }
	labels := map[string]bool{}
	for _, a := range snapshot.Actions { labels[a.Label] = a.Enabled }
	for _, want := range []string{"Down", "Up", "Page down", "Page up", "Top", "End", "Dark / Light", "Next look", "Mono", "Line numbers", "Chop lines", "Keep screen & quit", "Quit"} {
		if !labels[want] { t.Errorf("%s not enabled in %v", want, labels) }
	}
	if labels["Glyph / Kitty"] { t.Fatal("toggle advertised without Kitty support") }
}

func TestViewerStatusNamesMode(t *testing.T) {
	m := &viewer{pic: nt.New(nt.Options{AutoMode: true}), page: &page{title: "sample.md"}, o: options{font: "readable", mono: "jetbrains"}, width: 100}
	v := m.View()
	for _, want := range []string{"sample.md", "readable+jetbrains", "light", "glyph", "Quit"} {
		if !strings.Contains(v.Content, want) { t.Errorf("view lacks %q:\n%s", want, v.Content) }
	}
}
```

Check `tea.View`'s field name for the content string in bubbletea v2.1.0 (`Content`); adjust the test if it differs. Both tests must also build under `GOOS=js GOARCH=wasm`, so they must not touch the filesystem.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `GOWORK=off go -C examples test -mod=readonly -run 'TestViewer' ./typeset/`
Expected: FAIL, `HelpKeys` undefined.

- [ ] **Step 3: Rewrite the viewer's key handling on bindings**

In `viewer.go`:
- Add field `width int` (terminal columns from `WindowSizeMsg`) to `viewer`.
- `HelpKeys()` builds, with `demorun.Binding`: Primary `Down (j, down)`, `Up (k, up)`, `Page down (space, pgdown)`, `Page up (pgup)`, `Top (g, home)`, `End (G, end)`, `Dark / Light (d)`, `Next look (f)`, `Mono (m)`, `Glyph / Kitty (t)`; More `Line numbers (n)`, `Chop lines (c)`, `Keep screen & quit (Q)`, `Quit (q, ctrl+c)`. Call `toggle.SetEnabled(m.pic.KittySupported() == nt.KittyCapabilitySupported)` before returning.
- `Update`: on `tea.KeyPressMsg` call `m.key(msg)`; `key` switches with `key.Matches(msg, b)` over the bindings in the same order and effects as today, plus the toggle case: `if c := m.pic.ToggleMode(); c != nil { return m, tea.Batch(c, m.redraw()) }`. Keep the `default: return m, nil`.
- `WindowSizeMsg`: store `m.width = msg.Width`; `SetSize(max(0, msg.Width), max(0, msg.Height-2))`.
- `View`: status row `fmt.Sprintf("%s · %s%s · %s · %s · top %d", title, font, suffix(mono), palette, mode, top)` where `mode` is `"kitty"` when `m.pic.Mode() == nt.PictureKitty`, else `"glyph"`; the error appended as today; then `"\n" + m.HelpKeys().View(m.width)`. Picture, status, help: three parts joined with newlines.
- `show`: `return demorun.Run(&viewer{...}, "typeset", "Typeset a page")` in place of `tea.NewProgram(...).Run()`; keep `defer pic.Close()`.
- Remove the hand-written help string.

- [ ] **Step 4: Run the tests**

Run: `GOWORK=off go -C examples test -mod=readonly ./typeset/`
Expected: PASS, including the existing four tests.

- [ ] **Step 5: Run the native demo under a pseudo-terminal for a panic check**

Run: `task build-examples && perl -e 'alarm 4; exec @ARGV' script -q -F /dev/null ./bin/nttypeset-typeset </dev/null; echo exit=$?`
Expected: exit 142 (killed by the alarm), no panic text. On Linux use `timeout 4 script -qfc ./bin/nttypeset-typeset /dev/null`.

- [ ] **Step 6: Commit**

```bash
git add examples/typeset
git commit -m "Publish the typeset viewer's keys to booba"
```

### Task 3: The specimen demo

**Files:**
- Create: `examples/specimen/main.go`, `examples/specimen/viewer.go`, `examples/specimen/main_test.go`
- Modify: `Taskfile.yml` (`build`, `build-examples`, `run-specimen`, `cross-windows`), `README.md` Demo section (one line for the specimen)

**Interfaces:**
- Consumes: `nttypeset.Preview(ctx, looks, override, body, code string, width, height int, p Palette, opts ...Option) (*image.RGBA, error)`, `nttypeset.FontLooks()`, `ntfonts.Embedded`, `demorun` from Task 1, ntpicture's `Picture`.
- Produces: `func render(ctx context.Context, override string, dark bool, width, height int) (*image.RGBA, error)`; `func fit(p image.Point) (width, height int)` clamping each axis to `[160, 1200]`; `type viewer struct` with `HelpKeys()`.

- [ ] **Step 1: Write the failing tests** `examples/specimen/main_test.go`

```go
const body = "Sphinx of black quartz, judge my vow. Café, naïve, résumé — 0123456789"
const code = "func (e *Engine) Layout(doc *Document, width int) (*Layout, error)"

func TestFit(t *testing.T) {
	for _, tc := range []struct{ in image.Point; w, h int }{{image.Pt(800, 600), 800, 600}, {image.Pt(100, 100), 160, 160}, {image.Pt(4000, 50), 1200, 160}, {image.Pt(0, 0), 160, 160}} {
		if w, h := fit(tc.in); w != tc.w || h != tc.h { t.Errorf("fit(%v) = %d×%d, want %d×%d", tc.in, w, h, tc.w, tc.h) }
	}
}

func TestRenderPaintsBothPalettes(t *testing.T) {
	ctx := context.Background()
	light, err := render(ctx, "", false, 800, 600)
	if err != nil { t.Fatal(err) }
	if got := light.Bounds().Size(); got != image.Pt(800, 600) { t.Fatalf("size %v", got) }
	if countNot(light, nttypeset.LightPalette().Background) == 0 { t.Fatal("light specimen has no ink") }
	dark, err := render(ctx, "", true, 800, 600)
	if err != nil { t.Fatal(err) }
	if countNot(dark, nttypeset.DarkPalette().Background) == 0 { t.Fatal("dark specimen has no ink") }
	mono, err := render(ctx, "jetbrains", false, 800, 600)
	if err != nil { t.Fatal(err) }
	if bytes.Equal(mono.Pix, light.Pix) { t.Fatal("the mono override changed no pixel") }
}

func TestSpecimenKeysFollowKitty(t *testing.T) {
	m := &viewer{pic: nt.New(nt.Options{AutoMode: true})}
	snapshot, diagnostics, err := bubblehelp.Snapshot(m.HelpKeys(), "nttypeset", "specimen", 1, bubblehelp.Options{})
	if err != nil || len(diagnostics) != 0 { t.Fatal(err, diagnostics) }
	labels := map[string]bool{}
	for _, a := range snapshot.Actions { labels[a.Label] = a.Enabled }
	for _, want := range []string{"Dark / Light", "Mono", "Keep screen & quit", "Quit"} {
		if !labels[want] { t.Errorf("%s not enabled", want) }
	}
	if labels["Glyph / Kitty"] { t.Fatal("toggle advertised without Kitty support") }
}

func TestRedrawBeforeGeometry(t *testing.T) {
	m := &viewer{ctx: context.Background(), pic: nt.New(nt.Options{AutoMode: true})}
	if cmd := m.redraw(); cmd != nil || m.err != nil { t.Fatalf("cmd %v err %v", cmd, m.err) }
}
```

`countNot` is copied from `examples/typeset/main_test.go` (a test helper, duplicated across two main packages). The `body` and `code` constants live in `main.go`; the test file declares none.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `GOWORK=off go -C examples test -mod=readonly ./specimen/`
Expected: FAIL, no Go files.

- [ ] **Step 3: Implement `examples/specimen/main.go`**

Package comment: "Command specimen is the nttypeset font specimen: one row per look from nttypeset.Preview, shown in the terminal through ntpicture, or written as a PNG with -o." Flags: `-dark`, `-mono` (`"jetbrains"` or empty), `-o`, `-width` 800, `-height` 600. `main` parses, and either `render` + PNG (as typeset's `run` does) or `show(ctx, o)`. `render` calls `nttypeset.Preview(ctx, nttypeset.FontLooks(), override, body, code, width, height, palette, nttypeset.WithFonts(ntfonts.Embedded))` with `LightPalette`/`DarkPalette`. `fit` clamps with `min(max(p.X, 160), 1200)` per axis.

- [ ] **Step 4: Implement `examples/specimen/viewer.go`**

`type viewer struct { ctx context.Context; pic *nt.Picture; dark bool; mono string; width int; geom nt.RasterGeometry; err error }`. `show` creates the picture with `AutoMode: true`, defers `Close`, and calls `demorun.Run(&viewer{...}, "specimen", "Font specimen")`. `Init` returns `pic.Init()`. `Update` mirrors typeset's: keys through `key.Matches`; `WindowSizeMsg` stores width and `SetSize(max(0, w), max(0, h-1))`; geometry change triggers `redraw`. `HelpKeys`: Primary `Dark / Light (d)`, `Mono (m)`, `Glyph / Kitty (t)` gated on Kitty support; More `Keep screen & quit (Q)`, `Quit (q, ctrl+c)`. `redraw`: `g := pic.RasterGeometry()`; if either pixel dimension is `<= 0` return nil without touching `err`; `w, h := fit(g.Pixels)`; `img, err := render(...)`; if `image.Pt(w, h) == g.Pixels` then `pic.SetImageFor(img, g)` else `pic.SetImage(img)`. `View`: picture, newline, then `specimen · light|dark[ · +jetbrains] · glyph|kitty` followed by two spaces and `HelpKeys().View(width)` on the same row, `AltScreen = true`.

- [ ] **Step 5: Run the tests**

Run: `GOWORK=off go -C examples test -mod=readonly ./specimen/`
Expected: PASS.

- [ ] **Step 6: Wire the Taskfile and README**

`build-examples` desc "Build bin/nttypeset-typeset and bin/nttypeset-specimen", add `go -C examples build -mod=readonly -o ../bin/nttypeset-specimen ./specimen`. Add `run-specimen` ("Run the font specimen (task run-specimen -- -dark)") running `go -C examples run -mod=readonly ./specimen {{.CLI_ARGS}}`. `cross-windows` adds `-o ../dist/nttypeset-specimen.exe ./specimen`. README Demo section gains, after the code block: "`examples/specimen` shows one row per look from `Preview`: `task run-specimen -- -dark`, or `-o specimen.png`."

- [ ] **Step 7: Write a PNG and look at it**

Run: `task run-specimen -- -o /tmp/specimen.png && task run-specimen -- -dark -mono jetbrains -o /tmp/specimen-dark.png`
Expected: two PNGs of 800×600 with four rows each; the implementer reports what the rows show and whether the accents and em dash rendered (a visual check, stated as such).

- [ ] **Step 8: Run the gate so far and commit**

Run: `task ci`
Expected: pass.

```bash
git add examples/specimen Taskfile.yml README.md
git commit -m "Add the specimen demo"
```

### Task 4: The site, its checks and the wasm tasks

**Files:**
- Create: `web/index.html`, `web/site.css`, `web/site.js`, `web/licenses/ghostty-web.txt`, `web/licenses/ghostty.txt`, `web/licenses/pako.txt`, `scripts/check-site.mjs`, `scripts/smoke-demo.mjs`
- Modify: `Taskfile.yml` (`build-wasm`, `build-wasm-site`, `check-site`, `serve-wasm-site`, `test-wasm`, `vulncheck-wasm`, `ci`, `clean`)

**Interfaces:**
- Consumes: `web/demos/typeset.wasm` and `web/demos/specimen.wasm` built from Tasks 2 and 3; booba's exported `web/booba/booba.js` (`BoobaTerminal`, `installActionBar`, `installRendererHud`, `parseRendererFromURL`); the booba bridge globals `bubbletea_read`, `bubbletea_write`, `bubbletea_resize`, `bubbletea_exited` under Node.
- Produces: `task build-wasm-site` leaves a complete `web/`; `task check-site` passes.

- [ ] **Step 1: Copy the licenses and stylesheet**

`cp ~/projects/ntpicture/web/licenses/*.txt web/licenses/` (three files) and `cp ~/projects/ntqrcode/web/site.css web/site.css`, unchanged.

- [ ] **Step 2: Write `web/index.html`**

ntqrcode's structure with: `<title>nttypeset · typesetting for the terminal</title>`; brand `nttypeset<span>Typesetting for the terminal.</span>`; source link `https://github.com/NimbleTerminal/nttypeset`; two cards in order:

```html
<a href="?demo=typeset" data-demo="typeset"><strong>Typeset a page</strong><span>The sample Markdown page, laid out with the embedded fonts. Scroll it, cycle the looks, swap the code face, go dark.</span></a>
<a href="?demo=specimen" data-demo="specimen"><strong>Font specimen</strong><span>One row per look: Literata, Newsreader, IBM Plex Sans and Source Serif 4 for body text, Iosevka and IBM Plex Mono for code, with JetBrains Mono as the override.</span></a>
```

The toolbar has `<h1 id="title">Typeset a page</h1>` and `<span><a id="source" href="https://github.com/NimbleTerminal/nttypeset/tree/main/examples/typeset">Example source ↗</a> · <a id="restart" href="?demo=typeset">Restart ↻</a></span>` (ntpicture's toolbar). Status, help, footer and the two script tags as ntqrcode's.

- [ ] **Step 3: Write `web/site.js`**

ntqrcode's loader with `demos = { typeset: 'Typeset a page', specimen: 'Font specimen' }`, default `typeset`, `document.title = \`${demos[demo]} · nttypeset\``, source link `https://github.com/NimbleTerminal/nttypeset/tree/main/examples/${demo}`, help text per demo:
- typeset: `j/k or arrows scroll · space and PgUp page · g/G ends · d dark · f next look · m mono · t glyph/Kitty · n numbers · c chop · Q keeps the screen · q quits. Click the terminal to focus.`
- specimen: `d dark · m mono · t glyph/Kitty · Q keeps the screen · q quits. Click the terminal to focus.`

Before `fetch`, set `status.textContent = 'Downloading the demo (about 6 MB)…'`. Everything else as ntqrcode's.

- [ ] **Step 4: Write `scripts/check-site.mjs`**

ntqrcode's, with the demo list asserted `['typeset', 'specimen']`, the `id="source"` assertion from ntpicture's, and existence checks for `examples/typeset` and `examples/specimen`.

- [ ] **Step 5: Write `scripts/smoke-demo.mjs`**

ntpicture's shape, `demo` from `process.argv[2]` in `['typeset', 'specimen']`, `keep` flag. Sequence: wait for `bubbletea_read`; `bubbletea_resize(100, 40)`; collect output until it contains `quit` (lower-cased); assert the `\x1b]7501;?` query was sent; answer it and parse the `booba-actions/v1` snapshot; assert `Quit` enabled; assert no action labelled `Glyph / Kitty` is enabled. For `typeset`: assert `Down`, `Next look`, `Dark / Light` enabled; `bubbletea_write('j')` and assert output follows within 100 polls. For `specimen`: `bubbletea_write('d')` and assert output follows. Then `bubbletea_resize(60, 20)` and assert output follows. Exit with `Q` when `keep`, else the quit key from the snapshot; await `running`; assert exit code 0 and `bubbletea_exited === true`; `process.exit(0)`. 15 s timeout as ntpicture's.

- [ ] **Step 6: Add the Taskfile tasks**

```yaml
  build-wasm:
    desc: Build the browser demos into web/demos/
    env:
      GOWORK: '{{.ROOT_DIR}}/wasm.work'
      GOOS: js
      GOARCH: wasm
      CGO_ENABLED: '0'
    cmds:
      - mkdir -p web/demos
      - for: [typeset, specimen]
        cmd: go -C examples build -mod=readonly -trimpath -ldflags='-s -w' -o ../web/demos/{{.ITEM}}.wasm ./{{.ITEM}}
  build-wasm-site:
    desc: Build the browser demos and export the pinned booba runtime into web/
    cmds:
      - task: build-wasm
      - go -C examples tool booba-assets --no-index ../web
      - cp LICENSE THIRD_PARTY_NOTICES.md web/
      - task: check-site
  check-site:
    desc: Check site script syntax, required assets, and both demos under Node
    cmds:
      - node --check web/site.js
      - node scripts/check-site.mjs
      - node scripts/smoke-demo.mjs typeset
      - node scripts/smoke-demo.mjs specimen
      - node scripts/smoke-demo.mjs typeset keep
  serve-wasm-site:
    desc: Serve web/ at http://localhost:8000/nttypeset/ (matches the Pages prefix)
    deps: [build-wasm-site]
    vars: {SERVE_DIR: /tmp/nttypeset-site}
    cmds:
      - mkdir -p "{{.SERVE_DIR}}"
      - ln -sfn "$PWD/web" "{{.SERVE_DIR}}/nttypeset"
      - python3 -m http.server 8000 --bind 127.0.0.1 --directory "{{.SERVE_DIR}}"
  test-wasm:
    desc: Test the examples module under js/wasm with Node
    env:
      GOWORK: '{{.ROOT_DIR}}/wasm.work'
      GOOS: js
      GOARCH: wasm
      CGO_ENABLED: '0'
    cmds:
      - 'go -C examples test -mod=readonly -exec="$(go env GOROOT)/lib/wasm/go_js_wasm_exec" ./...'
  vulncheck-wasm:
    desc: Scan the browser build's dependencies (requires network)
    env:
      GOWORK: '{{.ROOT_DIR}}/wasm.work'
      GOOS: js
      GOARCH: wasm
      CGO_ENABLED: '0'
    cmds:
      - task: vulncheck-install
      - cmd: '"{{.ROOT_DIR}}/.task/bin/govulncheck" ./...'
        dir: examples
```

`ci` appends `build-wasm-site`, `test-wasm`, `vulncheck-wasm` after `vulncheck`. `clean` becomes `rm -rf bin dist .task web/demos web/booba web/ghostty-web web/wasm_exec.js web/LICENSE web/THIRD_PARTY_NOTICES.md`. The `go -C examples tool booba-assets` call needs `GOWORK=off` (the Taskfile's global env already sets it).

- [ ] **Step 7: Build and check**

Run: `task build-wasm-site`
Expected: `web/demos/typeset.wasm` and `specimen.wasm` exist, `web/booba/booba.js` exported, `check-site` prints that the site links, runtime assets and two WASM binaries checked, and three smoke lines pass. `task test-wasm` passes. `git status` shows nothing generated as untracked.

- [ ] **Step 8: Commit**

```bash
git add web scripts Taskfile.yml
git commit -m "Add the browser demo site and its checks"
```

### Task 5: Notices for the demos and the browser runtime

**Files:**
- Modify: `internal/tools/notices/main.go`, `NOTICE.md`, `THIRD_PARTY_NOTICES.md` (regenerated)

**Interfaces:**
- Consumes: `wasm.work` at the repository root; `web/licenses/*.txt` from Task 4.
- Produces: `THIRD_PARTY_NOTICES.md` with sections for every module the demos link natively and under wasm, and three `Browser runtime:` sections.

- [ ] **Step 1: Extend the generator**

After the platform loop, run `go list -deps -f <same template> ./...` twice with `cmd.Dir = "examples"`: once with `GOWORK=off`, once with `GOOS=js`, `GOARCH=wasm`, `GOWORK=<filepath.Abs("wasm.work")>` (ntpicture's `mustWorkspace`). Merge into the same `modules` map, skipping any line whose path is `nimbleterminal.dev/nttypeset` (the local replace) and any with an empty version. Change the header to:

```
nttypeset is MIT licensed; see LICENSE. The Go standard library and the
dependencies below are linked by the library, the demos in examples/ and the
browser site. Generated by internal/tools/notices.
```

After the module sections and the missing-license check, append `section(&b, "Browser runtime: "+name, "", text)` for `ghostty-web`, `ghostty`, `pako` read from `web/licenses/<name>.txt`, as ntpicture's generator does.

- [ ] **Step 2: Regenerate and inspect**

Run: `task notices && grep -c '^## ' THIRD_PARTY_NOTICES.md && grep -n '^## \(github.com/NimbleMarkets/go-booba\|github.com/neomantra/bubbletea\|nimbleterminal.dev/ntfonts\|nimbleterminal.dev/ntpicture\|Browser runtime\)' THIRD_PARTY_NOTICES.md`
Expected: booba, the Bubble Tea fork, ntfonts, ntpicture and three browser runtime sections present; no `nimbleterminal.dev/nttypeset` section. `task notices-check` passes.

- [ ] **Step 3: Update `NOTICE.md`**

Add two paragraphs: the generated demo site includes booba and ghostty-web runtime assets, with their license texts in THIRD_PARTY_NOTICES.md copied into the site, and `web/licenses/` copied from go-booba's vendored ghostty-web, Ghostty and pako sources, to review when updating the pinned booba runtime; and `examples/typeset/landscape.png` is an original flat-shaded illustration drawn for gloss, Copyright Neomantra Corp, carrying no third-party notice.

- [ ] **Step 4: Commit**

```bash
git add internal/tools/notices/main.go NOTICE.md THIRD_PARTY_NOTICES.md
git commit -m "Credit the demos' and the browser runtime's licenses"
```

### Task 6: CI, Pages and the documentation

**Files:**
- Modify: `.github/workflows/ci.yml`, `DEVELOP.md`, `AGENTS.md`, `CHANGELOG.md`, `README.md`
- Create: `.github/workflows/pages.yml`

- [ ] **Step 1: `ci.yml`**

Add `wasm.work.sum` to `cache-dependency-path` and, after setup-go, `- uses: actions/setup-node@v7` with `node-version: '24'`. Both matrix runners keep `task ci`.

- [ ] **Step 2: `pages.yml`**

`cp ~/projects/ntpicture/.github/workflows/pages.yml .github/workflows/pages.yml` unchanged. Confirm it uses `setup-go@v7` with Go `1.27.2`, `setup-node@v7` with Node 24, `go-task/setup-task@v2`, runs `task ci`, and uploads `web`.

- [ ] **Step 3: Documentation**

- `README.md`: under Demo, add a "Browser demos" paragraph: both demos run in the browser at `https://nimbleterminal.github.io/nttypeset/`, compiled to WebAssembly and shown through booba and ghostty-web; `task serve-wasm-site` serves the same site locally.
- `DEVELOP.md`: replace "There is no browser build." with a sentence pointing to a new `## Browser site` section. That section covers: `wasm.work` and why the fork is needed, `build-wasm`, `build-wasm-site`, `check-site`, `serve-wasm-site` (at `http://localhost:8000/nttypeset/`), `test-wasm`, `vulncheck-wasm`; that generated `web/` paths are ignored and never committed; the wasm size (about 23 MB raw, 6 MB compressed) and that it is accepted; the Pages workflow deploys `web/` from `main` after `task ci`, with Pages enabled on the repository by the maintainer; and a manual acceptance check: before announcing a site change, open both demos in a browser from `task serve-wasm-site`, scroll, cycle looks, toggle dark and mono, and record what was seen, since the Node smoke is not a visual check. Add the new tasks to the command list and update the Dependencies paragraph: the notices now cover the library, the demos and the browser runtime.
- `AGENTS.md`: commands gain `run-specimen`, `build-wasm-site`, `check-site`, `serve-wasm-site`, `test-wasm`; "What it is not" keeps "It does no terminal output" and the module layout paragraph replaces "There is no browser build." with the browser build through `wasm.work` and the fork; the dependency list adds booba and bubbles for the examples module; the file map adds `examples/specimen`, `examples/internal/demorun`, `web/`, `scripts/`, `wasm.work`; Invariants add "Generated site assets are never committed; `web/demos`, `web/booba`, `web/ghostty-web` and the copied files are build output."
- `CHANGELOG.md` Unreleased: "`examples/specimen`, a font specimen from `Preview`, one row per look." and "A browser demo site at nimbleterminal.github.io/nttypeset: both demos compiled to WebAssembly and run through booba and ghostty-web, built and checked by `task ci` and deployed by GitHub Pages. The demos publish their keys as booba actions and gain a `t` glyph/Kitty toggle. `THIRD_PARTY_NOTICES.md` now credits the demos' dependencies and the browser runtime."

- [ ] **Step 4: Full gate and the library-untouched check**

Run: `task ci && git diff main --stat -- '*.go' ':!examples' ':!internal/tools'`
Expected: `task ci` passes; the diff is empty.

- [ ] **Step 5: Commit and push the branch**

```bash
git add .github DEVELOP.md AGENTS.md CHANGELOG.md README.md
git commit -m "Build and deploy the demo site in CI"
git push -u origin feat/example-site
```

Then `gh run list --branch feat/example-site --limit 2` and `gh run watch <id> --exit-status` for the CI workflow on both runners. The Pages workflow does not run on the branch.

### Task 7: Visual check (controller)

Not a subagent task. From the branch, `task serve-wasm-site`, open `http://localhost:8000/nttypeset/?demo=typeset` and `?demo=specimen` in Chrome, and record: the page loads and the status says it runs locally; the action bar shows the buttons; scrolling, next look, dark and mono visibly change the page; the specimen shows four rows. Record the findings in the ledger. A failure here is a fix round on the task that owns the code.
