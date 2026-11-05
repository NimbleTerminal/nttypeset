package nttypeset

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2"
)

const (
	lineChunkBytes     = 64 << 10
	lineChunkLines     = 256
	maxLineSourceBytes = 128 << 20
)

type lineChunk struct{ start, end, first, lines int }

// LineChunks indexes at most 128 MiB of valid UTF-8 text into chunks of at most
// 256 complete lines and 64 KiB. It retains the input and a small chunk index,
// not the highlighted ASTs. Each line must fit 64 KiB; the ordinary per-line
// rune limit is still enforced by Layout. Normalize CRLF before calling.
// Highlighting is per chunk: diff/log lexers work naturally; lexical state
// spanning chunks (such as a source-language multiline string) is not retained.
// Hosts with parser context should implement ChunkSource with styled ASTs.
type LineChunks struct {
	text, language string
	style          *chroma.Style
	chunks         []lineChunk
	lines          int
}

// NewLineChunks indexes text, with no shaping, highlighting, or layout.
func NewLineChunks(ctx context.Context, text, language string, style *chroma.Style) (out *LineChunks, err error) {
	defer recoverChunk(&err)
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(text) > maxLineSourceBytes {
		return nil, &LimitError{What: "typeset line source exceeds 128 MiB", Limit: maxLineSourceBytes}
	}
	if !utf8.ValidString(text) {
		return nil, fmt.Errorf("nttypeset: line source must be UTF-8")
	}
	out = &LineChunks{text: text, language: language, style: style}
	current := lineChunk{first: 1}
	offset := 0
	for offset < len(text) || len(text) == 0 && offset == 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		end := len(text)
		if n := strings.IndexByte(text[offset:], '\n'); n >= 0 {
			end = offset + n + 1
		}
		if end-offset > lineChunkBytes {
			return nil, &LimitError{What: "typeset line exceeds 64 KiB", Limit: lineChunkBytes}
		}
		if current.lines > 0 && (current.lines == lineChunkLines || end-current.start > lineChunkBytes) {
			out.chunks = append(out.chunks, current)
			current = lineChunk{start: offset, first: out.lines + 1}
		}
		current.end = end
		current.lines++
		out.lines++
		if end == len(text) {
			break
		}
		offset = end
	}
	out.chunks = append(out.chunks, current)
	return out, nil
}

func (s *LineChunks) Chunks() int { return len(s.chunks) }

// Lines is the total file line count. An empty input has one empty display line.
func (s *LineChunks) Lines() int { return s.lines }

// LineRange is the one-based first line and line count of a chunk; invalid
// indexes return zeroes.
func (s *LineChunks) LineRange(index int) (first, count int) {
	if index < 0 || index >= len(s.chunks) {
		return 0, 0
	}
	c := s.chunks[index]
	return c.first, c.lines
}

// ChunkForLine finds a one-based file line, clamped to the source's range.
func (s *LineChunks) ChunkForLine(line int) int {
	line = max(1, min(line, s.lines))
	return max(0, sort.Search(len(s.chunks), func(i int) bool { return s.chunks[i].first > line })-1)
}

// Chunk builds only this chunk's highlighted line AST.
func (s *LineChunks) Chunk(ctx context.Context, index int) (doc *Document, err error) {
	defer recoverChunk(&err)
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if index < 0 || index >= len(s.chunks) {
		return nil, fmt.Errorf("nttypeset: invalid line chunk %d", index)
	}
	c := s.chunks[index]
	doc, err = Lines(ctx, s.text[c.start:c.end], s.language, s.style)
	if err == nil {
		err = ctx.Err()
	}
	return doc, err
}
