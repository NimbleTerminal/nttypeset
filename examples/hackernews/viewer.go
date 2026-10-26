package main

import (
	"context"
	"fmt"
	"image"
	"slices"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	nt "nimbleterminal.dev/ntpicture"
	"nimbleterminal.dev/nttypeset"
	"nimbleterminal.dev/nttypeset/examples/internal/demorun"
)

const (
	hnScrollStep = 60
	hnMinZoom    = 50
	hnMaxZoom    = 200
	hnZoomStep   = 10
	mouseSettle  = 50 * time.Millisecond
)

func settleMouse() tea.Msg {
	time.Sleep(mouseSettle)
	return nil
}

type refreshMsg struct {
	generation int
	stories    []story
	err        error
}

type openedMsg struct{ err error }

type viewer struct {
	ctx     context.Context
	client  *hnClient
	eng     *nttypeset.Engine
	pic     *nt.Picture
	o       options
	stories []story
	doc     *nttypeset.Document
	lay     *nttypeset.Layout

	zoom     int // percent; zero means 100
	selected int
	follow   bool
	top      int
	height   int
	width    int
	geom     nt.RasterGeometry
	view     nttypeset.View
	hover    []nttypeset.Mark
	hoverURL string
	note     string
	err      error
	loading  bool
	refresh  int
	quitting bool
}

func show(ctx context.Context, client *hnClient, eng *nttypeset.Engine, stories []story, o options) error {
	pic := nt.New(nt.Options{AutoMode: true})
	defer func() { _ = pic.Close() }()
	m := &viewer{ctx: ctx, client: client, eng: eng, pic: pic, stories: stories, o: o}
	m.doc = buildDocument(stories, time.Now())
	return demorun.Run(m, "hackernews", "Read Hacker News")
}

type bindings struct {
	down, up, pgdown, pgup, top, end key.Binding
	open, comments, copy, refresh    key.Binding
	zoomIn, zoomOut, zoomReset       key.Binding
	keep, quit                       key.Binding
}

func newBindings() bindings {
	return bindings{
		down:      demorun.Binding("Next story", "j", "down"),
		up:        demorun.Binding("Previous story", "k", "up"),
		pgdown:    demorun.Binding("Page down", "space", "pgdown"),
		pgup:      demorun.Binding("Page up", "pgup"),
		top:       demorun.Binding("Top", "g", "home"),
		end:       demorun.Binding("End", "G", "end"),
		open:      demorun.Binding("Open story", "enter", "o"),
		comments:  demorun.Binding("Comments", "c"),
		copy:      demorun.Binding("Copy story URL", "y"),
		refresh:   demorun.Binding("Refresh", "r"),
		zoomIn:    demorun.Binding("Zoom in", "=", "+"),
		zoomOut:   demorun.Binding("Zoom out", "-", "_"),
		zoomReset: demorun.Binding("Reset zoom", "0"),
		keep:      demorun.Binding("Keep screen & quit", "Q"),
		quit:      demorun.Binding("Quit", "q", "ctrl+c"),
	}
}

func (b bindings) keys() demorun.Keys {
	return demorun.Keys{
		Primary: []key.Binding{b.down, b.up, b.pgdown, b.pgup, b.open, b.comments, b.refresh, b.zoomIn, b.zoomOut},
		More:    []key.Binding{b.zoomReset, b.copy, b.top, b.end, b.keep, b.quit},
	}
}

func (m *viewer) HelpKeys() demorun.Keys {
	b := newBindings()
	b.zoomIn.SetEnabled(m.zoomPercent() < hnMaxZoom)
	b.zoomOut.SetEnabled(m.zoomPercent() > hnMinZoom)
	b.zoomReset.SetEnabled(m.zoomPercent() != 100)
	return b.keys()
}

func (m *viewer) zoomPercent() int {
	if m.zoom == 0 {
		return 100
	}
	return m.zoom
}

func (m *viewer) setZoom(percent int) tea.Cmd {
	percent = max(hnMinZoom, min(hnMaxZoom, percent))
	if percent == m.zoomPercent() {
		return nil
	}
	m.zoom = percent
	// Reflow at the effective width, keeping the selected story in view.
	m.lay, m.top, m.follow = nil, 0, true
	m.hover, m.hoverURL = nil, ""
	return m.redraw()
}

func (m *viewer) Init() tea.Cmd {
	return tea.Batch(m.pic.Init(), m.fetchStories())
}

func (m *viewer) fetchStories() tea.Cmd {
	if m.loading {
		return nil
	}
	m.loading = true
	m.refresh++
	generation := m.refresh
	return func() tea.Msg {
		stories, err := m.client.stories(m.ctx, m.o.count)
		return refreshMsg{generation: generation, stories: stories, err: err}
	}
}

