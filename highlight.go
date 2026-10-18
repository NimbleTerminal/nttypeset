package nttypeset

import (
	"context"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// maxHighlightTokens bounds the tokens Highlight will accept from a lexer.
const maxHighlightTokens = 32768

// Highlight tokenizes source as language with chroma and returns styled spans
// for a Code block or a Lines document. Every span is Mono, with Bold and
// Italic taken from style's entry for the token. Token is always set; Ink is
// never set, because the Palette's Syntax colors tokens at paint time.
// Adjacent tokens of equal token type and style are coalesced into one span.
//
// Highlight never guesses a language and never fails: it returns one plain
// span {Text: source, Style: Mono} when language is empty or unknown, style is
// nil, the lexer errors or panics, it yields more than 32,768 tokens, ctx is
// cancelled, or the lexer's output does not reconstruct source exactly.
func Highlight(ctx context.Context, source, language string, style *chroma.Style) (out []Span) {
	plain := []Span{{Text: source, Style: Mono}}
	defer func() {
		if recover() != nil {
			out = plain
		}
	}()
	lexer := lexers.Get(language)
	if language == "" || lexer == nil || style == nil {
		return plain
	}
	iterator, err := chroma.Coalesce(lexer).Tokenise(&chroma.TokeniseOptions{State: "root", Nested: true}, source)
	if err != nil {
		return plain
	}
	var reconstructed, pending strings.Builder
	var current Span
	count := 0
	flush := func() {
		if pending.Len() > 0 {
			current.Text = pending.String()
			out = append(out, current)
			pending.Reset()
		}
	}
	for token := iterator(); token != chroma.EOF; token = iterator() {
		count++
		if count > maxHighlightTokens || (ctx != nil && ctx.Err() != nil) {
			return plain
		}
		if token.Value == "" {
			continue
		}
		reconstructed.WriteString(token.Value)
		entry := style.Get(token.Type)
		sp := Span{Text: token.Value, Style: Mono, Token: token.Type}
		if entry.Bold == chroma.Yes {
			sp.Style |= Bold
		}
		if entry.Italic == chroma.Yes {
			sp.Style |= Italic
		}
		// Buffer adjacent equal styles: repeated string concatenation would
		// make punctuation-heavy untrusted code quadratic.
		if pending.Len() > 0 && (current.Token != sp.Token || current.Style != sp.Style) {
			flush()
		}
		current = sp
		pending.WriteString(sp.Text)
	}
	flush()

	// A lexer must never add or remove code, indentation or line breaks.
	if reconstructed.String() != source {
		return plain
	}
	return out
}
