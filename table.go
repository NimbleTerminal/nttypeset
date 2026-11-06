package nttypeset

import (
	"fmt"
	"image"
	"image/color"
	"math/bits"
)

// CellFormat supplies physical insets, paint and horizontal text alignment.
// TextAlign is "", "left", "center" or "right". Empty means left.
// Zero padding and transparent paint are explicit, not semantic defaults.
// Borders are separate solid edges; transparent borders still reserve space.
// Cells align to the top of their row. Sizes use the same limits as Insets.
type CellFormat struct {
	Padding                  Insets
	Background               color.RGBA
	Top, Right, Bottom, Left Border
	TextAlign                string
}

// TableFormat opts into cell styling without implicit header fill or row rules.
// Cells must exactly match a nonempty rectangular Block.Rows. Column allocation
// remains content-weighted; there are no spans, border collapsing or CSS sizing.
// Total styled cells per Document cannot exceed Limits.Blocks. Callers keep the
// document and its style slices immutable for the lifetime of a Layout.
type TableFormat struct{ Cells [][]CellFormat }

func (l *Layout) initTables() error {
	remaining := l.e.limits.Blocks
	for _, b := range l.doc.Blocks {
		if b.Table == nil {
			continue
		}
		if b.Kind != KindTable || len(b.Rows) == 0 || len(b.Rows) != len(b.Table.Cells) || len(b.Rows[0]) == 0 {
			return fmt.Errorf("nttypeset: table styles require matching nonempty table rows")
		}
		cols := len(b.Rows[0])
		if len(b.Rows) > remaining/cols {
			return &LimitError{What: "typeset styled table cells exceed block budget", Limit: int64(l.e.limits.Blocks)}
		}
		remaining -= len(b.Rows) * cols
		for r, row := range b.Table.Cells {
			if len(row) != cols || len(b.Rows[r]) != cols {
				return fmt.Errorf("nttypeset: styled tables must be rectangular with one format per cell")
			}
			for _, c := range row {
				if err := validInsets(c.Padding); err != nil {
					return err
				}
				if err := validSpacing(c.Top.Width, c.Right.Width, c.Bottom.Width, c.Left.Width); err != nil {
					return err
				}
				switch c.TextAlign {
				case "", "left", "center", "right":
				default:
					return fmt.Errorf("nttypeset: unsupported table text alignment %q", c.TextAlign)
				}
			}
		}
	}
	return nil
}

func (c CellFormat) insets() Insets {
	return Insets{Top: c.Padding.Top + c.Top.Width, Right: c.Padding.Right + c.Right.Width, Bottom: c.Padding.Bottom + c.Bottom.Width, Left: c.Padding.Left + c.Left.Width}
}

func alignCell(ops []operation, x, width int, align string) {
	if align != "center" && align != "right" {
		return
	}
	for start := 0; start < len(ops); {
		end := start + 1
		right := ops[start].rect.Max.X
		for end < len(ops) && ops[end].baseline == ops[start].baseline {
			right = max(right, ops[end].rect.Max.X)
			end++
		}
		dx := max(0, x+width-right)
		if align == "center" {
			dx /= 2
		}
		for i := start; i < end; i++ {
			ops[i].rect = ops[i].rect.Add(image.Pt(dx, 0))
		}
		start = end
	}
}

func (l *Layout) tableOperationBudget(pending int) {
	if len(l.ops)+pending > l.e.limits.Operations {
		panic(&LimitError{What: "typeset table exceeds display-list operations", Limit: int64(l.e.limits.Operations)})
	}
}

func (l *Layout) styledTable(b *Block, x, width int) {
	cols := len(b.Rows[0])
	weights := make([]int, cols)
	var sum uint64
	for r, row := range b.Rows {
		l.e.check()
		for i, cell := range row {
			s := l.e.shape(cell, tableSize(b))
			advance := 0
			for _, run := range s.runs {
				advance += run.Advance.Ceil()
			}
			inset := b.Table.Cells[r][i].insets()
			weights[i] = max(weights[i], min(width/2, max(80, advance+inset.Left+inset.Right)))
		}
	}
	for _, w := range weights {
		sum += uint64(w)
	}
	if sum == 0 {
		return
	}
	for r, row := range b.Rows {
		l.e.check()
		var ops []operation
		widths := make([]int, cols)
		xx, height := x, 0
		for i, cell := range row {
			l.e.check()
			style := b.Table.Cells[r][i]
			inset := style.insets()
			// Avoid overflow on 32-bit hosts and very wide logical layouts.
			hi, lo := bits.Mul64(uint64(width), uint64(weights[i]))
			share, _ := bits.Div64(hi, lo, sum)
			w := int(share)
			if i == cols-1 {
				w = x + width - xx
			}
			widths[i] = w
			contentWidth := max(1, w-inset.Left-inset.Right)
			cellOps, h := l.text(cell, tableSize(b), xx+inset.Left, l.y+inset.Top, contentWidth)
			alignCell(cellOps, xx+inset.Left, contentWidth, style.TextAlign)
			l.tableOperationBudget(len(ops) + len(cellOps))
			ops = append(ops, cellOps...)
			height = max(height, h+inset.Top+inset.Bottom)
			xx += w
		}
		if l.y+height+Margin > l.e.limits.Height {
			panic(&LimitError{What: "typeset table exceeds layout height", Limit: int64(l.e.limits.Height)})
		}
		xx = x
		for i, c := range b.Table.Cells[r] {
			box := image.Rect(xx, l.y, xx+widths[i], l.y+height)
			l.solid(box, c.Background)
			l.solid(image.Rect(box.Min.X, box.Min.Y, min(box.Max.X, box.Min.X+c.Left.Width), box.Max.Y), c.Left.Color)
			l.solid(image.Rect(max(box.Min.X, box.Max.X-c.Right.Width), box.Min.Y, box.Max.X, box.Max.Y), c.Right.Color)
			l.solid(image.Rect(box.Min.X, box.Min.Y, box.Max.X, min(box.Max.Y, box.Min.Y+c.Top.Width)), c.Top.Color)
			l.solid(image.Rect(box.Min.X, max(box.Min.Y, box.Max.Y-c.Bottom.Width), box.Max.X, box.Max.Y), c.Bottom.Color)
			l.tableOperationBudget(len(ops))
			xx += widths[i]
		}
		l.ops = append(l.ops, ops...)
		l.y += height
	}
	l.y += blockGap(b, 20)
}