func (m *viewer) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.quitting {
		switch msg.(type) {
		case tea.KeyPressMsg, tea.MouseMsg:
			return m, nil
		}
	}
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.key(msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		cmd := m.pic.Update(msg)
		return m, tea.Batch(cmd, m.pic.SetSize(max(0, msg.Width), max(0, msg.Height-2)), m.redraw())
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelDown:
			m.top += hnScrollStep
			return m, m.redraw()
		case tea.MouseWheelUp:
			m.top = max(0, m.top-hnScrollStep)
			return m, m.redraw()
		}
	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft && msg.Mod&(tea.ModShift|tea.ModAlt) == 0 && m.live() {
			if raw, ok := m.linkAt(msg.X, msg.Y); ok {
				m.hover, m.hoverURL = nil, ""
				return m, tea.Batch(m.redraw(), openLink(raw))
			}
		}
	case tea.MouseMotionMsg:
		if m.live() {
			return m, m.hoverAt(msg.X, msg.Y)
		}
	case refreshMsg:
		if msg.generation != m.refresh {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.stories = msg.stories
		m.doc = buildDocument(m.stories, time.Now())
		m.selected = min(m.selected, max(0, len(m.stories)-1))
		m.lay, m.top, m.err = nil, 0, nil
		m.note = fmt.Sprintf("refreshed %d stories", len(m.stories))
		return m, m.redraw()
	case openedMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.err = nil
			m.note = "opened link"
		}
		return m, nil
	}
	cmd := m.pic.Update(msg)
	if m.pic.RasterGeometry() != m.geom {
		return m, tea.Batch(cmd, m.redraw())
	}
	return m, cmd
}

func (m *viewer) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	b := newBindings()
	m.note, m.err = "", nil
	switch {
	case key.Matches(msg, b.quit):
		m.quitting = true
		return m, tea.Sequence(m.pic.Close(), settleMouse, tea.Quit)
	case key.Matches(msg, b.keep):
		m.quitting = true
		return m, tea.Sequence(m.pic.Keep(), settleMouse, tea.Quit)
	case key.Matches(msg, b.down):
		m.selected = max(0, min(len(m.stories)-1, m.selected+1))
		m.follow = true
	case key.Matches(msg, b.up):
		m.selected = max(0, m.selected-1)
		m.follow = true
	case key.Matches(msg, b.pgdown):
		m.top += m.height
	case key.Matches(msg, b.pgup):
		m.top = max(0, m.top-m.height)
	case key.Matches(msg, b.top):
		m.selected, m.top, m.follow = 0, 0, true
	case key.Matches(msg, b.end):
		m.selected = max(0, len(m.stories)-1)
		m.follow = true
	case key.Matches(msg, b.open):
		if len(m.stories) > 0 {
			return m, openLink(m.stories[m.selected].storyURL())
		}
	case key.Matches(msg, b.comments):
		if len(m.stories) > 0 {
			return m, openLink(m.stories[m.selected].discussionURL())
		}
	case key.Matches(msg, b.copy):
		if len(m.stories) > 0 {
			m.note = "copied story URL"
			return m, tea.SetClipboard(m.stories[m.selected].storyURL())
		}
	case key.Matches(msg, b.zoomIn):
		return m, m.setZoom(m.zoomPercent() + hnZoomStep)
	case key.Matches(msg, b.zoomOut):
		return m, m.setZoom(m.zoomPercent() - hnZoomStep)
	case key.Matches(msg, b.zoomReset):
		return m, m.setZoom(100)
	case key.Matches(msg, b.refresh):
		return m, m.fetchStories()
	default:
		return m, nil
	}
	return m, m.redraw()
}

func (m *viewer) live() bool {
	return m.lay != nil && m.err == nil && m.pic.RasterGeometry() == m.geom
}

func (m *viewer) pageAt(x, y int) (image.Point, bool) {
	g := m.geom
	if g.Cells.X <= 0 || g.Cells.Y <= 0 || g.Pixels.X <= 0 || g.Pixels.Y <= 0 {
		return image.Point{}, false
	}
	if x < 0 || y < 0 || x >= g.Cells.X || y >= g.Cells.Y {
		return image.Point{}, false
	}
	px := (2*x + 1) * g.Pixels.X / (2 * g.Cells.X)
	py := (2*y + 1) * g.Pixels.Y / (2 * g.Cells.Y)
	px, py = max(0, min(g.Pixels.X-1, px)), max(0, min(g.Pixels.Y-1, py))
	return m.view.Unproject(image.Pt(px, py)), true
}

func (m *viewer) linkAt(x, y int) (string, bool) {
	p, ok := m.pageAt(x, y)
	if !ok {
		return "", false
	}
	for _, link := range m.lay.Links(nttypeset.View{Width: m.lay.Width(), Height: m.lay.Extent(), Scale: 1}) {
		if p.In(link.Rect) {
			return link.URL, true
		}
	}
	return "", false
}

