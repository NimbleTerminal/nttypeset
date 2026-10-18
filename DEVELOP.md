# Development

Use Go 1.27.2 for development and CI (the module minimum is Go 1.26). This is a
single module with no examples module and no browser build. Installed QA tools
live in `.task/bin/` and generated output in `bin/` and `dist/`; all are
ignored.

```sh
task ci                  # same gate as GitHub Actions; includes network scans
task test                # tests
task test-race           # tests under the race detector
task lint                # golangci-lint v2.14.0, installed into .task/bin
task cross-windows       # check Windows compilation
task cross-wasm          # check js/wasm compilation
task notices             # regenerate THIRD_PARTY_NOTICES.md
task clean               # remove generated artifacts and installed QA tools
```

Install [Go Task](https://taskfile.dev) and a C compiler for the race detector.
`task --list-all` lists maintenance tasks. CI runs on Linux and macOS. Tasks
explicitly use `GOWORK=off`.

`task vulncheck` installs govulncheck v1.8.0 into `.task/bin` and scans the
module. It requires network access to the Go vulnerability database; failures
stop CI.

## Tests

`task test` runs every package, including `raster` and `goldmark`; tests use
`internal/testfonts` (the Go fonts), never ntfonts. Run one test with
`GOWORK=off go test -mod=readonly -run 'TestName' .`. `task ci` is the full gate:
format, tidy, verify, race tests, vet, lint, Windows and wasm builds, the notices
check and vulncheck.

Pixel parity with gloss is checked in gloss, not here: gloss's
`scripts/typeset-baseline.sh` exports typeset PNGs across fonts and themes
through this module and compares their hashes with a recorded baseline. A change to layout, shaping or raster
output needs that check before release.

## Dependencies

The core depends on go-text typesetting v0.3.5 and render v0.2.1,
chroma v2.27.0, and x/image v0.47.0. goldmark appears only in the `goldmark`
subpackage. It never depends on ntpicture, Bubble Tea, ntfonts, or gloss. Programs
that want the embedded faces import ntfonts themselves and pass `ntfonts.Embedded`
to `WithFonts`.

Run `task notices` after dependency updates, then review and commit
THIRD_PARTY_NOTICES.md. `task notices-check` enforces that generated text in CI.

## Releases

This is a Go library: CI runs for branches, pull requests, and tags. Release
publication is manual, and there is no binary packaging.

Before the first release, configure the `nimbleterminal.dev/nttypeset`
`go-import` metadata to resolve to `github.com/NimbleTerminal/nttypeset`, and
confirm module discovery from a fresh consumer without workspace or local
replacements. Update CHANGELOG.md, run `task ci`, and review dependency
licenses. Then commit the release preparation, tag the intended version, wait
for tag CI to pass, and publish the GitHub release with its changelog.
