package nttypeset

import (
	"fmt"
	"image/color"
	"strings"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2"
	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"
)

// shaped is one paragraph's shaped runs and the per-rune attributes the
// painter needs. It is cached and shared; callers must not modify it.
type shaped struct {
	tokens []chroma.TokenType
	text   []rune
	runs   []shaping.Output
	styles []Style
	inks   []color.RGBA
	roles  []Role
	size   int
}

// shape shapes spans at size layout pixels, reusing the cache. It panics with
// a *LimitError for a paragraph over Limits.Runes, and with the context's
// error once the engine's context is done; the Layout boundary recovers both.
func (e *Engine) shape(spans []Span, size int) *shaped {
	e.check()
	count := 0
	for _, sp := range spans {
		count += utf8.RuneCountInString(sp.Text)
	}
	if count > e.limits.Runes {
		panic(&LimitError{
			What:  fmt.Sprintf("typeset paragraph exceeds %s runes", groupDigits(e.limits.Runes)),
			Limit: e.limits.Runes,
		})
	}

	var key strings.Builder
	fmt.Fprintf(&key, "%d/", size)
	for _, s := range spans {
		fmt.Fprintf(&key, "%d:%d:%v:%d:%d:%s", s.Token, s.Style, s.Ink, s.Role, len(s.Text), s.Text)
	}
	k := key.String()
	if s := e.cache[k]; s != nil {
		return s
	}
	s := &shaped{size: size}
	for _, sp := range spans {
		r := []rune(sp.Text)
		s.text = append(s.text, r...)
		for range r {
			s.styles = append(s.styles, sp.Style)
			s.tokens = append(s.tokens, sp.Token)
			s.inks = append(s.inks, sp.Ink)
			s.roles = append(s.roles, sp.Role)
		}
	}
	offset := 0
	for _, sp := range spans {
		end := offset + utf8.RuneCountInString(sp.Text)
		if end == offset {
			continue
		}
		in := shaping.Input{
			Text: s.text, RunStart: offset, RunEnd: end,
			Direction: di.DirectionLTR, Face: e.faces.faces[sp.Style&7],
			FontFeatures: noLigatures, Size: fixed.I(size),
		}
		for _, run := range e.segmenter.Split(in, fallbackMap{e.faces, sp.Style}) {
			s.runs = append(s.runs, e.shaper.Shape(run))
		}
		offset = end
	}
	// The cache is bounded by an estimate of its size. Eviction is simple: when
	// the next entry would not fit, drop everything.
	bytes := len(s.text)*28 + len(k)
	for _, r := range s.runs {
		bytes += len(r.Glyphs) * 128
	}
	if e.cacheBytes+bytes > e.limits.ShapeCache {
		e.cache = map[string]*shaped{}
		e.cacheBytes = 0
	}
	if bytes <= e.limits.ShapeCache {
		e.cache[k] = s
		e.cacheBytes += bytes
	}
	return s
}

// groupDigits formats n with thousands separators: 16384 becomes "16,384".
func groupDigits(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