func (m *viewer) hoverAt(x, y int) tea.Cmd {
	raw, ok := m.linkAt(x, y)
	if !ok {
		raw = ""
	}
	if raw == m.hoverURL {
		return nil
	}
	m.hoverURL = raw
	m.hover = nil
	if raw != "" {
		links := m.lay.Links(nttypeset.View{Width: m.lay.Width(), Height: m.lay.Extent(), Scale: 1})
		for i, link := range links {
			if p, ok := m.pageAt(x, y); ok && p.In(link.Rect) {
				m.hover = hnLinkMarks(links, i)
				break
			}
		}
	}
	return m.redraw()
}

func hnLinkMarks(links []nttypeset.Link, i int) []nttypeset.Mark {
	one := func(a, b nttypeset.Link) bool {
		return a.URL == b.URL && a.Block == b.Block && a.Para == b.Para && a.Runes[1] >= b.Runes[0]
	}
	first, last := i, i
	for first > 0 && one(links[first-1], links[first]) {
		first--
	}
	for last+1 < len(links) && one(links[last], links[last+1]) {
		last++
	}
	var marks []nttypeset.Mark
	for _, link := range links[first : last+1] {
		marks = append(marks, link.Mark())
	}
	return marks
}

func (m *viewer) redraw() tea.Cmd {
	g := m.pic.RasterGeometry()
	if g.Pixels.X <= 0 || g.Pixels.Y <= 0 {
		return nil
	}
	m.geom = g
	width := max(1, max(1, m.o.width)*100/m.zoomPercent())
	scale := float64(g.Pixels.X) / float64(width)
	m.height = max(1, int(float64(g.Pixels.Y)/scale))
	if m.lay == nil {
		lay, err := m.eng.Layout(m.doc, width, nttypeset.LayoutOptions{})
		if err != nil {
			m.err = err
			return nil
		}
		m.lay = lay
	}
	if len(m.stories) > 0 {
		if err := m.lay.EnsureBlock(m.ctx, 2+m.selected*2); err != nil {
			m.err = err
			return nil
		}
	}
	if err := m.lay.Ensure(m.ctx, m.top+m.height); err != nil {
		m.err = err
		return nil
	}
	if m.follow {
		m.keepSelectedVisible()
		m.follow = false
	}
	if m.lay.Done() {
		m.top = min(m.top, max(0, m.lay.Extent()-m.height))
	}
	v := nttypeset.View{Top: m.top, Width: width, Height: m.height, Scale: scale, Pixels: g.Pixels}
	marks := slices.Clone(m.hover)
	if len(m.stories) > 0 {
		block := 1 + m.selected*2
		marks = append(marks, nttypeset.Mark{Block: block, End: spanRunes(m.doc.Blocks[block].Spans)})
	}
	img, err := m.lay.Raster(m.ctx, v, nttypeset.RasterOptions{Palette: hnPalette(), Marks: marks})
	if err != nil {
		m.err = err
		return nil
	}
	m.err = nil
	m.view = v
	return m.pic.SetImageFor(img, g)
}

func (m *viewer) keepSelectedVisible() {
	if len(m.stories) == 0 {
		return
	}
	bounds := m.lay.Bounds()
	first, last := 1+m.selected*2, 2+m.selected*2
	if last >= len(bounds) {
		return
	}
	top, bottom := bounds[first].Min.Y, bounds[last].Max.Y
	if top < m.top+nttypeset.Margin {
		m.top = max(0, top-nttypeset.Margin)
	}
	if bottom > m.top+m.height {
		m.top = max(0, bottom-m.height+8)
	}
}

func spanRunes(spans []nttypeset.Span) int {
	n := 0
	for _, span := range spans {
		n += utf8.RuneCountInString(span.Text)
	}
	return n
}

func (m *viewer) View() tea.View {
	status := fmt.Sprintf("Hacker News · %d stories · %d%%", len(m.stories), m.zoomPercent())
	if len(m.stories) > 0 {
		status += fmt.Sprintf(" · %d/%d", m.selected+1, len(m.stories))
	}
	if m.loading {
		status += " · refreshing"
	}
	if m.note != "" {
		status += " · " + m.note
	}
	if m.err != nil {
		status += " · " + m.err.Error()
	}
	if m.width > 0 {
		status = ansi.Truncate(status, m.width, "…")
	}
	v := tea.NewView(m.pic.View() + "\n" + status + "\n" + m.HelpKeys().View(m.width))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeAllMotion
	if m.quitting {
		v.MouseMode = tea.MouseModeNone
	}
	return v
}
