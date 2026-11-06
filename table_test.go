package nttypeset

import (
	"context"
	"errors"
	"image"
	"image/color"
	"strings"
	"testing"
)

func TestStyledTablePaint(t *testing.T) {
	red := color.RGBA{220, 30, 40, 255}
	green := color.RGBA{20, 180, 70, 255}
	black := color.RGBA{0, 0, 0, 255}
	d := &Document{Blocks: []Block{{Kind: KindTable, Format: &BlockFormat{LineHeight: 20}, Rows: [][][]Span{{nil, nil}, {nil, nil}}, Table: &TableFormat{Cells: [][]CellFormat{
		{{Padding: Insets{Top: 3, Right: 4, Bottom: 5, Left: 6}, Background: red, Top: Border{2, black}, Right: Border{3, black}, Bottom: Border{4, black}, Left: Border{5, black}}, {Background: green}},
		{{}, {}},
	}}}}}
	e := testEngine(t)
	l := mustLayout(t, e, d, 300, LayoutOptions{})
	p := LightPalette()
	pic, err := l.Raster(context.Background(), View{Width: 300, Height: 200, Scale: 1}, RasterOptions{Palette: p})
	if err != nil {
		t.Fatal(err)
	}
	// Two equal 100px columns at x=50. Row 1 is 20+3+5+2+4=34px;
	// row 2 is 20px. These coordinates come from the fixture, not a golden.
	for _, sample := range []struct {
		x, y int
		c    color.RGBA
	}{
		{50, 60, black}, {100, 50, black}, {148, 60, black}, {100, 82, black},
		{56, 53, red}, {140, 78, red}, {170, 60, green}, {170, 82, green},
		{70, 85, p.Background}, {170, 85, p.Background}, {100, 103, p.Background},
	} {
		if got := pic.RGBAAt(sample.x, sample.y); got != sample.c {
			t.Errorf("(%d,%d)=%v want %v", sample.x, sample.y, got, sample.c)
		}
	}
	if l.Extent() != 154 {
		t.Errorf("extent=%d want 154", l.Extent())
	}
}

func TestStyledTableAlignmentAndSelection(t *testing.T) {
	ctx := context.Background()
	for _, align := range []string{"left", "center", "right"} {
		t.Run(align, func(t *testing.T) {
			text := "one two three four five six seven"
			d := &Document{Blocks: []Block{{Kind: KindTable, Size: 16, Format: &BlockFormat{}, Rows: [][][]Span{{{{Text: text, Href: "https://example.test"}}}}, Table: &TableFormat{Cells: [][]CellFormat{{{Padding: Insets{Left: 10, Right: 10}, TextAlign: align}}}}}}}
			e := testEngine(t)
			for _, width := range []int{220, 300} {
				l := mustLayout(t, e, d, width, LayoutOptions{})
				if err := l.Complete(ctx); err != nil {
					t.Fatal(err)
				}
				lines := map[int]image.Rectangle{}
				for _, op := range l.ops {
					if op.run != nil {
						lines[op.baseline] = lines[op.baseline].Union(op.rect)
					}
				}
				if len(lines) < 2 {
					t.Fatal("fixture did not wrap")
				}
				for _, r := range lines {
					left, right := Margin+10, width-Margin-10
					switch align {
					case "left":
						if r.Min.X != left {
							t.Errorf("left edge=%d want %d", r.Min.X, left)
						}
					case "right":
						if r.Max.X != right {
							t.Errorf("right edge=%d want %d", r.Max.X, right)
						}
					case "center":
						if delta := (r.Min.X - left) - (right - r.Max.X); delta < -1 || delta > 1 {
							t.Errorf("unbalanced center: %v", r)
						}
					}
					if _, ok := l.TextAt(image.Pt(r.Min.X+1, r.Min.Y+1)); !ok {
						t.Error("aligned text lost hit area")
					}
				}
				selected, err := l.Select(ctx, Selection{Anchor: Place{Block: 0, Para: 0, Rune: 0, Placed: true}, Focus: Place{Block: 0, Para: 0, Rune: len([]rune(text)), Placed: true}})
				if err != nil || selected == nil || selected.Text() != text {
					t.Fatalf("selection %v %v", selected, err)
				}
				if len(l.Links(View{Width: width, Height: 400, Scale: 1})) == 0 {
					t.Fatal("lost aligned link")
				}
			}
		})
	}
}

