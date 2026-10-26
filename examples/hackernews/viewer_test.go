package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"nimbleterminal.dev/ntfonts"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	nt "nimbleterminal.dev/ntpicture"
	"nimbleterminal.dev/nttypeset"
	"nimbleterminal.dev/nttypeset/examples/internal/demorun"
)

func press(m *viewer, r rune) tea.Cmd {
	_, cmd := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	return cmd
}

func TestViewerStoryNavigationAndCopy(t *testing.T) {
	pic := nt.New(nt.Options{AutoMode: true})
	defer func() { _ = pic.Close() }()
	m := &viewer{
		pic: pic,
		doc: &nttypeset.Document{},
		stories: []story{
			{ID: 1, Title: "one", URL: "https://example.com/1"},
			{ID: 2, Title: "two", URL: "https://example.com/2"},
		},
		height: 200,
	}
	press(m, 'j')
	if m.selected != 1 {
		t.Fatalf("selected = %d, want 1", m.selected)
	}
	press(m, 'j')
	if m.selected != 1 {
		t.Fatal("j moved beyond the last story")
	}
	press(m, 'k')
	if m.selected != 0 {
		t.Fatalf("selected = %d, want 0", m.selected)
	}
	press(m, 'G')
	if m.selected != 1 || !m.follow {
		t.Fatalf("G selected %d, follow %v", m.selected, m.follow)
	}
	if cmd := press(m, 'y'); cmd == nil || !strings.Contains(fmt.Sprint(cmd()), "https://example.com/2") {
		t.Fatal("y did not copy the selected story URL")
	}
	if cmd := press(m, 'o'); cmd == nil {
		t.Fatal("o returned no open command")
	}
	if cmd := press(m, 'c'); cmd == nil {
		t.Fatal("c returned no comments command")
	}
}

func TestViewerEmptyStoriesDoNotMoveSelectionNegative(t *testing.T) {
	pic := nt.New(nt.Options{AutoMode: true})
	defer func() { _ = pic.Close() }()
	m := &viewer{pic: pic, doc: &nttypeset.Document{}}
	press(m, 'j')
	if m.selected != 0 {
		t.Fatalf("selected = %d", m.selected)
	}
}

func TestViewerKeysFitBooba(t *testing.T) {
	m := &viewer{}
	if err := demorun.Check(m, "hackernews", "Read Hacker News"); err != nil {
		t.Fatal(err)
	}
}

