# Notices

Source code is MIT licensed; see LICENSE.

`raster/raster.go` adapts shaped-run outline, bitmap, and SVG glyph rendering
from [`github.com/go-text/render`](https://github.com/go-text/render) v0.2.1
(Copyright 2021 The go-text authors, licensed under `Unlicense OR BSD-3-Clause`).

nttypeset contains no fonts. Faces come from a `FaceSource` supplied by the
caller, typically `nimbleterminal.dev/ntfonts`, which carries the SIL Open Font
License notices for the fonts it embeds.

Go dependency and adapted-code license texts are collected in
THIRD_PARTY_NOTICES.md, generated with `task notices`.

The generated demo site includes the go-booba and ghostty-web runtime assets.
Their license texts are in THIRD_PARTY_NOTICES.md, copied into the site.
`web/licenses/` was copied from go-booba's vendored ghostty-web, Ghostty and
pako sources; review it when updating the pinned booba runtime.

`examples/typeset/landscape.png` is an original flat-shaded illustration drawn
for gloss, Copyright (c) 2026 Neomantra Corp. It carries no third-party notice.
