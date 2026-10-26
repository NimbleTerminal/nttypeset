package main

import (
	"fmt"
	"image/color"
	"net/url"
	"strings"
	"time"

	"nimbleterminal.dev/nttypeset"
)

var (
	hnBlack = color.RGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xff}
	hnGray  = color.RGBA{R: 0x82, G: 0x82, B: 0x82, A: 0xff}
)

func hnPalette() nttypeset.Palette {
	return nttypeset.Palette{
		Background: color.RGBA{R: 0xf6, G: 0xf6, B: 0xef, A: 0xff},
		Foreground: hnBlack,
		Muted:      hnGray,
		Border:     color.RGBA{R: 0xff, G: 0x66, B: 0x00, A: 0xff},
		Code:       color.RGBA{R: 0xff, G: 0x66, B: 0x00, A: 0xff},
		Header:     color.RGBA{R: 0xff, G: 0x66, B: 0x00, A: 0xff},
		Accent:     hnBlack,
		Selection:  color.RGBA{R: 0xff, G: 0xd8, B: 0xb0, A: 0xff},
		Mark:       color.RGBA{R: 0xff, G: 0xe8, B: 0xd2, A: 0xff},
	}
}

func buildDocument(stories []story, now time.Time) *nttypeset.Document {
	doc := &nttypeset.Document{Blocks: []nttypeset.Block{{
		Kind: nttypeset.KindCode, Size: 20, Spans: headerSpans(),
	}}}
	for i, item := range stories {
		title := []nttypeset.Span{{Text: fmt.Sprintf("%d. ", i+1), Ink: hnGray}}
		title = append(title, nttypeset.Span{Text: item.Title, Href: item.storyURL()})
		if item.URL != "" {
			title = append(title, nttypeset.Span{Text: " (" + storyHost(item.storyURL()) + ")", Ink: hnGray})
		}
		comments := "discuss"
		if item.Descendants != 0 {
			comments = fmt.Sprintf("%d %s", item.Descendants, plural(item.Descendants, "comment"))
		}
		doc.Blocks = append(doc.Blocks,
			nttypeset.Block{Kind: nttypeset.KindLine, Size: 22, Spans: title},
			nttypeset.Block{Kind: nttypeset.KindParagraph, Size: 18, Indent: 34, Spans: []nttypeset.Span{
				{Text: fmt.Sprintf("%d %s by %s %s | ", item.Score, plural(item.Score, "point"), item.By, relativeTime(item.Time, now)), Ink: hnGray},
				{Text: comments, Ink: hnGray, Href: item.discussionURL()},
			}},
		)
	}
	doc.Blocks = append(doc.Blocks,
		nttypeset.Block{Kind: nttypeset.KindRule},
		nttypeset.Block{Kind: nttypeset.KindParagraph, Size: 18, Spans: footerSpans()},
	)
	return doc
}

func headerSpans() []nttypeset.Span {
	return []nttypeset.Span{
		{Text: "Y ", Style: nttypeset.Bold, Href: "https://news.ycombinator.com/"},
		{Text: "Hacker News", Style: nttypeset.Bold, Href: "https://news.ycombinator.com/"},
		{Text: "  "},
		{Text: "new", Href: "https://news.ycombinator.com/newest"},
		{Text: " | "},
		{Text: "past", Href: "https://news.ycombinator.com/front"},
		{Text: " | "},
		{Text: "comments", Href: "https://news.ycombinator.com/newcomments"},
		{Text: " | "},
		{Text: "ask", Href: "https://news.ycombinator.com/ask"},
		{Text: " | "},
		{Text: "show", Href: "https://news.ycombinator.com/show"},
		{Text: " | "},
		{Text: "jobs", Href: "https://news.ycombinator.com/jobs"},
		{Text: " | "},
		{Text: "submit", Href: "https://news.ycombinator.com/submit"},
	}
}

func footerSpans() []nttypeset.Span {
	return []nttypeset.Span{
		{Text: "Guidelines", Href: "https://news.ycombinator.com/newsguidelines.html"},
		{Text: " | "},
		{Text: "FAQ", Href: "https://news.ycombinator.com/newsfaq.html"},
		{Text: " | "},
		{Text: "Lists", Href: "https://news.ycombinator.com/lists"},
		{Text: " | "},
		{Text: "API", Href: "https://github.com/HackerNews/API"},
		{Text: " | "},
		{Text: "Security", Href: "https://news.ycombinator.com/security.html"},
		{Text: " | "},
		{Text: "Legal", Href: "https://www.ycombinator.com/legal/"},
		{Text: " | "},
		{Text: "Apply to YC", Href: "https://www.ycombinator.com/apply/"},
		{Text: " | Contact", Href: "mailto:hn@ycombinator.com"},
	}
}

func storyHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "news.ycombinator.com"
	}
	return strings.TrimPrefix(u.Hostname(), "www.")
}

func relativeTime(unix int64, now time.Time) string {
	seconds := max(0, int(now.Unix()-unix))
	switch {
	case seconds < 60:
		return "just now"
	case seconds < 3600:
		return fmt.Sprintf("%d %s ago", seconds/60, plural(seconds/60, "minute"))
	case seconds < 86400:
		return fmt.Sprintf("%d %s ago", seconds/3600, plural(seconds/3600, "hour"))
	default:
		return fmt.Sprintf("%d %s ago", seconds/86400, plural(seconds/86400, "day"))
	}
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
