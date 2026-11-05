package nttypeset

import (
	"context"
	"errors"
	"fmt"
)

// ChunkSource supplies immutable, independently lay-outable AST documents in
// reading order. A chunk must satisfy the ordinary Document/Layout limits.
// Hosts choose semantic boundaries (for example conversation messages), retain
// parser context and stable SourceIDs, and bound their source storage. Chunk
// count and contents must stay fixed for the lifetime of a Chunked layout.
// Implementations must honor cancellation and must not fetch untrusted assets.
type ChunkSource interface {
	Chunks() int
	Chunk(context.Context, int) (*Document, error)
}

// Chunk is one resident document and its layout. Places, rectangles, and block
// indexes are local to this chunk; keep Index alongside a selection or anchor.
// Layout uses the usual incremental Ensure/Raster API and all its hard limits.
// Do not retain Chunks after switching away if bounded residency is required.
type Chunk struct {
	Index      int
	Document   *Document
	Layout     *Layout
	width      int
	options    LayoutOptions
	generation int
}

// Chunked keeps at most two recently used chunk layouts, sharing its Engine's
// bounded font/shaping/raster caches. It never loads or lays out the whole source
// to open its first or last chunk. Like Engine, it is not concurrency-safe.
type Chunked struct {
	engine *Engine
	source ChunkSource
	count  int
	recent [2]*Chunk
}

// NewChunked starts a bounded layout cache for source without loading a chunk.
// A source error or panic is returned as an error; existing layouts stay usable.
func (e *Engine) NewChunked(source ChunkSource) (out *Chunked, err error) {
	defer recoverChunk(&err)
	if source == nil {
		return nil, errors.New("nttypeset: nil chunk source")
	}
	n := source.Chunks()
	if n < 1 {
		return nil, errors.New("nttypeset: chunk source is empty")
	}
	return &Chunked{engine: e, source: source, count: n}, nil
}

// Chunks is the number of independently loadable documents.
func (c *Chunked) Chunks() int { return c.count }

// At loads a chunk on demand and starts its layout at width. It reuses recent
// chunks at unchanged width/options/fonts. It does not rasterize or complete
// layout. A width, option, or font change replaces only the requested layout.
func (c *Chunked) At(ctx context.Context, index, width int, opts LayoutOptions) (out *Chunk, err error) {
	defer recoverChunk(&err)
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if index < 0 || index >= c.count {
		return nil, fmt.Errorf("nttypeset: chunk %d outside 0..%d", index, c.count-1)
	}
	if width < 1 {
		return nil, errors.New("nttypeset: width must be positive")
	}
	var doc *Document
	for i, saved := range c.recent {
		if saved == nil || saved.Index != index {
			continue
		}
		if saved.width == width && saved.options == opts && saved.generation == c.engine.fontGeneration {
			if i == 1 {
				c.recent[0], c.recent[1] = c.recent[1], c.recent[0]
			}
			return saved, nil
		}
		doc = saved.Document
	}
	if doc == nil {
		doc, err = c.source.Chunk(ctx, index)
		if err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	lay, err := c.engine.Layout(doc, width, opts)
	if err != nil {
		return nil, err
	}
	out = &Chunk{Index: index, Document: doc, Layout: lay, width: width, options: opts, generation: c.engine.fontGeneration}
	previous := c.recent[0]
	if previous != nil && previous.Index == index {
		previous = c.recent[1]
	}
	c.recent = [2]*Chunk{out, previous}
	return out, nil
}

func recoverChunk(err *error) {
	if p := recover(); p != nil {
		if e, ok := p.(error); ok {
			*err = e
		} else {
			*err = fmt.Errorf("nttypeset: %v", p)
		}
	}
}
