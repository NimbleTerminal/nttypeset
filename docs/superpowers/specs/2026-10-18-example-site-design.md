# nttypeset example site

A GitHub Pages site at `https://nimbleterminal.github.io/nttypeset/` that runs
the nttypeset demos in the browser, built the way the ntpicture and ntqrcode
sites are built: each demo compiled to WebAssembly and run inside ghostty-web
through the booba runtime, with booba's action bar as the on-screen keyboard.
No guide, no Hugo. ntfonts gets nothing; the specimen demo here stands in for
it.

## Why

nttypeset v0.1.0 is tagged and has a terminal demo (`examples/typeset`), but
nothing a visitor can see without cloning and building. Every other Nimble
Terminal library with a visual has a site in the booba pattern. This gives
nttypeset the same front door and shows the embedded fonts set against real
prose, tables and highlighted code, which says more about ntfonts than a table
of family names.

## Goals

- A visitor opens the site and, after one download, scrolls the sample page,
  cycles the four looks, swaps the code face, and toggles the dark palette,
  with buttons for every key.
- A second card shows a font specimen: one row per look, from
  `nttypeset.Preview`.
- `task ci` builds and checks the site, so a broken site fails CI on every
  push, and the Pages deploy only ever publishes a revision that passed.
- The same two demos still run natively (`task run-typeset`, `task
  run-specimen`) and under `-o` as PNG.

## Non-goals

- No file upload or editing in the browser. The browser build shows the
  embedded sample page only.
- No guide or prose documentation on the site beyond the cards and the help
  line. README and pkg.go.dev remain the documentation.
- No changes to the library's public API, dependencies or invariants.
- No ntfonts site, README section or example. ntfonts is unchanged.
- No shrinking of the wasm beyond `-trimpath -ldflags='-s -w'`. See Size.

## Anti-goals

- The library importing booba, Bubble Tea or ntpicture. Those stay in the
  examples module.
- Generated site assets committed to git.
- A site that works in the browser but whose native demo regressed, or the
  reverse. Both run from the same source.

## Constraints

- All of `AGENTS.md` still holds: the core imports only go-text, chroma and
  x/image; the examples module replaces nttypeset with the parent directory
  and never adds sibling `replace` directives; no panic crosses the public
  boundary.
- Go 1.27.2 toolchain, module minimum 1.26. Node 24 in CI for the site checks.
- The browser Bubble Tea is the fork the sibling sites pin:
  `charm.land/bubbletea/v2 => github.com/neomantra/bubbletea/v2 v2.0.0-20261009201241-c397c32a5074`,
  through a `wasm.work` workspace used only for js/wasm builds. Native builds
  keep upstream Bubble Tea from `examples/go.mod`. `wasm.work.sum` is
  committed.
- go-booba `v0.9.2` and bubbles `v2.2.1`, the versions ntpicture pins, with
  `tool github.com/NimbleMarkets/go-booba/cmd/booba-assets` in
  `examples/go.mod`.
- The CI matrix stays ubuntu-latest and macos-latest, and `task ci` passes on
  both. The Pages workflow runs `task ci` on ubuntu before uploading `web/`.
- Pixel output of the library is untouched: the gloss baseline
  (`scripts/typeset-baseline.sh check` in gloss) still passes against this
  branch.

## The site

`web/` holds the committed page and the generated assets beside it:

| Path | Committed | What |
| --- | --- | --- |
| `web/index.html` | yes | The page: header, two demo cards, the terminal frame, status, help, footer. Modeled on ntqrcode's. |
| `web/site.css` | yes | ntqrcode's stylesheet, copied as is. |
| `web/site.js` | yes | Picks the demo from `?demo=`, loads `demos/<demo>.wasm`, starts `BoobaTerminal`, installs the action bar and the renderer HUD. Modeled on ntqrcode's. |
| `web/licenses/{ghostty-web,ghostty,pako}.txt` | yes | Copied from ntpicture's `web/licenses/`. |
| `web/demos/typeset.wasm`, `web/demos/specimen.wasm` | no | Built by `task build-wasm`. |
| `web/booba/`, `web/ghostty-web/`, `web/wasm_exec.js` | no | Exported by `go -C examples tool booba-assets --no-index ../web`. |
| `web/LICENSE`, `web/THIRD_PARTY_NOTICES.md` | no | Copied at build time. |

The two cards, in this order, with `typeset` the default:

- **Typeset a page** (`?demo=typeset`): "The sample Markdown page, laid out with
  the embedded fonts. Scroll it, cycle the looks, swap the code face, go dark."
- **Font specimen** (`?demo=specimen`): "One row per look: Literata, Newsreader,
  IBM Plex Sans and Source Serif 4 for body text, Iosevka and IBM Plex Mono for
  code, with JetBrains Mono as the override."