func TestStyledTableValidationAndLimits(t *testing.T) {
	good := func() *Document {
		return &Document{Blocks: []Block{{Kind: KindTable, Rows: [][][]Span{{nil}}, Table: &TableFormat{Cells: [][]CellFormat{{{}}}}}}}
	}
	for _, tc := range []struct {
		name   string
		mutate func(*Document)
	}{
		{"wrong kind", func(d *Document) { d.Blocks[0].Kind = KindParagraph }},
		{"missing row", func(d *Document) { d.Blocks[0].Table.Cells = nil }},
		{"ragged", func(d *Document) { d.Blocks[0].Rows[0] = append(d.Blocks[0].Rows[0], nil) }},
		{"negative inset", func(d *Document) { d.Blocks[0].Table.Cells[0][0].Padding.Left = -1 }},
		{"huge border", func(d *Document) { d.Blocks[0].Table.Cells[0][0].Left.Width = 65537 }},
		{"alignment", func(d *Document) { d.Blocks[0].Table.Cells[0][0].TextAlign = "justify" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := good()
			tc.mutate(d)
			if _, err := testEngine(t).Layout(d, 300, LayoutOptions{}); err == nil {
				t.Fatal("invalid style accepted")
			}
		})
	}
	e, err := NewEngine(WithFonts(aliasSource{}), WithLimits(Limits{Blocks: 1, Operations: 2}))
	if err != nil {
		t.Fatal(err)
	}
	d := good()
	d.Blocks[0].Rows[0] = append(d.Blocks[0].Rows[0], nil)
	d.Blocks[0].Table.Cells[0] = append(d.Blocks[0].Table.Cells[0], CellFormat{})
	if _, err := e.Layout(d, 300, LayoutOptions{}); !errors.Is(err, ErrLimit) {
		t.Fatal("cell budget", err)
	}
	d = good()
	d.Blocks[0].Rows[0][0] = []Span{{Text: strings.Repeat("word ", 100)}}
	l := mustLayout(t, e, d, 200, LayoutOptions{})
	if err := l.Complete(context.Background()); !errors.Is(err, ErrLimit) {
		t.Fatal("operation budget", err)
	}
	if l.Laid() != 0 || len(l.ops) != 0 {
		t.Fatal("failed table retained partial layout")
	}
	l = mustLayout(t, testEngine(t), good(), 200, LayoutOptions{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := l.Complete(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation", err)
	}
}

func TestLegacyTableDecorationUnchanged(t *testing.T) {
	e := testEngine(t)
	d := &Document{Blocks: []Block{{Kind: KindTable, Format: &BlockFormat{LineHeight: 20}, Rows: [][][]Span{{nil}, {nil}}}}}
	l := mustLayout(t, e, d, 300, LayoutOptions{})
	p := LightPalette()
	pic, err := l.Raster(context.Background(), View{Width: 300, Height: 200, Scale: 1}, RasterOptions{Palette: p})
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range []struct {
		y   int
		ink color.RGBA
	}{{60, p.Header}, {89, p.Border}, {100, p.Background}, {129, p.Border}} {
		if got := pic.RGBAAt(100, sample.y); got != sample.ink {
			t.Errorf("legacy y=%d: %v want %v", sample.y, got, sample.ink)
		}
	}
	if l.Extent() != 180 {
		t.Errorf("legacy extent=%d want 180", l.Extent())
	}
}

func TestStyledTableWideLayout(t *testing.T) {
	width := int(^uint(0) >> 1)
	ink := color.RGBA{10, 20, 30, 255}
	d := &Document{Blocks: []Block{{Kind: KindTable, Format: &BlockFormat{LineHeight: 20}, Rows: [][][]Span{{nil, nil}}, Table: &TableFormat{Cells: [][]CellFormat{{{Background: ink}, {Background: ink}}}}}}}
	l := mustLayout(t, testEngine(t), d, width, LayoutOptions{})
	if err := l.Complete(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(l.ops) != 2 {
		t.Fatalf("paint ops=%d", len(l.ops))
	}
	content := width - 2*Margin
	if l.ops[0].rect.Dx() != content/2 || l.ops[1].rect.Max.X != width-Margin || l.ops[0].rect.Max.X != l.ops[1].rect.Min.X {
		t.Fatal("overflow or gap in wide table", l.ops[0].rect, l.ops[1].rect)
	}
}
