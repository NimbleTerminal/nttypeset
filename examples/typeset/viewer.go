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

// scrollStep is one j/k press, in layout pixels.
const scrollStep = 60

// mouseSettle is how long quitting waits, with mouse reporting turned off,
// before giving the terminal back. Bubble Tea stops reading before it turns
// reporting off, so a report sent as the user quit, the pointer still moving,
// would be left for the shell, typed at its prompt; read here, it is dropped.
const mouseSettle = 50 * time.Millisecond

func settleMouse() tea.Msg {
	time.Sleep(mouseSettle)
	return nil
}

// viewer is the terminal program: the page in an ntpicture Picture, rastered
// at the picture's own device pixels so the terminal copies the raster rather
// than resampling it. The reading scale follows the terminal width. A left
// drag over the picture selects text, which y copies.
type viewer struct {
	ctx    context.Context
	page   *page
	eng    *nttypeset.Engine
	pic    *nt.Picture
	o      options
	lay    *nttypeset.Layout
	top    int               // the page row at the top of the view, in layout pixels
	height int               // the view's height, in layout pixels
	width  int               // the terminal's width, in columns
	geom   nt.RasterGeometry // what the shown raster was made for
	view   nttypeset.View    // the region of the page the shown raster maps to
	last   *image.RGBA       // the raster redraw last submitted
	err    error

	sel      *nttypeset.Selection // the selection; placed once it lands on text
	selected string               // the text of sel
	dragging bool                 // a left drag that began on the picture is in progress
	dragFrom image.Point          // the page point where the drag began
	dragTo   image.Point          // the page point the selection's focus last followed
	note     string               // the last action's result, shown until the next key or click
	hover    []nttypeset.Mark     // the lines of the link under the pointer, tinted
	quitting bool                 // q or Q was pressed: the mouse is off while the program settles
}

func show(ctx context.Context, p *page, eng *nttypeset.Engine, o options) error {
	pic := nt.New(nt.Options{AutoMode: true})
	defer func() { _ = pic.Close() }()
	return demorun.Run(&viewer{ctx: ctx, page: p, eng: eng, pic: pic, o: o, sel: p.sel}, "typeset", "Typeset a page")
}

// bindings are the viewer's keys by name.
type bindings struct {
	down, up, pgdown, pgup, top, end key.Binding
	dark, look, mono, toggle         key.Binding
	numbers, chop, copy, clear       key.Binding
	keep, quit                       key.Binding
}

// newBindings makes the bindings; the Kitty toggle is enabled only where
// Kitty graphics are supported.
func newBindings(kitty bool) bindings {
	b := bindings{
		down:    demorun.Binding("Down", "j", "down"),
		up:      demorun.Binding("Up", "k", "up"),
		pgdown:  demorun.Binding("Page down", "space", "pgdown"),
		pgup:    demorun.Binding("Page up", "pgup"),
		top:     demorun.Binding("Top", "g", "home"),
		end:     demorun.Binding("End", "G", "end"),
		dark:    demorun.Binding("Dark / Light", "d"),
		look:    demorun.Binding("Next look", "f"),
		mono:    demorun.Binding("Mono", "m"),
		toggle:  demorun.Binding("Glyph / Kitty", "t"),
		numbers: demorun.Binding("Line numbers", "n"),
		chop:    demorun.Binding("Chop lines", "c"),
		copy:    demorun.Binding("Copy", "y"),
		clear:   demorun.Binding("Clear selection", "esc"),
		keep:    demorun.Binding("Keep screen & quit", "Q"),
		quit:    demorun.Binding("Quit", "q", "ctrl+c"),
	}
	b.toggle.SetEnabled(kitty)
	return b
}

