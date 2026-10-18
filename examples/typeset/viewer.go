package main

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
	nt "nimbleterminal.dev/ntpicture"
	"nimbleterminal.dev/nttypeset"
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
	geom   nt.RasterGeometry // what the shown raster was made for
	err    error
}

func show(ctx context.Context, p *page, eng *nttypeset.Engine, o options) error {
	pic := nt.New(nt.Options{AutoMode: true})
	defer func() { _ = pic.Close() }()
	_, err := tea.NewProgram(&viewer{ctx: ctx, page: p, eng: eng, pic: pic, o: o}).Run()
	return err
}

func (m *viewer) Init() tea.Cmd { return m.pic.Init() }

func (m *viewer) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.key(msg.String())
	case tea.WindowSizeMsg:
		cmd := m.pic.Update(msg)
		return m, tea.Batch(cmd, m.pic.SetSize(max(0, msg.Width), max(0, msg.Height-1)), m.redraw())
	}
	cmd := m.pic.Update(msg)
	if m.pic.RasterGeometry() != m.geom {
		return m, tea.Batch(cmd, m.redraw())
	}
	return m, cmd
}

func (m *viewer) key(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "q", "ctrl+c":
		return m, tea.Sequence(m.pic.Close(), tea.Quit)
	case "Q":
		return m, tea.Sequence(m.pic.Keep(), tea.Quit)
	case "j", "down":
		m.top += scrollStep
	case "k", "up":
		m.top = max(0, m.top-scrollStep)
	case "space", "pgdown":
		m.top += m.height
	case "pgup":
		m.top = max(0, m.top-m.height)
	case "g", "home":
		m.top = 0
	case "G", "end":
		if m.lay != nil {
			if m.err = m.lay.Complete(m.ctx); m.err == nil {
				m.top = max(0, m.lay.Extent()-m.height)
			}
		}
	case "d":
		m.o.dark = !m.o.dark
	case "f":
		m.nextLook()
	case "m":
		if m.o.mono == "" {
			m.o.mono = "jetbrains"
		} else {
			m.o.mono = ""
		}
		m.setFonts()
	case "n":
		m.page.opts.Numbers = !m.page.opts.Numbers
		m.lay = nil
	case "c":
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
	status := fmt.Sprintf("%s · %s%s · %s · top %d", m.page.title, m.o.font, suffix(m.o.mono), palette, m.top)
	if m.err != nil {
		status += " · " + m.err.Error()
	}
	status += "   j/k scroll  space page  g/G ends  d dark  f font  m mono  n numbers  c chop  q quit  Q keep"
	v := tea.NewView(m.pic.View() + "\n" + status)
	v.AltScreen = true
	return v
}

func suffix(mono string) string {
	if mono == "" {
		return ""
	}
	return "+" + mono
}
