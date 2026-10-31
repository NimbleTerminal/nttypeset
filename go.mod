module nimbleterminal.dev/nttypeset

go 1.26.0

// SVG glyph rendering fork; keep this replacement in sync with examples/go.mod.
replace github.com/srwiley/rasterx => github.com/neomantra/rasterx v0.1.0

require (
	github.com/NimbleMarkets/oksvg v0.1.4
	github.com/alecthomas/chroma/v2 v2.27.0
	github.com/go-text/typesetting v0.3.5
	github.com/srwiley/rasterx v0.0.0-20220730225603-2ab79fcdd4ef
	github.com/yuin/goldmark v1.8.7
	github.com/yuin/goldmark-emoji v1.0.6
	golang.org/x/image v0.47.0
)

require (
	github.com/dlclark/regexp2/v2 v2.12.0 // indirect
	golang.org/x/net v0.62.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)