// keys flattens the bindings into the primary buttons and the rest.
func (b bindings) keys() demorun.Keys {
	return demorun.Keys{
		Primary: []key.Binding{b.down, b.up, b.pgdown, b.pgup, b.top, b.end, b.dark, b.look, b.mono, b.toggle},
		More:    []key.Binding{b.numbers, b.chop, b.copy, b.clear, b.keep, b.quit},
	}
}

// bindings returns the current bindings.
func (m *viewer) bindings() bindings {
	return newBindings(m.pic.KittySupported() == nt.KittyCapabilitySupported)
}

// HelpKeys lists the viewer's bindings: the help row and the browser buttons
// both come from here. The Kitty toggle is offered only where Kitty graphics
// are supported.
func (m *viewer) HelpKeys() demorun.Keys { return m.bindings().keys() }

func (m *viewer) Init() tea.Cmd { return m.pic.Init() }

func (m *viewer) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.quitting {
		switch msg.(type) {
		case tea.KeyPressMsg, tea.MouseMsg:
			return m, nil // What is still typed or pointed at is dropped.
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
		// With mouse reporting on, the terminal sends the wheel as this
		// message, not as arrow keys: it scrolls like j and k.
		switch msg.Button {
		case tea.MouseWheelDown:
			m.top += scrollStep
			return m, m.redraw()
		case tea.MouseWheelUp:
			m.top = max(0, m.top-scrollStep)
			return m, m.redraw()
		}
	case tea.MouseClickMsg, tea.MouseMotionMsg, tea.MouseReleaseMsg:
		if cmd, ok := m.mouse(msg); ok {
			return m, cmd
		}
	}
	cmd := m.pic.Update(msg)
	if m.pic.RasterGeometry() != m.geom {
		return m, tea.Batch(cmd, m.redraw())
	}
	return m, cmd
}

func (m *viewer) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	b := m.bindings()
	if !key.Matches(msg, b.copy) {
		m.note = ""
	}
	switch {
	case key.Matches(msg, b.quit):
		m.quitting = true
		return m, tea.Sequence(m.pic.Close(), settleMouse, tea.Quit)
	case key.Matches(msg, b.keep):
		m.quitting = true
		return m, tea.Sequence(m.pic.Keep(), settleMouse, tea.Quit)
	case key.Matches(msg, b.down):
		m.top += scrollStep
	case key.Matches(msg, b.up):
		m.top = max(0, m.top-scrollStep)
	case key.Matches(msg, b.pgdown):
		m.top += m.height
	case key.Matches(msg, b.pgup):
		m.top = max(0, m.top-m.height)
	case key.Matches(msg, b.top):
		m.top = 0
	case key.Matches(msg, b.end):
		if m.lay != nil {
			if m.err = m.lay.Complete(m.ctx); m.err == nil {
				m.top = max(0, m.lay.Extent()-m.height)
			}
		}
	case key.Matches(msg, b.dark):
		m.o.dark = !m.o.dark
	case key.Matches(msg, b.look):
		m.nextLook()
	case key.Matches(msg, b.mono):
		if m.o.mono == "" {
			m.o.mono = "jetbrains"
		} else {
			m.o.mono = ""
		}
		m.setFonts()
	case key.Matches(msg, b.toggle):
		if c := m.pic.ToggleMode(); c != nil {
			return m, tea.Batch(c, m.redraw())
		}
	case key.Matches(msg, b.numbers):
		m.page.opts.Numbers = !m.page.opts.Numbers
		m.lay = nil
	case key.Matches(msg, b.chop):
		m.page.opts.Chop = !m.page.opts.Chop
		m.lay = nil
	case key.Matches(msg, b.copy):
		if m.selected == "" {
			return m, nil
		}
		m.note = fmt.Sprintf("copied %d characters", utf8.RuneCountInString(m.selected))
		return m, tea.SetClipboard(m.selected)
	case key.Matches(msg, b.clear):
		if m.sel == nil {
			return m, nil
		}
		m.sel, m.selected, m.dragging = nil, "", false
	default:
		return m, nil
	}
	return m, m.redraw()
}