func TestInitialFetchFailureCanRetry(t *testing.T) {
	pic := nt.New(nt.Options{AutoMode: true})
	defer func() { _ = pic.Close() }()
	calls := 0
	client := &hnClient{baseURL: "https://example.com/v0", http: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		status, body := http.StatusOK, `[]`
		if calls == 1 {
			status, body = http.StatusServiceUnavailable, "unavailable"
		}
		return &http.Response{StatusCode: status, Status: http.StatusText(status), Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}}
	m := &viewer{ctx: context.Background(), client: client, pic: pic, o: options{count: 3}}
	cmd := m.fetchStories()
	if cmd == nil || !m.loading {
		t.Fatal("initial fetch did not start")
	}
	if m.fetchStories() != nil {
		t.Fatal("duplicate fetch started")
	}
	m.Update(cmd())
	if m.loading || m.err == nil {
		t.Fatal("failure did not leave a retryable state")
	}
	retry := press(m, 'r')
	if retry == nil || !m.loading {
		t.Fatal("retry did not start")
	}
	// A stale response must not complete a newer request.
	m.Update(refreshMsg{generation: 1})
	if !m.loading {
		t.Fatal("stale response completed retry")
	}
	m.Update(retry())
	if m.loading || m.err != nil || calls != 2 {
		t.Fatalf("retry: loading=%v err=%v calls=%d", m.loading, m.err, calls)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestViewerZoomKeysAndLimits(t *testing.T) {
	pic := nt.New(nt.Options{AutoMode: true})
	defer func() { _ = pic.Close() }()
	m := &viewer{pic: pic}
	if m.zoomPercent() != 100 {
		t.Fatal("default zoom", m.zoomPercent())
	}
	press(m, '+')
	if m.zoomPercent() != 110 {
		t.Fatal("plus did not zoom", m.zoomPercent())
	}
	press(m, '=')
	if m.zoomPercent() != 120 {
		t.Fatal("equals did not zoom", m.zoomPercent())
	}
	press(m, '-')
	if m.zoomPercent() != 110 {
		t.Fatal("minus did not zoom", m.zoomPercent())
	}
	press(m, '0')
	if m.zoomPercent() != 100 {
		t.Fatal("reset did not restore zoom")
	}
	for range 30 {
		press(m, '=')
	}
	if m.zoomPercent() != hnMaxZoom {
		t.Fatal("zoom exceeds maximum", m.zoomPercent())
	}
	for range 30 {
		press(m, '-')
	}
	if m.zoomPercent() != hnMinZoom {
		t.Fatal("zoom below minimum", m.zoomPercent())
	}
	for _, percent := range []int{50, 100, 200} {
		m.zoom = percent
		if err := demorun.Check(m, "hackernews", "Read Hacker News"); err != nil {
			t.Fatalf("%d%% actions: %v", percent, err)
		}
	}
}

func TestViewerZoomReflowsAndKeepsLinksAligned(t *testing.T) {
	eng, err := nttypeset.NewEngine(nttypeset.WithFonts(ntfonts.Embedded), nttypeset.WithLook("technical", ""))
	if err != nil {
		t.Fatal(err)
	}
	pic := nt.New(nt.Options{AutoMode: true})
	defer func() { _ = pic.Close() }()
	pic.SetSize(120, 80)
	stories := make([]story, 20)
	for i := range stories {
		stories[i] = story{ID: i + 1, Title: strings.Repeat("A readable story title ", 3), URL: fmt.Sprintf("https://example.com/%d", i+1)}
	}
	m := &viewer{ctx: context.Background(), eng: eng, pic: pic, stories: stories, doc: buildDocument(stories, time.Now()), selected: 5, o: options{width: 800}}
	m.redraw()
	initialHeight := m.lay.Bounds()[11].Dy()
	for _, percent := range []int{200, 50, 100} {
		m.hover = []nttypeset.Mark{{Block: 1, End: 5}}
		m.hoverURL = "old link"
		m.setZoom(percent)
		if m.err != nil {
			t.Fatal(m.err)
		}
		if m.lay.Width() != 800*100/percent || m.selected != 5 || m.hoverURL != "" || len(m.hover) != 0 {
			t.Fatalf("zoom state %d: width=%d selected=%d hover=%s", percent, m.lay.Width(), m.selected, m.hoverURL)
		}
		if m.view.Scale != float64(m.geom.Pixels.X)/float64(m.lay.Width()) || m.view.Pixels != m.geom.Pixels {
			t.Fatal("raster and view geometry disagree")
		}
		title := m.lay.Bounds()[11]
		if percent == 200 && title.Dy() <= initialHeight {
			t.Fatal("larger text did not reflow")
		}
		if title.Min.Y < m.top || title.Max.Y > m.top+m.height {
			t.Fatal("selected story left viewport", title, m.view)
		}
		found := false
		for _, link := range m.lay.Links(m.view) {
			if link.URL != stories[m.selected].URL {
				continue
			}
			center := link.Rect.Min.Add(link.Rect.Size().Div(2))
			x, y := center.X*m.geom.Cells.X/m.geom.Pixels.X, center.Y*m.geom.Cells.Y/m.geom.Pixels.Y
			if raw, ok := m.linkAt(x, y); !ok || raw != link.URL {
				t.Fatalf("%d%% link hit: %q %v, want %s", percent, raw, ok, link.URL)
			}
			found = true
			break
		}
		if !found {
			t.Fatal("selected story has no visible link")
		}
	}
	m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if m.top != m.view.Top {
		t.Fatal("scroll position and hit geometry disagree")
	}
	m.pic.SetSize(100, 60)
	m.redraw()
	if m.err != nil || m.view.Pixels != m.pic.RasterGeometry().Pixels {
		t.Fatal("resize after zoom", m.err)
	}
}
