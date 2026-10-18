package nttypeset

import (
	"context"
	"errors"
	"slices"

	"github.com/go-text/typesetting/shaping"
	"nimbleterminal.dev/nttypeset/raster"
)

// options collects what the Option functions set.
type options struct {
	src      FaceSource
	name     string
	override string
	limits   Limits
}

// An Option configures NewEngine.
type Option func(*options)

// WithFonts sets the source of font bytes. It is required.
func WithFonts(src FaceSource) Option { return func(o *options) { o.src = src } }

// WithLook chooses the font look and the mono override. The default is
// "readable" with no override.
func WithLook(name, override string) Option {
	return func(o *options) { o.name, o.override = name, override }
}

// WithLimits lowers the engine's limits. A field that is zero, negative or
// above its default uses the default, so a limit can only be made tighter.
func WithLimits(l Limits) Option { return func(o *options) { o.limits = l } }

// Engine owns faces, the shaper, the shaped-run cache and the glyph-mask
// cache. It is not safe for concurrent use: a caller serializes it.
type Engine struct {
	src        FaceSource
	faces      *faces
	look       FontLook
	fontKey    string
	shaper     shaping.HarfbuzzShaper
	segmenter  shaping.Segmenter
	cache      map[string]*shaped
	cacheBytes int
	rasterizer *raster.Rasterizer
	limits     Limits
	ctx        context.Context
}

// NewEngine builds an engine. WithFonts is required; the look must resolve and
// its faces must load from the source.
func NewEngine(opts ...Option) (*Engine, error) {
	o := options{}
	for _, opt := range opts {
		opt(&o)
	}
	if o.src == nil {
		return nil, errors.New("nttypeset: WithFonts is required")
	}
	limits := o.limits.withDefaults()
	look, err := ResolveFont(o.name, o.override)
	if err != nil {
		return nil, err
	}
	f, err := newFaces(o.src, look)
	if err != nil {
		return nil, err
	}
	return &Engine{
		src:        o.src,
		faces:      f,
		look:       look,
		fontKey:    FontKey(o.name, o.override),
		cache:      map[string]*shaped{},
		rasterizer: raster.New(limits.GlyphCache),
		limits:     limits,
	}, nil
}

// SetFonts switches to another look. It returns false and does nothing when
// the look is the current one. Otherwise it clears the shape cache and the
// glyph-mask cache and returns true. On error the engine is as it was.
func (e *Engine) SetFonts(name, override string) (changed bool, err error) {
	key := FontKey(name, override)
	if e.fontKey == key {
		return false, nil
	}
	look, err := ResolveFont(name, override)
	if err != nil {
		return false, err
	}
	f, err := newFaces(e.src, look)
	if err != nil {
		return false, err
	}
	e.faces = f
	e.look = look
	e.fontKey = key
	e.cache = map[string]*shaped{}
	e.cacheBytes = 0
	e.rasterizer.Clear()
	return true, nil
}

// Look returns the current font look.
func (e *Engine) Look() FontLook { return e.look }

// FontKey returns the current look's cache key, as FontKey would for the name
// and override that selected it.
func (e *Engine) FontKey() string { return e.fontKey }

// Fallbacks returns the fallback families used so far, sorted.
func (e *Engine) Fallbacks() []string {
	names := make([]string, 0, len(e.faces.fallbackUsed))
	for name := range e.faces.fallbackUsed {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// Missing returns up to 32 runes that no face of the current look has a glyph
// for, sorted. A fallback family the font source cannot supply leaves its
// runes here.
func (e *Engine) Missing() []rune {
	runes := make([]rune, 0, len(e.faces.missing))
	for r := range e.faces.missing {
		runes = append(runes, r)
	}
	slices.Sort(runes)
	return runes
}

// GlyphCache reports the glyph-mask cache's size and hit counts.
func (e *Engine) GlyphCache() raster.Stats { return e.rasterizer.Stats() }

// SetGlyphBudget sets the glyph-mask cache budget in bytes, evicting down to it.
func (e *Engine) SetGlyphBudget(bytes int) { e.rasterizer.SetBudget(bytes) }

// enter makes ctx the cancellation of the layout entry point that calls it,
// and returns the function that gives back what was there before, so an entry
// point reached from another one leaves the outer request's context in place.
func (e *Engine) enter(ctx context.Context) func() {
	previous := e.ctx
	e.ctx = ctx
	return func() { e.ctx = previous }
}

// check panics with the context's error once it is done. The panic is
// recovered at the Layout boundary and returned as an error.
func (e *Engine) check() {
	if e.ctx != nil {
		if err := e.ctx.Err(); err != nil {
			panic(err)
		}
	}
}
