# AGENTS.md

Guidance for coding agents working in this repository.

## What this is

`nimbleterminal.dev/nttypeset` is a Go library that lays out a `Document` of
blocks and styled spans and rasters viewports into `*image.RGBA`. It has
incremental layout, a shaping engine with bounded caches, a glyph-mask
rasterizer, logical selection, and link and image hit areas. It was extracted
from gloss's `--typeset` view. Develop with Go 1.27.2; the module minimum is
Go 1.26.

What it is not:

- It does no terminal output. ntpicture shows the images it produces.
- The core does no Markdown parsing. The `goldmark` adapter subpackage takes a
  parsed tree.
- Document faces come from a `FaceSource`; the separate module
  `nimbleterminal.dev/ntfonts` supplies embedded OFL subsets as
  `ntfonts.Embedded`. The oksvg fork embeds Go font fallbacks for text inside
  SVGs only; these do not supply the document's faces.

Longer-form docs: `README.md` (overview), `DEVELOP.md` (tooling and releases),
`CHANGELOG.md`. Pixel parity with gloss is checked in gloss.

## Commands

All tasks run through [Go Task](https://taskfile.dev) (`Taskfile.yml`). Tasks
set `GOWORK=off`, except `build-wasm`, `test-wasm` and `vulncheck-wasm`, which
use `wasm.work`; the site tasks that call them run under `GOWORK=off`. The site
checks need Node 24.

```sh
task ci                  # full gate, same as GitHub Actions (vulncheck needs network)
task test                # tests
task test-race           # race detector (needs a C compiler)
task lint                # golangci-lint v2 (installed into .task/bin)
task vet fmt fmt-check go-tidy-check go-verify
task build cross-windows cross-wasm
task notices             # regenerate THIRD_PARTY_NOTICES.md after dependency changes
task run-typeset         # the demo: task run-typeset -- -dark FILE
task run-specimen        # the font specimen: task run-specimen -- -dark
task run-hackernews      # the live HN reader; -o hackernews.png exports a PNG
task build-examples      # bin/nttypeset-{typeset,specimen,hackernews}
task build-wasm-site     # the browser site in web/, then check-site
task check-site          # Node checks of web/ and all demos
task serve-wasm-site     # serve web/ at http://localhost:8000/nttypeset/
task test-wasm           # examples tests under js/wasm with Node
task clean
```

Single test:

```sh
GOWORK=off go test -mod=readonly -run 'TestName' .
```

## Module layout

Two Go modules: the library, `nimbleterminal.dev/nttypeset`, and
`examples/` (`nimbleterminal.dev/nttypeset/examples`), which holds the demos and
replaces nttypeset with the parent directory so it tracks the working tree.
The browser build uses `wasm.work`, a workspace that swaps in a browser Bubble
Tea fork for js/wasm builds only; native builds use the upstream Bubble Tea.
Never add sibling `replace` directives: ntfonts and ntpicture are pinned by
version.

Glyph outline and SVG glyph rendering in `raster` (adapted from
`go-text/render` v0.2.1) imports `github.com/NimbleMarkets/oksvg v0.1.4`
directly and uses a versioned replacement for `rasterx`:
`github.com/srwiley/rasterx => github.com/neomantra/rasterx v0.1.0`.
Keep this replacement identical in root and examples `go.mod`; dependency
replacements are not inherited. The browser workspace uses this module
replacement too. Use published versions, never sibling paths. Consumers that
want the `rasterx` fork add the replacement to their own main module; keep the
README instructions current when changing pins.

Dependencies: go-text typesetting v0.3.5, NimbleMarkets/oksvg v0.1.4, rasterx,
chroma v2.27.0, and x/image v0.47.0. goldmark v1.8.7 and goldmark-emoji v1.0.6
are allowed only in the `goldmark` subpackage. The library never imports
ntpicture, Bubble Tea, ntfonts, or gloss; the examples module imports all but
gloss, and adds booba (go-booba) and bubbles for the browser build.

Generated output is ignored: `bin/`, `dist/`, `.task/`, and the site's build
output under `web/`.

## File map

Root package `nttypeset`:

| File | Role |
| --- | --- |
| `doc.go` | Package comment. |
| `document.go` | The model: `Document`, `Block`, `Span`, `Image`, `Style`, `Role`, `Kind`, `Bounds`, `Margin`. |
| `limits.go` | `Limits`, `LimitError`, `ErrLimit` and the default limits. |
| `fonts.go` | `FontLook`, `ResolveFont`, `FontKey`, `FaceSource`, face loading and glyph fallback. |
| `engine.go` | `Engine`, `NewEngine` and the `Option`s; cancellation through `enter` and `check`. |
| `shape.go` | The shaping pipeline and the bounded shaped-run cache. |
| `layout.go` | `Layout`, incremental layout, `text`, blocks and tables, `Bounds`, and the `guard` that recovers panics. |
| `lines.go` | `Lines`: source text as a line document. |
| `highlight.go` | `Highlight`: chroma tokens as styled spans. |
| `view.go` | `View`: the region of the page and the raster it maps to; `Project` and `Unproject`. |
| `palette.go` | `Palette` (its `Mark` tint, else Selection blended with Background), `LightPalette`, `DarkPalette`. |
| `raster.go` | `Layout.Raster`, `Links`, `Images`, `RasterOptions` with its `Marks` (`Mark`, `Link.Mark`); draws the display list. |
| `select.go` | `Place`, `Selection`, `Selected`, `Layout.Select`: logical selection and carets; `Hit` and `Layout.TextAt`, the text under a point. |
| `preview.go` | `Preview`: the font chooser picture. |
| `export_test.go` | Test-only access to unexported engine methods. |
| `*_test.go` | Tests for the file of the same name; `example_test.go` holds `ExampleLines`. |

Other packages:

| Path | Role |
| --- | --- |
| `goldmark/goldmark.go` | `goldmark.Build`, `goldmark.Options`: a parsed goldmark tree as a `Document`. The only package that imports goldmark. |
| `raster/raster.go` | `raster.Rasterizer`: draws go-text runs into RGBA images through a bounded glyph-mask cache. |
| `internal/testfonts` | A `FaceSource` over the Go fonts, for tests and the example. |
| `internal/tools/notices` | Generates `THIRD_PARTY_NOTICES.md` (`task notices`). |
| `examples/typeset` | The demo (separate module): a Markdown or text file in the terminal through ntpicture, with mouse selection and `y` to copy, or as a PNG with `-o`. |
| `examples/specimen` | The font specimen: one row per look from `Preview`, in the terminal or as a PNG with `-o`. |
| `examples/hackernews` | A live Hacker News front-page reader in the HN palette, with story and comment links, in the terminal or as a PNG with `-o`; also available in the browser site. |
| `examples/internal/demorun` | Publishes a demo's keys as booba actions and runs it; `demorun.Check` and the `...KeysFitBooba` tests guard booba's 2048-byte status-message cap. |
| `examples/internal/pixels` | The demos' shared test helper: counts pixels that differ from a colour. |
| `web/` | The Pages site: `index.html`, `site.js`, `site.css` and `licenses/` are source; `demos/`, `booba/`, `ghostty-web/` and the copied files are build output. |
| `scripts/` | Node scripts: `check-site.mjs` checks the built site; `smoke-demo.mjs` runs a demo's wasm under Node. |
| `wasm.work` | The Go workspace for js/wasm builds; swaps in the browser Bubble Tea fork. |
| `testdata/` | Fixtures for tests. |

## Invariants

Breaking these breaks consumers, even if tests still pass.

- **No panic crosses the public boundary.** Every exported method recovers into
  an error. Context errors pass through unwrapped. Limit errors are
  `*LimitError` and match `ErrLimit`. Font parsing recovers go-text's panics on
  corrupt bytes into an error.
- **Only the built-in looks.** A `FaceSource` serves the built-in family names;
  there is no custom-look API yet.
- **A font change invalidates layouts.** After `SetFonts` changes the look, a
  `Layout` made before it returns an error from every method that lays out or
  rasters.
- **Limits can be lowered, never raised.** Hard limits: 32,768 blocks; 16,384
  runes per paragraph; 262,144 glyphs; 32,768 display-list entries; 32 Mi raster
  pixels; 4,096 raster pixels per em (`Limits.Em`); 15 s layout time. A
  `Block.Size` over 1,024, or negative, is a `*LimitError` (a constant, not a
  `Limits` field). `LimitError.Limit` is an `int64` so the deadline fits in
  nanoseconds on 32-bit targets.
- **Caches are bounded.** The shaped-run cache and the glyph-mask cache are each
  capped at 32 MiB.
- **Generated site assets are never committed.** `web/demos`, `web/booba`,
  `web/ghostty-web` and the copied files are build output.
- **The display list is private.** It is not exported API.
- **The core imports only go-text/typesetting, NimbleMarkets/oksvg, rasterx,
  chroma and x/image** (plus the standard library). goldmark belongs to the
  `goldmark` subpackage alone.
- **No concurrent use.** An `Engine` or `Layout` is not safe for concurrent use;
  the caller serializes access.

## Tests and reporting

- Tests that compare rendered pixels use independent reference data. Never
  regenerate a golden from nttypeset's own output to make a test pass.
- Automated tests do not establish visual quality. When reporting results,
  distinguish unit coverage from a visual check, and never claim a visual check
  that did not happen.
- Run `task ci` before finishing a change.

## Documentation

- Update `CHANGELOG.md` under an `Unreleased` heading for user-visible changes.
  Keep entries short and direct.
- Keep README, DEVELOP, and CHANGELOG consistent with the code.
- Run `task notices` after dependency updates; `task notices-check` enforces it
  in CI.
