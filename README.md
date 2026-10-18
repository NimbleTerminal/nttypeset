# nttypeset — Typesetting for Nimble Terminal

<p>
    <a href="https://pkg.go.dev/nimbleterminal.dev/nttypeset"><img src="https://pkg.go.dev/badge/nimbleterminal.dev/nttypeset.svg" alt="Go Reference"></a>
    <a href="https://github.com/NimbleTerminal/nttypeset/actions/workflows/ci.yml"><img src="https://github.com/NimbleTerminal/nttypeset/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
    <a href="./CODE_OF_CONDUCT.md"><img src="https://img.shields.io/badge/Contributor%20Covenant-2.1-4baaaa.svg" alt="Code Of Conduct"></a>
</p>

nttypeset is a Go library that lays out a `Document` of blocks and styled spans
and rasters viewports of it into `*image.RGBA`. It lays out incrementally, shapes
text with an engine that keeps bounded caches, draws through a glyph-mask
rasterizer, and reports logical selections along with link and image hit areas.
It was extracted from the `--typeset` view of
[Gloss](https://github.com/NimbleMarkets/gloss).

It draws images and nothing else. It writes nothing to a terminal; show its
output with [ntpicture](https://nimbleterminal.dev/ntpicture). The core does not
parse Markdown: a `goldmark` subpackage will take an already parsed tree. It
ships no fonts either: faces come from a `FaceSource`, and the sibling module
`nimbleterminal.dev/ntfonts` will supply embedded OFL subsets.

```sh
go get nimbleterminal.dev/nttypeset
```

A quick start follows once the API lands.

**Status:** extracted from gloss, API settling before v0.1.0.

## License

MIT; see [LICENSE](./LICENSE) and [NOTICE.md](./NOTICE.md).
