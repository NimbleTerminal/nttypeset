package nttypeset

import (
	"context"
	"errors"
	"image"
	"image/draw"
	"strings"
)

// Preview draws a font chooser: one sample row per look, stacked in a
// width×height picture. Each row shows the look's name in bold, then body and
// code, each on a single line, with the same shaper and rasterizer as a
// Layout. An empty body or code leaves its line out.
//
// It builds one Engine per look from opts plus WithLook(look.Name, override),
// so opts must include WithFonts, and each look's Name must be a built-in
// look; a look's Body and Code fields are informational and not used. Both
// dimensions must be 160 to 1200, and there must be at least one look. A
// cancelled ctx returns its error; a nil ctx is context.Background().
func Preview(ctx context.Context, looks []FontLook, override, body, code string, width, height int, p Palette, opts ...Option) (*image.RGBA, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(looks) == 0 {
		return nil, errors.New("nttypeset: preview needs at least one look")
	}
	rowHeight := height / len(looks)
	if width < 160 || height < 160 || width > 1200 || height > 1200 || rowHeight < 1 {
		return nil, errors.New("nttypeset: preview size out of range")
	}
	result := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(result, result.Bounds(), image.NewUniform(p.Background), image.Point{}, draw.Src)
	for i, look := range looks {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// Copy opts: append must not write into the caller's backing array.
		eopts := append(append([]Option(nil), opts...), WithLook(look.Name, override))
		e, err := NewEngine(eopts...)
		if err != nil {
			return nil, err
		}
		row, err := e.previewRow(ctx, body, code, width, rowHeight, p)
		if err != nil {
			return nil, err
		}
		draw.Draw(result, image.Rect(0, i*rowHeight, width, (i+1)*rowHeight), row, image.Point{}, draw.Src)
	}
	return result, nil
}

// previewRow draws the sample row of the engine's look. It is a cancellation
// entry point: it recovers the panic that gives up the request into an error.
func (e *Engine) previewRow(ctx context.Context, body, code string, width, rowHeight int, p Palette) (row *image.RGBA, err error) {
	l, err := e.Layout(&Document{}, width, LayoutOptions{})
	if err != nil {
		return nil, err
	}
	defer l.guard(ctx, &err)
	defer e.enter(ctx)()
	size := max(10, min(24, rowHeight/5))
	y := 4
	for j, s := range []string{strings.ToUpper(e.look.Name[:1]) + e.look.Name[1:], body, code} {
		if s == "" {
			continue
		}
		var style Style
		if j == 0 {
			style = Bold
		}
		if j == 2 {
			style = Mono
		}
		ops, _ := l.text([]Span{{Text: s, Style: style}}, size, 10, y, width-20)
		// One wrapped line per sample; keep all runs of its first baseline.
		if len(ops) > 0 {
			baseline := ops[0].baseline
			for _, op := range ops {
				if op.baseline == baseline {
					l.ops = append(l.ops, op)
				}
			}
		}
		y += size*3/2 + 2
	}
	return l.rasterTo(View{Width: width, Height: rowHeight, Scale: 1}, width, rowHeight, RasterOptions{Palette: p}), nil
}
