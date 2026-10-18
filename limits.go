package nttypeset

import (
	"errors"
	"time"
)

// Limits bound a layout. The zero value means the defaults; a value may lower
// a limit, never raise it (ntpicture's convention).
type Limits struct {
	Blocks     int           // 32,768 per document
	Runs       int           // 32,768 style runs per block
	Runes      int           // 16,384 per paragraph
	Glyphs     int           // 262,144 per layout
	Operations int           // 32,768 display-list entries
	Height     int           // 11,250,000 layout pixels (1125 × 10,000 pages)
	Pixels     int           // 32 Mi raster pixels
	Deadline   time.Duration // 15 s of layout time per layout, cumulative
	ShapeCache int           // 32 MiB
	GlyphCache int           // 32 MiB
}

// defaultLimits are the ceilings; the zero Limits means these.
var defaultLimits = Limits{
	Blocks:     32768,
	Runs:       32768,
	Runes:      16384,
	Glyphs:     262144,
	Operations: 32768,
	Height:     11250000,
	Pixels:     32 << 20,
	Deadline:   15 * time.Second,
	ShapeCache: 32 << 20,
	GlyphCache: 32 << 20,
}

// withDefaults returns a copy with every zero or negative field set to its default and
// every nonzero field clamped to no more than its default.
func (l Limits) withDefaults() Limits {
	lower := func(v, def int) int {
		if v <= 0 {
			return def
		}
		return min(v, def)
	}
	d := defaultLimits
	l.Blocks = lower(l.Blocks, d.Blocks)
	l.Runs = lower(l.Runs, d.Runs)
	l.Runes = lower(l.Runes, d.Runes)
	l.Glyphs = lower(l.Glyphs, d.Glyphs)
	l.Operations = lower(l.Operations, d.Operations)
	l.Height = lower(l.Height, d.Height)
	l.Pixels = lower(l.Pixels, d.Pixels)
	l.ShapeCache = lower(l.ShapeCache, d.ShapeCache)
	l.GlyphCache = lower(l.GlyphCache, d.GlyphCache)
	if l.Deadline <= 0 {
		l.Deadline = d.Deadline
	} else {
		l.Deadline = min(l.Deadline, d.Deadline)
	}
	return l
}

// ErrLimit is matched by errors.Is for every *LimitError.
var ErrLimit = errors.New("nttypeset: limit exceeded")

// A LimitError says which limit was exceeded and keeps today's message text
// ("typeset layout exceeds 15 seconds", ...). errors.Is(err, ErrLimit) is true.
type LimitError struct {
	What  string // the message
	Limit int    // the limit that was exceeded
}

// Error returns the message.
func (e *LimitError) Error() string { return e.What }

// Is reports whether target is ErrLimit.
func (e *LimitError) Is(target error) bool { return target == ErrLimit }