// live reports whether mouse positions map onto the shown raster: the page is
// laid out, nothing failed, and the picture still has the geometry the raster
// was made for.
func (m *viewer) live() bool {
	return m.lay != nil && m.err == nil && m.pic.RasterGeometry() == m.geom
}

// pageAt is the page point under screen cell (x, y), whose picture starts at
// the screen's top left corner. A cell is the raster pixel at its center.
// With clamp, a cell beyond the picture maps to its nearest edge pixel, for a
// drag that leaves it; otherwise it maps to nothing.
func (m *viewer) pageAt(x, y int, clamp bool) (image.Point, bool) {
	g := m.geom
	if g.Cells.X <= 0 || g.Cells.Y <= 0 || g.Pixels.X <= 0 || g.Pixels.Y <= 0 {
		return image.Point{}, false
	}
	if !clamp && (x < 0 || y < 0 || x >= g.Cells.X || y >= g.Cells.Y) {
		return image.Point{}, false
	}
	px := (2*x + 1) * g.Pixels.X / (2 * g.Cells.X)
	py := (2*y + 1) * g.Pixels.Y / (2 * g.Cells.Y)
	px, py = max(0, min(g.Pixels.X-1, px)), max(0, min(g.Pixels.Y-1, py))
	return m.view.Unproject(image.Pt(px, py)), true
}

// hoverAt tints the link under cell x, y, every line of it, and untints the
// one the pointer left.
func (m *viewer) hoverAt(x, y int) tea.Cmd {
	var marks []nttypeset.Mark
	if p, ok := m.pageAt(x, y, false); ok && m.lay != nil {
		links := m.lay.Links(nttypeset.View{Height: m.lay.Extent(), Width: m.lay.Width(), Scale: 1})
		for i, k := range links {
			if p.In(k.Rect) {
				marks = linkMarks(links, i)
				break
			}
		}
	}
	if slices.Equal(marks, m.hover) {
		return nil
	}
	m.hover = marks
	return m.redraw()
}

// linkMarks are the marks of the link that links[i] is a line of: the lines
// beside it of the same URL whose runes meet it.
func linkMarks(links []nttypeset.Link, i int) []nttypeset.Mark {
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
	for _, k := range links[first : last+1] {
		marks = append(marks, k.Mark())
	}
	return marks
}

// mouse follows a left drag over the page and says whether the message was
// the page's, so that nothing else answers it. The page ignores buttons other
// than the left; Shift and Alt drags are left to the terminal.
func (m *viewer) mouse(msg tea.Msg) (tea.Cmd, bool) {
	if !m.live() {
		m.dragging = false
		return nil, false
	}
	switch msg := msg.(type) {
	case tea.MouseClickMsg:
		m.dragging = false
		if msg.Button != tea.MouseLeft || msg.Mod&(tea.ModShift|tea.ModAlt) != 0 {
			return nil, false
		}
		m.note = ""
		var cmd tea.Cmd
		if m.sel != nil {
			m.sel, m.selected = nil, ""
			cmd = m.redraw()
		}
		if p, ok := m.pageAt(msg.X, msg.Y, false); ok {
			m.dragging, m.dragFrom, m.dragTo = true, p, p
		}
		return cmd, true
	case tea.MouseMotionMsg:
		if !m.dragging {
			return m.hoverAt(msg.X, msg.Y), true
		}
		p, ok := m.pageAt(msg.X, msg.Y, true)
		if !ok || (p == m.dragTo && m.sel != nil) {
			return nil, true
		}
		// A new value, not a change in place: the shown selection may be
		// the one this came from.
		sel := nttypeset.Selection{Anchor: nttypeset.Place{Point: m.dragFrom}, Focus: nttypeset.Place{Point: p}}
		if m.sel != nil {
			sel.Anchor = m.sel.Anchor
		}
		m.sel, m.dragTo = &sel, p
		return m.redraw(), true
	case tea.MouseReleaseMsg:
		if !m.dragging {
			return nil, false
		}
		m.dragging = false
		if m.selected == "" {
			m.sel = nil
		}
		return nil, true
	}
	return nil, false
}

