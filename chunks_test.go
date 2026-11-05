package nttypeset

import (
	"context"
	"errors"
	"fmt"
	"image"
	"strings"
	"testing"
)

type testChunks struct {
	count int
	calls []int
	fail  int
}

func (s *testChunks) Chunks() int { return s.count }
func (s *testChunks) Chunk(ctx context.Context, i int) (*Document, error) {
	s.calls = append(s.calls, i)
	if i == s.fail {
		return nil, errors.New("source failed")
	}
	return &Document{Blocks: []Block{{SourceID: fmt.Sprintf("message-%d", i), Kind: KindParagraph, Spans: []Span{{Text: fmt.Sprintf("Message %d: hello", i), Href: "https://example.com"}}}}, Sections: []Section{{Start: 0, End: 1, SourceID: fmt.Sprintf("section-%d", i), Padding: Insets{Left: 10}}}}, nil
}
func TestChunksLazyASTAndBoundedResidency(t *testing.T) {
	e := testEngine(t)
	src := &testChunks{count: 100000, fail: -1}
	c, err := e.NewChunked(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(src.calls) != 0 {
		t.Fatal("eager source load")
	}
	ctx := context.Background()
	first, err := c.At(ctx, 0, 400, LayoutOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if first.Layout.Laid() != 0 {
		t.Fatal("eager layout")
	}
	img, err := first.Layout.Raster(ctx, View{Width: 400, Height: 200, Scale: 1}, RasterOptions{Palette: LightPalette()})
	if err != nil || img == nil {
		t.Fatal(err)
	}
	if links := first.Layout.Links(View{Width: 400, Height: 200, Scale: 1}); len(links) == 0 || links[0].URL != "https://example.com" {
		t.Fatal("AST links lost")
	}
	last, err := c.At(ctx, 99999, 400, LayoutOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(src.calls) != 2 || last.Document.Blocks[0].SourceID != "message-99999" {
		t.Fatal("last chunk scanned preceding ASTs")
	}
	again, err := c.At(ctx, 0, 400, LayoutOptions{})
	if err != nil || again != first {
		t.Fatal("neighbor not cached", err)
	}
	if _, err = c.At(ctx, 1, 400, LayoutOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err = c.At(ctx, 2, 400, LayoutOptions{}); err != nil {
		t.Fatal(err)
	}
	again, err = c.At(ctx, 0, 400, LayoutOptions{})
	if err != nil || again == first {
		t.Fatal("evicted chunk was retained", err)
	}
	if c.recent[0].Index != 0 || c.recent[1].Index != 2 {
		t.Fatal("cache exceeded neighboring chunks")
	}
	// Reflow keeps source identity, while replacing the width-dependent layout.
	wide, err := c.At(ctx, 0, 800, LayoutOptions{})
	if err != nil || wide.Document != again.Document || wide.Layout == again.Layout {
		t.Fatal("bad reflow", err)
	}
	src.fail = 3
	if _, err = c.At(ctx, 3, 800, LayoutOptions{}); err == nil {
		t.Fatal("lost source error")
	}
	if got, _ := c.At(ctx, 0, 800, LayoutOptions{}); got != wide {
		t.Fatal("error discarded readable chunk")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	calls := len(src.calls)
	if _, err = c.At(cancelled, 4, 400, LayoutOptions{}); !errors.Is(err, context.Canceled) || len(src.calls) != calls {
		t.Fatal("ignored cancellation", err)
	}
	if _, err = c.At(ctx, -1, 400, LayoutOptions{}); err == nil {
		t.Fatal("accepted negative chunk")
	}
	if _, err = c.At(ctx, c.Chunks(), 400, LayoutOptions{}); err == nil {
		t.Fatal("accepted past-end chunk")
	}
	if _, err = e.SetFonts("technical", ""); err != nil {
		t.Fatal(err)
	}
	fresh, err := c.At(ctx, 0, 800, LayoutOptions{})
	if err != nil || fresh.Layout == wide.Layout {
		t.Fatal("stale fonts", err)
	}
}

func TestLineChunksFullHistoryAndNumbering(t *testing.T) {
	ctx := context.Background()
	text := strings.Repeat("+hello 世界\n", 250000) + "last line"
	source, err := NewLineChunks(ctx, text, "diff", nil)
	if err != nil {
		t.Fatal(err)
	}
	if source.Lines() != 250001 || source.Chunks() < 2 {
		t.Fatal("history truncated")
	}
	e := testEngine(t)
	chunks, err := e.NewChunked(source)
	if err != nil {
		t.Fatal(err)
	}
	var restored strings.Builder
	for i := 0; i < source.Chunks(); i++ {
		first, n := source.LineRange(i)
		if n < 1 || n > 256 || source.ChunkForLine(first) != i || source.ChunkForLine(first+n-1) != i {
			t.Fatal("incorrect line mapping", i)
		}
		// No parser or layout is needed to find a chunk or count its lines.
		part := source.chunks[i]
		restored.WriteString(source.text[part.start:part.end])
	}
	if restored.String() != text {
		t.Fatal("chunk boundaries changed source")
	}
	index := source.Chunks() - 1
	first, _ := source.LineRange(index)
	chunk, err := chunks.At(ctx, index, 600, LayoutOptions{Numbers: true, LineOffset: first - 1, LineCount: source.Lines()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = chunk.Layout.Raster(ctx, View{Width: 600, Height: 200, Scale: 1}, RasterOptions{Palette: LightPalette()})
	if err != nil {
		t.Fatal(err)
	}
	var gutter strings.Builder
	for _, op := range chunk.Layout.ops {
		if op.gutter {
			gutter.WriteString(string(op.text))
		}
	}
	if !strings.Contains(gutter.String(), fmt.Sprint(first)) {
		t.Fatal("gutter uses chunk-local numbers", gutter.String())
	}
	hit, ok := chunk.Layout.TextAt(image.Pt(200, 60))
	if ok && !strings.Contains(hit.Text, "hello") {
		t.Fatal("wrong text hit", hit.Text)
	}
}

func TestLineChunksBoundsAndCancellation(t *testing.T) {
	for _, input := range []string{"", "one\n", "one\ntwo", "one\n\n", strings.Repeat("a", 32767) + "\n" + strings.Repeat("b", 32767) + "\nlast"} {
		s, err := NewLineChunks(context.Background(), input, "", nil)
		if err != nil {
			t.Fatal(err)
		}
		n := strings.Count(strings.TrimSuffix(input, "\n"), "\n") + 1
		if s.Lines() != n {
			t.Fatal("line count", s.Lines(), n)
		}
		for i := 0; i < s.Chunks(); i++ {
			doc, err := s.Chunk(context.Background(), i)
			if err != nil || len(doc.Blocks) > 256 {
				t.Fatal("unbounded chunk", err)
			}
		}
	}
	if _, err := NewLineChunks(context.Background(), "\xff", "", nil); err == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
	if _, err := NewLineChunks(context.Background(), strings.Repeat("x", 65537), "", nil); !errors.Is(err, ErrLimit) {
		t.Fatal("oversized line accepted", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewLineChunks(ctx, "text", "", nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

type panicChunks struct{ count bool }

func (s panicChunks) Chunks() int {
	if s.count {
		panic("count failed")
	}
	return 1
}
func (panicChunks) Chunk(context.Context, int) (*Document, error) { panic("load failed") }
func TestChunksRecoverProvidersAndKeepLayoutLimits(t *testing.T) {
	e := testEngine(t)
	if _, err := e.NewChunked(panicChunks{count: true}); err == nil {
		t.Fatal("source count panic escaped")
	}
	c, err := e.NewChunked(panicChunks{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.At(context.Background(), 0, 400, LayoutOptions{}); err == nil {
		t.Fatal("source load panic escaped")
	}
	src, err := NewLineChunks(context.Background(), strings.Repeat("x", 16385), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	c, err = e.NewChunked(src)
	if err != nil {
		t.Fatal(err)
	}
	chunk, err := c.At(context.Background(), 0, 400, LayoutOptions{})
	if err == nil {
		err = chunk.Layout.Complete(context.Background())
	}
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("chunk bypassed rune limit: %v", err)
	}
}
