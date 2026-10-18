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

The repository is a scaffold: only `doc.go` exists so far. Do not document API
that has not landed.

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