// nextLook moves to the next built-in look.
func (m *viewer) nextLook() {
	looks := nttypeset.FontLooks()
	for i, look := range looks {
		if look.Name == m.eng.Look().Name {
			m.o.font = looks[(i+1)%len(looks)].Name
			break
		}
	}
	m.setFonts()
}

// setFonts applies the look and override in the options; a change discards
// the layout, which the engine would refuse to advance anyway.
func (m *viewer) setFonts() {
	changed, err := m.eng.SetFonts(m.o.font, m.o.mono)
	if err != nil {
		m.err = err
		return
	}
	if changed {
		m.lay = nil
	}
}

// redraw rasters the view for the picture's current geometry. Nothing is
// drawn until the picture knows its device pixels.
func (m *viewer) redraw() tea.Cmd {
	g := m.pic.RasterGeometry()
	if g.Pixels.X <= 0 || g.Pixels.Y <= 0 {
		return nil
	}
	m.geom = g
	width := max(1, m.o.width)
	scale := float64(g.Pixels.X) / float64(width)
	m.height = max(1, int(float64(g.Pixels.Y)/scale))
	if m.lay == nil {
		lay, err := m.eng.Layout(m.page.doc, width, m.page.opts)
		if err != nil {
			m.err = err
			return nil
		}
		m.lay = lay
	}
	if err := m.lay.Ensure(m.ctx, m.top+m.height); err != nil {
		m.err = err
		return nil
	}
	if m.lay.Done() {
		m.top = min(m.top, max(0, m.lay.Extent()-m.height))
	}
	// The selection's ends are placed in the text once it lands on some, so
	// they keep through scrolling, a new look and a new layout.
	var shown *nttypeset.Selection
	if m.sel != nil {
		selected, err := m.lay.Select(m.ctx, *m.sel)
		if err != nil {
			m.err = err
			return nil
		}
		if selected != nil {
			places := selected.Places()
			m.sel, m.selected = &places, selected.Text()
			shown = m.sel
		} else {
			m.selected = ""
			if !m.dragging {
				m.sel = nil
			}
		}
	}
	v := nttypeset.View{Top: m.top, Width: width, Height: m.height, Scale: scale, Pixels: g.Pixels}
	img, err := m.page.render(m.ctx, m.lay, v, m.o.dark, shown, m.hover)
	if err != nil {
		m.err = err
		return nil
	}
	m.err = nil
	m.view, m.last = v, img
	return m.pic.SetImageFor(img, g)
}

func (m *viewer) View() tea.View {
	palette := "light"
	if m.o.dark {
		palette = "dark"
	}
	mode := "glyph"
	if m.pic.Mode() == nt.PictureKitty {
		mode = "kitty"
	}
	status := fmt.Sprintf("%s · %s%s · %s · %s · top %d", m.page.title, m.o.font, suffix(m.o.mono), palette, mode, m.top)
	switch {
	case m.note != "":
		status += " · " + m.note
	case m.selected != "":
		status += fmt.Sprintf(" · %d selected · y copies", utf8.RuneCountInString(m.selected))
	}
	if m.err != nil {
		status += " · " + m.err.Error()
	}
	if m.width > 0 {
		status = ansi.Truncate(status, m.width, "…")
	}
	v := tea.NewView(m.pic.View() + "\n" + status + "\n" + m.HelpKeys().View(m.width))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeAllMotion // Motion with no button held, for the link under the pointer.
	if m.quitting {
		v.MouseMode = tea.MouseModeNone // See mouseSettle.
	}
	return v
}

func suffix(mono string) string {
	if mono == "" {
		return ""
	}
	return "+" + mono
}
