# Development

Use Go 1.27.2 for development and CI (the module minimum is Go 1.26). The
library is one module; `examples/` is a second module holding the demos, with a
`replace` to the parent directory. The demos also build for the browser; see
[Browser site](#browser-site). Installed QA tools live in `.task/bin/` and
generated output in `bin/`, `dist/` and the site's build output under `web/`;
all are ignored.

```sh
task ci                  # same gate as GitHub Actions; includes network scans
task test                # tests
task test-race           # tests under the race detector
task lint                # golangci-lint v2.14.0, installed into .task/bin
task cross-windows       # check Windows compilation
task cross-wasm          # check js/wasm compilation
task notices             # regenerate THIRD_PARTY_NOTICES.md
task run-typeset         # the demo: task run-typeset -- -dark -font technical FILE
task run-specimen        # the font specimen: task run-specimen -- -dark
task run-hackernews      # the live HN reader; -o hackernews.png exports a PNG
task build-examples      # bin/nttypeset-{typeset,specimen,hackernews}
task build-wasm          # the browser demos in web/demos/
task build-wasm-site     # the whole browser site in web/, then check-site
task check-site          # Node checks of web/ and all demos
task serve-wasm-site     # serve web/ at http://localhost:8000/nttypeset/
task test-wasm           # examples tests under js/wasm with Node
task vulncheck-wasm      # govulncheck on the browser build
task clean               # remove generated artifacts and installed QA tools
```

Install [Go Task](https://taskfile.dev) and a C compiler for the race detector.
The site checks need Node 24. `task --list-all` lists maintenance tasks. CI runs
on Linux and macOS. Tasks explicitly use `GOWORK=off`, except `build-wasm`,
`test-wasm` and `vulncheck-wasm`, which point `GOWORK` at `wasm.work`; the site
tasks that call them run under `GOWORK=off`.

`task vulncheck` installs govulncheck v1.8.0 into `.task/bin` and scans the
module. It requires network access to the Go vulnerability database; failures
stop CI.

## Tests

`task test` runs every package, including `raster` and `goldmark`, and the
examples module; the library's tests use `internal/testfonts` (the Go fonts),
never ntfonts, while the demo's tests render with `ntfonts.Embedded`. Run one test with
`GOWORK=off go test -mod=readonly -run 'TestName' .`. `task ci` is the full gate:
format, tidy, verify, race tests, vet, lint, Windows and wasm builds, the notices
check, vulncheck, then the browser site build and its Node checks, the js/wasm
tests and the browser vulnerability scan.

Pixel parity with gloss is checked in gloss, not here: gloss's
`scripts/typeset-baseline.sh` exports typeset PNGs across fonts and themes
through this module and compares their hashes with a recorded baseline. A change to layout, shaping or raster
output needs that check before release.

## Dependencies

The core depends on go-text typesetting v0.3.5, NimbleMarkets/oksvg v0.1.3,
rasterx, chroma v2.27.0, and x/image v0.47.0 (with shaped-run outline, bitmap,
and SVG glyph rendering in `raster` adapted from `go-text/render` v0.2.1).
goldmark appears only in the `goldmark` subpackage. It never depends on
ntpicture, Bubble Tea, ntfonts, or gloss. Programs that want the embedded faces
import ntfonts themselves and pass `ntfonts.Embedded` to `WithFonts`. The
examples module pins ntfonts, ntpicture, Bubble Tea, bubbles and go-booba. The
notices cover the library, adapted code, the demos and the browser runtime
(booba, ghostty-web and pako), since the site distributes them.

Glyph and SVG rendering imports `github.com/NimbleMarkets/oksvg` v0.1.3
directly and pins the `rasterx` fork (`github.com/srwiley/rasterx =>
github.com/neomantra/rasterx v0.1.0`) through matching versioned replacements
in both `go.mod` files; see the
[consumer setup](README.md#svg-rendering-dependencies). Keep both `go.mod`
files in sync and validate native and browser builds with `task ci` after
updating them.

Run `task notices` after dependency updates, then review and commit
THIRD_PARTY_NOTICES.md. The generator unions the packages linked on linux,
darwin and windows (amd64 and arm64) and js/wasm, so its output is the same on
every machine. `task notices-check` enforces that generated text in CI.

## Browser site

All three demos run in the browser at `https://nimbleterminal.github.io/nttypeset/`.
They are compiled to js/wasm and shown through go-booba and ghostty-web. The
site is the `web/` directory: `index.html`, `site.js`, `site.css` and
`licenses/` are source, and everything else the build puts there is output.

`wasm.work` is a Go workspace used only for js/wasm builds. It swaps upstream
Bubble Tea for a browser fork, because upstream does not run in a browser.
Native builds and `GOWORK=off` tasks never see it. `go work sync` bumps the root
`go.mod` to the workspace's Go version; revert that so the library minimum stays
`go 1.26.0`.

```sh
task build-wasm          # web/demos/{typeset,specimen,hackernews}.wasm
task build-wasm-site     # build-wasm, export the booba runtime, copy LICENSE and notices, check-site
task check-site          # script syntax, required assets, and each demo under Node
task serve-wasm-site     # builds the site, serves it at http://localhost:8000/nttypeset/
# Add SERVE_PORT=8001 when port 8000 is in use.
task test-wasm           # the examples module's tests under js/wasm with Node
task vulncheck-wasm      # govulncheck on the browser build (requires network)
```

`web/demos`, `web/booba`, `web/ghostty-web`, `web/wasm_exec.js`, `web/LICENSE`
and `web/THIRD_PARTY_NOTICES.md` are ignored and never committed.
`task clean` removes them.

The wasm files are large: typeset.wasm is 23.2 MB raw (about 5.8 MB gzipped) and
specimen.wasm is 20.7 MB raw. The Hacker News demo also bundles the HTTP client.
That size is accepted.

The demos publish their keys to booba as actions through
`examples/internal/demorun`. Booba caps its status message at 2048 bytes, so
`demorun` sets `Preferred` only on actions that take several keys and `FullHelp`
lists only the More group. The expanded browser action bar therefore shows More
before Primary, while the terminal help row shows Primary then More.
`demorun.Check` and the `...KeysFitBooba` tests guard the cap. The terminal help
row always ends with Quit and truncates the rest with an ellipsis in narrow
terminals; the booba action bar always lists every action. The snapshot carries
only each action's first key, so alias keys such as the arrows and ctrl+c are
not listed on the browser's buttons, though the demo still answers them. The
Node smoke script resizes the terminal to 100 columns and waits for Quit.

`.github/workflows/pages.yml` deploys `web/` to GitHub Pages on each push to
`main`, on release tag pushes matching `v*`, and on manual runs. It runs
`task ci` on the triggering revision first and uploads the site only if every
check passes. Other branch pushes do not trigger deployment. The `github-pages`
environment must allow deployments from `main` and release tags. After the first
merge the maintainer enables Pages on the repository once, with the source set
to GitHub Actions.

The Node smoke script runs the wasm and checks its output; it is not a visual check.
Hacker News uses mocked Fetch responses in the smoke check, including an API
failure and retry, so CI does not depend on the live service. Browser navigation
uses the page host function `nttypesetOpenURL`; the native opener remains separate.
The page retains a real link as a fallback for popup blockers. Initial loading
and refresh run asynchronously, so API errors can be retried with `r`.
Before announcing a site change, run `task serve-wasm-site`, open the demos in
a browser, scroll, cycle the looks, toggle dark and mono, drag over the page
and press `y` and check the clipboard, scroll with the wheel, and record what you saw.

## Releases

This is a Go library: CI runs for branches, pull requests, and tags. Release
publication is manual, and there is no binary packaging.

The `nimbleterminal.dev/nttypeset` `go-import` metadata already resolves to
`github.com/NimbleTerminal/nttypeset`. To release, update CHANGELOG.md, run
`task ci`, review dependency licenses and commit. Then tag the intended
version, push the tag, and verify the version on pkg.go.dev once tag CI passes.

The section model and adapter contract are documented in [docs/sections.md](docs/sections.md).
`go run ./internal/tools/sections` regenerates the illustrative before/after PNGs;
they are demonstration artifacts, never pixel-test reference data.
