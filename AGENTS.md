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
- The core does no Markdown parsing. A `goldmark` adapter subpackage will take a
  parsed tree.
- It carries no fonts. Faces come from a `FaceSource`; the sibling module
  `nimbleterminal.dev/ntfonts` will supply embedded OFL subsets.

Do not document API that has not landed: the `goldmark` adapter and the
`ntfonts` module do not exist yet.

Longer-form docs: `README.md` (overview), `DEVELOP.md` (tooling and releases),
`CHANGELOG.md`.

## Commands

All tasks run through [Go Task](https://taskfile.dev) (`Taskfile.yml`). Tasks
set `GOWORK=off`.

```sh
task ci                  # full gate, same as GitHub Actions (vulncheck needs network)
task test                # tests
task test-race           # race detector (needs a C compiler)
task lint                # golangci-lint v2 (installed into .task/bin)
task vet fmt fmt-check go-tidy-check go-verify
task build cross-windows cross-wasm
task notices             # regenerate THIRD_PARTY_NOTICES.md after dependency changes
task clean
```

Single test:

```sh
GOWORK=off go test -mod=readonly -run 'TestName' .
```

## Module layout

One Go module, `nimbleterminal.dev/nttypeset`. There are no examples module and
no browser build. Never add local sibling `replace` directives.

Planned dependencies, when the code lands: go-text typesetting v0.3.5 and
render v0.2.1, chroma v2.27.0, and x/image v0.47.0. goldmark is allowed only in
the `goldmark` subpackage. The module never imports ntpicture, Bubble Tea,
ntfonts, or gloss.

Generated output is ignored: `bin/`, `dist/`, `.task/`.

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
| `layout.go` | `Layout`, incremental layout, `text`, blocks and tables, and the `guard` that recovers panics. |
| `lines.go` | `Lines`: source text as a line document. |
| `highlight.go` | `Highlight`: chroma tokens as styled spans. |
| `view.go` | `View`: the region of the page and the raster it maps to. |
| `palette.go` | `Palette`, `LightPalette`, `DarkPalette`. |
| `raster.go` | `Layout.Raster`, `Links`, `Images`, `RasterOptions`; draws the display list. |
| `select.go` | `Place`, `Selection`, `Selected`, `Layout.Select`: logical selection and carets. |
| `preview.go` | `Preview`: the font chooser picture. |
| `export_test.go` | Test-only access to unexported engine methods. |
| `*_test.go` | Tests for the file of the same name; `example_test.go` holds `ExampleLines`. |

Other packages:

| Path | Role |
| --- | --- |
| `raster/raster.go` | `raster.Rasterizer`: draws go-text runs into RGBA images through a bounded glyph-mask cache. |
| `internal/testfonts` | A `FaceSource` over the Go fonts, for tests and the example. |
| `internal/tools/notices` | Generates `THIRD_PARTY_NOTICES.md` (`task notices`). |
| `testdata/` | Fixtures for tests. |

## Invariants

Breaking these breaks consumers, even if tests still pass.

- **No panic crosses the public boundary.** Every exported method recovers into
  an error. Context errors pass through unwrapped. Limit errors are
  `*LimitError` and match `ErrLimit`.
- **Limits can be lowered, never raised.** Hard limits: 32,768 blocks; 16,384
  runes per paragraph; 262,144 glyphs; 32,768 display-list entries; 32 Mi raster
  pixels; 15 s layout time.
- **Caches are bounded.** The shaped-run cache and the glyph-mask cache are each
  capped at 32 MiB.
- **The display list is private.** It is not exported API.
- **The core imports only go-text, chroma and x/image** (plus the standard
  library). goldmark belongs to the `goldmark` subpackage alone.
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
