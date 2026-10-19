package main

import (
	"context"
	"fmt"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	nt "nimbleterminal.dev/ntpicture"
	"nimbleterminal.dev/nttypeset"
	"nimbleterminal.dev/nttypeset/examples/internal/demorun"
)

// scrollStep is one j/k press, in layout pixels.
const scrollStep = 60

// viewer is the terminal program: the page in an ntpicture Picture, rastered
// at the picture's own device pixels so the terminal copies the raster rather
// than resampling it. The reading scale follows the terminal width.
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
	err    error
}

func show(ctx context.Context, p *page, eng *nttypeset.Engine, o options) error {
	pic := nt.New(nt.Options{AutoMode: true})
	defer func() { _ = pic.Close() }()
	return demorun.Run(&viewer{ctx: ctx, page: p, eng: eng, pic: pic, o: o}, "typeset", "Typeset a page")
}

// bindings are the viewer's keys by name.
type bindings struct {
	down, up, pgdown, pgup, top, end key.Binding
	dark, look, mono, toggle         key.Binding
	numbers, chop, keep, quit        key.Binding
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
		More:    []key.Binding{b.numbers, b.chop, b.keep, b.quit},
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
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.key(msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		cmd := m.pic.Update(msg)
		return m, tea.Batch(cmd, m.pic.SetSize(max(0, msg.Width), max(0, msg.Height-2)), m.redraw())
	}
	cmd := m.pic.Update(msg)
	if m.pic.RasterGeometry() != m.geom {
		return m, tea.Batch(cmd, m.redraw())
	}
	return m, cmd
}

func (m *viewer) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	b := m.bindings()
	switch {
	case key.Matches(msg, b.quit):
		return m, tea.Sequence(m.pic.Close(), tea.Quit)
	case key.Matches(msg, b.keep):
		return m, tea.Sequence(m.pic.Keep(), tea.Quit)
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
	default:
		return m, nil
	}
	return m, m.redraw()
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
	v := nttypeset.View{Top: m.top, Width: width, Height: m.height, Scale: scale, Pixels: g.Pixels}
	img, err := m.page.render(m.ctx, m.lay, v, m.o.dark, m.page.sel)
	if err != nil {
		m.err = err
		return nil
	}
	m.err = nil
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
	if m.err != nil {
		status += " · " + m.err.Error()
	}
	v := tea.NewView(m.pic.View() + "\n" + status + "\n" + m.HelpKeys().View(m.width))
	v.AltScreen = true
	return v
}

func suffix(mono string) string {
	if mono == "" {
		return ""
	}
	return "+" + mono
}
