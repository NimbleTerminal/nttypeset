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

// HelpKeys lists the viewer's bindings: the help row and the browser buttons
// both come from here. The Kitty toggle is offered only where Kitty graphics
// are supported.
func (m *viewer) HelpKeys() demorun.Keys {
	return keys(m.pic.KittySupported() == nt.KittyCapabilitySupported)
}

// keys is HelpKeys with the toggle's availability given.
func keys(kitty bool) demorun.Keys {
	toggle := demorun.Binding("Glyph / Kitty", "t")
	toggle.SetEnabled(kitty)
	return demorun.Keys{
		Primary: []key.Binding{
			demorun.Binding("Down", "j", "down"),
			demorun.Binding("Up", "k", "up"),
			demorun.Binding("Page down", "space", "pgdown"),
			demorun.Binding("Page up", "pgup"),
			demorun.Binding("Top", "g", "home"),
			demorun.Binding("End", "G", "end"),
			demorun.Binding("Dark / Light", "d"),
			demorun.Binding("Next look", "f"),
			demorun.Binding("Mono", "m"),
			toggle,
		},
		More: []key.Binding{
			demorun.Binding("Line numbers", "n"),
			demorun.Binding("Chop lines", "c"),
			demorun.Binding("Keep screen & quit", "Q"),
			demorun.Binding("Quit", "q", "ctrl+c"),
		},
	}
}

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
	k := m.HelpKeys()
	down, up, pgdown, pgup, top, end := k.Primary[0], k.Primary[1], k.Primary[2], k.Primary[3], k.Primary[4], k.Primary[5]
	dark, look, mono, toggle := k.Primary[6], k.Primary[7], k.Primary[8], k.Primary[9]
	numbers, chop, keep, quit := k.More[0], k.More[1], k.More[2], k.More[3]
	switch {
	case key.Matches(msg, quit):
		return m, tea.Sequence(m.pic.Close(), tea.Quit)
	case key.Matches(msg, keep):
		return m, tea.Sequence(m.pic.Keep(), tea.Quit)
	case key.Matches(msg, down):
		m.top += scrollStep
	case key.Matches(msg, up):
		m.top = max(0, m.top-scrollStep)
	case key.Matches(msg, pgdown):
		m.top += m.height
	case key.Matches(msg, pgup):
		m.top = max(0, m.top-m.height)
	case key.Matches(msg, top):
		m.top = 0
	case key.Matches(msg, end):
		if m.lay != nil {
			if m.err = m.lay.Complete(m.ctx); m.err == nil {
				m.top = max(0, m.lay.Extent()-m.height)
			}
		}
	case key.Matches(msg, dark):
		m.o.dark = !m.o.dark
	case key.Matches(msg, look):
		m.nextLook()
	case key.Matches(msg, mono):
		if m.o.mono == "" {
			m.o.mono = "jetbrains"
		} else {
			m.o.mono = ""
		}
		m.setFonts()
	case key.Matches(msg, toggle):
		if c := m.pic.ToggleMode(); c != nil {
			return m, tea.Batch(c, m.redraw())
		}
	case key.Matches(msg, numbers):
		m.page.opts.Numbers = !m.page.opts.Numbers
		m.lay = nil
	case key.Matches(msg, chop):
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
	img, err := m.page.render(m.ctx, m.lay, v, m.o.dark)
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
