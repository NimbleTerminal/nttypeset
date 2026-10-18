package nttypeset

import (
	"context"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
)

func TestHighlightCode(t *testing.T) {
	style := styles.Get("github")
	for _, tc := range []struct{ language, source string }{
		{"go", "// café\nfunc greeting() string {\n\treturn \"résumé\"\n}"},
		{"python", "# café\ndef greeting():\n    return \"résumé\"\n"},
		{"json", "{\"answer\": 42, \"ok\": true}"},
	} {
		t.Run(tc.language, func(t *testing.T) {
			spans := Highlight(context.Background(), tc.source, tc.language, style)
			var reconstructed strings.Builder
			tokens := map[chroma.TokenType]bool{}
			for _, s := range spans {
				reconstructed.WriteString(s.Text)
				if s.Token != 0 {
					tokens[s.Token] = true
				}
				if s.Style&Mono == 0 {
					t.Fatal("code lost monospace face")
				}
				if s.Ink.A != 0 || s.Token == 0 {
					t.Fatalf("span %+v: Ink must be unset and Token set", s)
				}
			}
			if reconstructed.String() != tc.source {
				t.Fatal("tokenization changed source")
			}
			if len(tokens) < 2 {
				t.Fatal("missing token types")
			}
		})
	}
	for _, language := range []string{"", "not-a-real-language"} {
		source := "  unknown\n\n\tcode café\n"
		spans := Highlight(context.Background(), source, language, style)
		if len(spans) != 1 || spans[0].Text != source || spans[0].Style != Mono || spans[0].Ink.A != 0 || spans[0].Token != 0 {
			t.Fatal("unknown/untagged code must stay plain")
		}
	}
	if spans := Highlight(context.Background(), "x := 1", "go", nil); len(spans) != 1 || spans[0].Token != 0 {
		t.Fatal("nil style must stay plain")
	}
}

func TestHighlightBoldItalic(t *testing.T) {
	// The borland style makes keywords bold and comments italic.
	spans := Highlight(context.Background(), "// note\nfunc f() {}", "go", styles.Get("borland"))
	var bold, italic bool
	for _, s := range spans {
		bold = bold || s.Style&Bold != 0
		italic = italic || s.Style&Italic != 0
		if s.Style&Mono == 0 {
			t.Fatal("lost Mono")
		}
	}
	if !bold || !italic {
		t.Fatal("Bold or Italic not carried from the style")
	}
}

func TestHighlightTokenLimitAndCancel(t *testing.T) {
	style := styles.Get("github")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	source := "func main() {}\n"
	spans := Highlight(ctx, source, "go", style)
	if len(spans) != 1 || spans[0].Text != source || spans[0].Style != Mono || spans[0].Token != 0 {
		t.Fatalf("cancelled highlight = %+v", spans)
	}
	// More than 32,768 tokens falls back to plain.
	big := strings.Repeat("a;", 20000)
	spans = Highlight(context.Background(), big, "go", style)
	if len(spans) != 1 || spans[0].Text != big || spans[0].Token != 0 {
		t.Fatalf("token-limit highlight returned %d spans", len(spans))
	}
}