Footer as ntqrcode's: powered by booba and ghostty-web, links to `LICENSE` and
`THIRD_PARTY_NOTICES.md`, made by Nimble Terminal. The "Example source" link
points at `https://github.com/NimbleTerminal/nttypeset/tree/main/examples/<demo>`.

While the wasm downloads the status line says the download is about 6 MB, so
the wait is explained. Once running it says the demo runs locally in the
browser, as the sibling sites do.

## The demos

Both live in the examples module and both build natively and for js/wasm.

### Booba wiring: `examples/internal/demorun`

A copy of ntqrcode's `demorun` with the app name `nttypeset`:

- `type Keys struct{ Primary, More []key.Binding }` with `ShortHelp` and
  `FullHelp`.
- `func Binding(shortcut, label string) key.Binding` and a variant that takes
  several shortcuts (`j` and `down` are one action).
- `func Run(model tea.Model, context, summary string, keys Keys) error` wraps
  the model with `bubblehelp.NewModel` and runs it with `booba.Run`.

Each demo replaces `tea.NewProgram(...).Run()` with `demorun.Run`. The
bindings are the single source of the help line and the browser buttons: key
handling uses `key.Matches` on the same bindings, and the terminal help row is
`help.New().ShortHelpView(keys.Primary)` plus Quit. Under booba the model
remains the only key handler; booba only advertises the bindings.

### `typeset` (existing, adapted)

Behavior is unchanged except for the wiring above and the status layout. The
view reserves two rows under the picture (`SetSize(width, height-2)`): a status
row (`title · look[+mono] · light|dark · top N`, then the error if any) and a
help row from the bindings.

Bindings and labels:

| Primary | Keys |
| --- | --- |
| Down | `j`, `down` |
| Up | `k`, `up` |
| Page down | `space`, `pgdown` |
| Page up | `pgup` |
| Top | `g`, `home` |
| End | `G`, `end` |
| Dark / Light | `d` |
| Next look | `f` |
| Mono | `m` |

| More | Keys |
| --- | --- |
| Line numbers | `n` |
| Chop lines | `c` |
| Keep screen & quit | `Q` |
| Quit | `q`, `ctrl+c` |

In the browser there is no FILE argument, so it shows the embedded sample. The
flags still parse; `go.argv` is left empty by the site.

### `specimen` (new): `examples/specimen/main.go`

Shows `nttypeset.Preview` of `nttypeset.FontLooks()` in a Picture, re-rendered
at the picture's raster geometry.

- Body sample: `Sphinx of black quartz, judge my vow. Café, naïve, résumé — 0123456789`.
  Code sample: `func (e *Engine) Layout(doc *Document, width int) (*Layout, error)`.
  Both fixed; they exercise accents, the em dash from the general-punctuation
  range, and digits.
- Size: `Preview` accepts 160 to 1200 in each dimension. Render at the
  picture's device pixels clamped to that range. When the clamped size equals
  the geometry's pixels, use `SetImageFor(img, g)`; otherwise `SetImage(img)`
  and let ntpicture fit it.
- Flags: `-dark`, `-mono` (empty or `jetbrains`), `-o PNG`, `-width`
  (default 800), `-height` (default 600) for the PNG path.
- Bindings: Primary `Dark / Light` (`d`), `Mono` (`m`); More `Keep screen &
  quit` (`Q`), `Quit` (`q`, `ctrl+c`). One row under the picture holds status
  and help: `specimen · light|dark[ · +jetbrains]` then the help.
- Tests: the PNG path at 800×600 light and dark each paints ink (reuse the
  pixel-count approach from `examples/typeset/main_test.go`); `-mono jetbrains`
  changes pixels against the default.

### Browser single thread

Layout and raster run synchronously inside `Update`. The sample page is small
and `Ensure` only lays out as far as the view needs, so no yielding helper is
required. Nothing in either demo loops continuously.

## Build and check

Taskfile additions, with `GOWORK` set per task as ntpicture does:

| Task | Does |
| --- | --- |
| `build-wasm` | `GOWORK={{.ROOT_DIR}}/wasm.work GOOS=js GOARCH=wasm CGO_ENABLED=0 go -C examples build -mod=readonly -trimpath -ldflags='-s -w' -o ../web/demos/<demo>.wasm ./<demo>` for `typeset` and `specimen`. |
| `build-wasm-site` | `build-wasm`; `go -C examples tool booba-assets --no-index ../web`; copy `LICENSE` and `THIRD_PARTY_NOTICES.md` into `web/`; then `check-site`. |
| `check-site` | `node --check web/site.js`; `node scripts/check-site.mjs`; `node scripts/smoke-demo.mjs typeset`; `node scripts/smoke-demo.mjs specimen`; `node scripts/smoke-demo.mjs typeset keep`. |
| `serve-wasm-site` | `build-wasm-site`, then serve so the site is at `http://localhost:8000/nttypeset/`, matching the Pages prefix, as ntqrcode does. |
| `test-wasm` | The examples module's tests under js/wasm with `go_js_wasm_exec`, through `wasm.work`. |
| `vulncheck-wasm` | govulncheck for js/wasm through `wasm.work`, as ntpicture's. |
| `build-examples`, `run-specimen` | Add `bin/nttypeset-specimen` and `task run-specimen -- -dark`. |
| `ci` | Adds `build-wasm-site` (which runs `check-site`), `test-wasm` and `vulncheck-wasm` after the existing steps. `cross-wasm` stays as the library-only compile check. |
| `clean` | Also removes the generated `web/` paths. |

