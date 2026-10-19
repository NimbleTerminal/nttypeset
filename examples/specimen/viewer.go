package main

import (
	"context"
	"fmt"
	"image"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	nt "nimbleterminal.dev/ntpicture"
	"nimbleterminal.dev/nttypeset/examples/internal/demorun"
)

// viewer is the terminal program: the specimen in an ntpicture Picture,
// rastered at the picture's own device pixels (clamped to what Preview
// accepts) so the terminal copies the raster rather than resampling it.
type viewer struct {
	ctx   context.Context
	pic   *nt.Picture
	dark  bool
	mono  string
	width int               // the terminal's width, in columns
	geom  nt.RasterGeometry // what the shown raster was made for
	err   error
}

// bindings are the viewer's keys by name.
type bindings struct {
	dark, mono, toggle, keep, quit key.Binding
}

// newBindings makes the bindings; the Kitty toggle is enabled only where
// Kitty graphics are supported.
func newBindings(kitty bool) bindings {
	b := bindings{
		dark:   demorun.Binding("Dark / Light", "d"),
		mono:   demorun.Binding("Mono", "m"),
		toggle: demorun.Binding("Glyph / Kitty", "t"),
		keep:   demorun.Binding("Keep screen & quit", "Q"),
		quit:   demorun.Binding("Quit", "q", "ctrl+c"),
	}
	b.toggle.SetEnabled(kitty)
	return b
}

// keys flattens the bindings into the primary buttons and the rest.
func (b bindings) keys() demorun.Keys {
	return demorun.Keys{
		Primary: []key.Binding{b.dark, b.mono, b.toggle},
		More:    []key.Binding{b.keep, b.quit},
	}
}

func show(ctx context.Context, o options) error {
	pic := nt.New(nt.Options{AutoMode: true})
	defer func() { _ = pic.Close() }()
	return demorun.Run(&viewer{ctx: ctx, pic: pic, dark: o.dark, mono: o.mono}, "specimen", "Font specimen")
}

// bindings returns the current bindings.
func (m *viewer) bindings() bindings {
	return newBindings(m.pic.KittySupported() == nt.KittyCapabilitySupported)
}

// HelpKeys lists the viewer's bindings: the help row and the browser buttons
// both come from here.
func (m *viewer) HelpKeys() demorun.Keys { return m.bindings().keys() }

func (m *viewer) Init() tea.Cmd { return m.pic.Init() }

func (m *viewer) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.key(msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		cmd := m.pic.Update(msg)
		return m, tea.Batch(cmd, m.pic.SetSize(max(0, msg.Width), max(0, msg.Height-1)), m.redraw())
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
	case key.Matches(msg, b.dark):
		m.dark = !m.dark
	case key.Matches(msg, b.mono):
		if m.mono == "" {
			m.mono = "jetbrains"
		} else {
			m.mono = ""
		}
	case key.Matches(msg, b.toggle):
		if c := m.pic.ToggleMode(); c != nil {
			return m, tea.Batch(c, m.redraw())
		}
	default:
		return m, nil
	}
	return m, m.redraw()
}

// redraw rasters the specimen for the picture's current geometry. Nothing is
// drawn until the picture knows its device pixels. When the picture is
// outside what Preview accepts the specimen is made at the clamped size and
// the terminal scales it.
func (m *viewer) redraw() tea.Cmd {
	g := m.pic.RasterGeometry()
	if g.Pixels.X <= 0 || g.Pixels.Y <= 0 {
		return nil
	}
	m.geom = g
	w, h := fit(g.Pixels)
	img, err := render(m.ctx, m.mono, m.dark, w, h)
	if err != nil {
		m.err = err
		return nil
	}
	m.err = nil
	if image.Pt(w, h) == g.Pixels {
		return m.pic.SetImageFor(img, g)
	}
	return m.pic.SetImage(img)
}

func (m *viewer) View() tea.View {
	palette := "light"
	if m.dark {
		palette = "dark"
	}
	mono := ""
	if m.mono != "" {
		mono = " · +" + m.mono
	}
	mode := "glyph"
	if m.pic.Mode() == nt.PictureKitty {
		mode = "kitty"
	}
	status := fmt.Sprintf("specimen · %s%s · %s", palette, mono, mode)
	if m.err != nil {
		status += " · " + m.err.Error()
	}
	row := status
	if w := m.width - ansi.StringWidth(status) - 2; w >= 1 {
		row += "  " + m.HelpKeys().View(w)
	}
	v := tea.NewView(m.pic.View() + "\n" + row)
	v.AltScreen = true
	return v
}
