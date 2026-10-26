package main

import (
	"context"
	"image/color"
	"strings"
	"testing"
	"time"

	"nimbleterminal.dev/ntfonts"
	"nimbleterminal.dev/nttypeset"
)

func TestDocumentBuildsRankedStoriesAndLinks(t *testing.T) {
	now := time.Unix(4000, 0)
	stories := []story{
		{ID: 1, Title: "External", URL: "https://www.example.com/path", Score: 1, By: "alice", Descendants: 1, Time: now.Add(-time.Minute).Unix()},
		{ID: 2, Title: "Ask HN", Score: 2, By: "bob", Time: now.Add(-2 * time.Hour).Unix()},
	}
	doc := buildDocument(stories, now)
	if len(doc.Blocks) != 7 {
		t.Fatalf("%d blocks, want 7", len(doc.Blocks))
	}
	title := doc.Blocks[1]
	if title.Kind != nttypeset.KindLine || title.Spans[0].Text != "1. " {
		t.Fatalf("title = %+v", title)
	}
	if got := title.Spans[1].Href; got != stories[0].URL {
		t.Fatalf("title link = %q", got)
	}
	if got := title.Spans[2].Text; got != " (example.com)" {
		t.Fatalf("domain = %q", got)
	}
	byline := doc.Blocks[2].Spans
	if !strings.Contains(byline[0].Text, "1 point by alice 1 minute ago") {
		t.Fatalf("byline = %q", byline[0].Text)
	}
	if byline[1].Text != "1 comment" || byline[1].Href != "https://news.ycombinator.com/item?id=1" {
		t.Fatalf("comments = %+v", byline[1])
	}
	if got := doc.Blocks[3].Spans[1].Href; got != stories[1].discussionURL() {
		t.Fatalf("Ask HN link = %q", got)
	}
	if got := doc.Blocks[4].Spans[1].Text; got != "discuss" {
		t.Fatalf("zero comments = %q", got)
	}
}

func TestRelativeTimeAndPlural(t *testing.T) {
	now := time.Unix(200000, 0)
	for _, test := range []struct {
		age  time.Duration
		want string
	}{
		{0, "just now"},
		{time.Minute, "1 minute ago"},
		{2 * time.Minute, "2 minutes ago"},
		{time.Hour, "1 hour ago"},
		{3 * time.Hour, "3 hours ago"},
		{24 * time.Hour, "1 day ago"},
		{48 * time.Hour, "2 days ago"},
		{-time.Hour, "just now"},
	} {
		if got := relativeTime(now.Add(-test.age).Unix(), now); got != test.want {
			t.Errorf("age %s = %q, want %q", test.age, got, test.want)
		}
	}
	if plural(1, "point") != "point" || plural(0, "point") != "points" {
		t.Fatal("plural forms are wrong")
	}
}

func TestRenderUsesHackerNewsColors(t *testing.T) {
	ctx := context.Background()
	eng, err := nttypeset.NewEngine(nttypeset.WithFonts(ntfonts.Embedded), nttypeset.WithLook("technical", ""))
	if err != nil {
		t.Fatal(err)
	}
	doc := buildDocument([]story{{ID: 1, Title: "A typeset front page", URL: "https://example.com", Score: 12, By: "alice", Descendants: 3, Time: time.Now().Unix()}}, time.Now())
	lay, err := eng.Layout(doc, 800, nttypeset.LayoutOptions{})
	if err != nil {
		t.Fatal(err)
	}
	img, err := lay.Raster(ctx, nttypeset.View{Width: 800, Height: 300, Scale: 1}, nttypeset.RasterOptions{Palette: hnPalette()})
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 800 || img.Bounds().Dy() != 300 {
		t.Fatalf("size = %v", img.Bounds().Size())
	}
	palette := hnPalette()
	if countColor(img, palette.Header) == 0 {
		t.Fatal("no orange header pixels")
	}
	if countColor(img, palette.Background) == 0 {
		t.Fatal("no warm background pixels")
	}
	if countColor(img, hnBlack) == 0 {
		t.Fatal("no black text pixels")
	}
}

func countColor(img interface{ RGBAAt(int, int) color.RGBA }, want color.RGBA) int {
	count := 0
	for y := 0; y < 300; y++ {
		for x := 0; x < 800; x++ {
			if img.RGBAAt(x, y) == want {
				count++
			}
		}
	}
	return count
}