`scripts/check-site.mjs`: ntqrcode's, with the demo list read from the cards
in `index.html` and asserted equal to `['typeset', 'specimen']`, and the
`examples/<demo>` directories checked to exist.

`scripts/smoke-demo.mjs <demo> [keep]`: ntpicture's shape. For each demo it
waits for the booba bridge, resizes to 100×40, waits for a view containing
`quit`, sends the OSC 7501 query and asserts a snapshot with an enabled
`Quit` action. For `typeset` it also asserts `Down`, `Next look` and `Dark /
Light` are enabled, sends `j` and asserts output follows. For `specimen` it
sends `d` and asserts output follows. With `keep` it exits with `Q`; otherwise
with the quit key. Exit code 0 and `bubbletea_exited` true.

`.gitignore` adds `*.wasm`, `/web/demos/`, `/web/booba/`, `/web/ghostty-web/`,
`/web/wasm_exec.js`, `/web/LICENSE`, `/web/THIRD_PARTY_NOTICES.md`.

### Notices

`internal/tools/notices` grows to credit what the site ships. It keeps the
platform union for the library and adds two more `go list` runs: the examples
module natively (`GOWORK=off`, dir `examples`) and under js/wasm with
`GOWORK=<abs wasm.work>`. Both runs exclude the main module and `nimbleterminal.dev/nttypeset` itself, which the examples module lists through its local replace (ntfonts and
ntpicture are credited like any dependency). After the module sections it
appends `Browser runtime: ghostty-web`, `ghostty` and `pako` from
`web/licenses/`, as ntpicture's does. The header sentence becomes: the library,
the demos and the browser site link the dependencies below.

`NOTICE.md` adds that the site includes booba and ghostty-web runtime assets,
where the browser licenses came from, and that `examples/typeset/landscape.png`
is an original illustration drawn for gloss (it is a 5 KB flat-shaded scene,
not a photograph) and carries no third-party notice.

### CI and deploy

- `.github/workflows/ci.yml`: add `actions/setup-node` with Node 24 and
  `wasm.work.sum` to the Go cache paths. Both runners keep running `task ci`.
- `.github/workflows/pages.yml`: ntpicture's, copied as is:
  on push to `main` and manual dispatch, build job runs `task ci` then uploads
  `web/`; deploy job publishes with `actions/deploy-pages`.
- Enabling Pages on the repository (source: GitHub Actions) is a one-time
  repository setting done after the branch merges. It is not part of the
  code change.

## Size

Measured on this machine with the fork and `-trimpath -ldflags='-s -w'`:
`typeset.wasm` is 22.8 MB raw and 5.8 MB gzipped. The sibling demos are 9 to
10 MB raw. The difference is chroma's full lexer registry, go-text's shaper and
tables, and 1.2 MB of fonts. This design accepts the size: Pages serves
compressed, the status line explains the wait, and trimming chroma would
change which files the native demo can highlight. The specimen build will be
smaller since it does not import chroma's lexers or goldmark.

## Documentation

- `README.md`: a "Browser demos" paragraph with the site URL under the Demo
  section.
- `DEVELOP.md`: replace "There is no browser build" with the site build:
  `wasm.work` and why, the tasks, `serve-wasm-site`, that generated `web/`
  paths are never committed, that the notices now cover the demos and the
  browser runtime, and a manual acceptance check: before announcing the site,
  open both demos in a real browser and record what was seen; the Node smoke
  is not a visual check.
- `AGENTS.md`: commands (`run-specimen`, `build-wasm-site`, `serve-wasm-site`,
  `test-wasm`), module layout (the browser build and `wasm.work`), file map
  rows for `examples/specimen`, `examples/internal/demorun`, `web/`,
  `scripts/`, `wasm.work`, and an invariant: generated site assets are never
  committed.
- `CHANGELOG.md` under Unreleased.

## Branch and delivery

Work on a feature branch in a worktree; the Pages workflow only deploys from
`main`, so the site appears after the merge and the repository setting. The
branch is done when `task ci` passes locally and on both GitHub runners, and
both demos have been opened in a browser from `task serve-wasm-site` and
visually checked.

## Left to the builder

- Exact HTML and CSS beyond the cards, titles and links named above.
- The smoke script's timeouts and polling, following ntpicture's.
- Test names and the arrangement of the examples' test files.
- The wording of help labels in the terminal row where the full label does
  not fit; browser buttons keep the full label.
