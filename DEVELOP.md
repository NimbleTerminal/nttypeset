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
task build-examples      # bin/nttypeset-typeset, bin/nttypeset-specimen
task build-wasm          # the browser demos in web/demos/
task build-wasm-site     # the whole browser site in web/, then check-site
task check-site          # Node checks of web/ and both demos
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

The core depends on go-text typesetting v0.3.5 and render v0.2.1,
chroma v2.27.0, and x/image v0.47.0. goldmark appears only in the `goldmark`
subpackage. It never depends on ntpicture, Bubble Tea, ntfonts, or gloss. Programs
that want the embedded faces import ntfonts themselves and pass `ntfonts.Embedded`
to `WithFonts`. The examples module pins ntfonts, ntpicture, Bubble Tea, bubbles
and go-booba. The notices cover the library, the demos and the browser runtime
(booba, ghostty-web and pako), since the site distributes them.

Run `task notices` after dependency updates, then review and commit
THIRD_PARTY_NOTICES.md. The generator unions the packages linked on linux,
darwin and windows (amd64 and arm64) and js/wasm, so its output is the same on
every machine. `task notices-check` enforces that generated text in CI.

## Browser site

Both demos run in the browser at `https://nimbleterminal.github.io/nttypeset/`.
They are compiled to js/wasm and shown through go-booba and ghostty-web. The
site is the `web/` directory: `index.html`, `site.js`, `site.css` and
`licenses/` are source, and everything else the build puts there is output.

`wasm.work` is a Go workspace used only for js/wasm builds. It swaps upstream
Bubble Tea for a browser fork, because upstream does not run in a browser.
Native builds and `GOWORK=off` tasks never see it. `go work sync` bumps the root
`go.mod` to the workspace's Go version; revert that so the library minimum stays
`go 1.26.0`.

```sh
task build-wasm          # web/demos/typeset.wasm and specimen.wasm
task build-wasm-site     # build-wasm, export the booba runtime, copy LICENSE and notices, check-site
task check-site          # script syntax, required assets, and each demo under Node
task serve-wasm-site     # builds the site, serves it at http://localhost:8000/nttypeset/
task test-wasm           # the examples module's tests under js/wasm with Node
task vulncheck-wasm      # govulncheck on the browser build (requires network)
```

`web/demos`, `web/booba`, `web/ghostty-web`, `web/wasm_exec.js`, `web/LICENSE`
and `web/THIRD_PARTY_NOTICES.md` are ignored and never committed.
`task clean` removes them.

The wasm files are large: typeset.wasm is 23.2 MB raw (about 5.8 MB gzipped) and
specimen.wasm is 20.7 MB raw. That size is accepted.

The demos publish their keys to booba as actions through
`examples/internal/demorun`. Booba caps its status message at 2048 bytes, so
`demorun` sets `Preferred` only on actions that take several keys and `FullHelp`
lists only the More group. The expanded browser action bar therefore shows More
before Primary, while the terminal help row shows Primary then More.
`demorun.Check` and the `...KeysFitBooba` tests guard the cap. The terminal help
row always ends with Quit and truncates the rest with an ellipsis in narrow
terminals; the booba action bar always lists every key. The Node smoke script
resizes the terminal to 100 columns and waits for Quit.

`.github/workflows/pages.yml` deploys `web/` to GitHub Pages on each push to
`main` and on manual runs. It runs `task ci` first and uploads the site only if
every check passes. It does not run on other branches. After the first merge the
maintainer enables Pages on the repository once, with the source set to GitHub
Actions.

The Node smoke script runs the wasm and checks its output; it is not a visual check.
Before announcing a site change, run `task serve-wasm-site`, open both demos in
a browser, scroll, cycle the looks, toggle dark and mono, and record what you
saw.

## Releases

This is a Go library: CI runs for branches, pull requests, and tags. Release
publication is manual, and there is no binary packaging.

The `nimbleterminal.dev/nttypeset` `go-import` metadata already resolves to
`github.com/NimbleTerminal/nttypeset`. To release, update CHANGELOG.md, run
`task ci`, review dependency licenses and commit. Then tag the intended
version, push the tag, and verify the version on pkg.go.dev once tag CI passes.
