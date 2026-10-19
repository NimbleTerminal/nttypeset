package main

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/go-booba/actions/bubblehelp"
	"github.com/charmbracelet/x/ansi"
	"nimbleterminal.dev/ntfonts"
	nt "nimbleterminal.dev/ntpicture"
	"nimbleterminal.dev/nttypeset"
	"nimbleterminal.dev/nttypeset/examples/internal/demorun"
)

func TestViewerKeysFollowKitty(t *testing.T) {
	m := &viewer{pic: nt.New(nt.Options{AutoMode: true}), o: options{font: "readable"}}
	// A fresh picture has not resolved Kitty support: the toggle is hidden.
	snapshot, diagnostics, err := bubblehelp.Snapshot(m.HelpKeys(), "nttypeset", "typeset", 1, bubblehelp.Options{})
	if err != nil || len(diagnostics) != 0 {
		t.Fatal(err, diagnostics)
	}
	labels := map[string]bool{}
	for _, a := range snapshot.Actions {
		labels[a.Label] = a.Enabled
	}
	for _, want := range []string{"Down", "Up", "Page down", "Page up", "Top", "End", "Dark / Light", "Next look", "Mono", "Line numbers", "Chop lines", "Copy", "Clear selection", "Keep screen & quit", "Quit"} {
		if !labels[want] {
			t.Errorf("%s not enabled in %v", want, labels)
		}
	}
	if labels["Glyph / Kitty"] {
		t.Fatal("toggle advertised without Kitty support")
	}
}

func TestViewerStatusNamesMode(t *testing.T) {
	m := &viewer{pic: nt.New(nt.Options{AutoMode: true}), page: &page{title: "sample.md"}, o: options{font: "readable", mono: "jetbrains"}, width: 200}
	v := m.View()
	for _, want := range []string{"sample.md", "readable+jetbrains", "light", "glyph", "Quit"} {
		if !strings.Contains(v.Content, want) {
			t.Errorf("view lacks %q:\n%s", want, v.Content)
		}
	}
}

// With every action advertised, the status message must still fit the 2048
// bytes booba allows.
func TestViewerKeysFitBooba(t *testing.T) {
	m := &viewer{pic: nt.New(nt.Options{AutoMode: true}), o: options{font: "readable"}}
	if err := demorun.Check(stubModel{m, newBindings(true).keys()}, "typeset", "Typeset a page"); err != nil {
		t.Fatal(err)
	}
}

// stubModel is a viewer whose help is fixed.
type stubModel struct {
	*viewer
	k demorun.Keys
}

func (s stubModel) HelpKeys() demorun.Keys { return s.k }

func press(m *viewer, r rune) tea.Cmd {
	_, cmd := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	return cmd
}

// Keys act by name: each press does what its binding says.
func TestViewerKeyDispatch(t *testing.T) {
	m := &viewer{
		pic: nt.New(nt.Options{AutoMode: true}), o: options{font: "readable"}, height: 400,
		page: &page{doc: &nttypeset.Document{}},
	}
	press(m, 'j')
	if m.top != scrollStep {
		t.Fatalf("top %d after j", m.top)
	}
	// Kitty is unsupported on a fresh picture: t does nothing.
	if cmd := press(m, 't'); cmd != nil || m.top != scrollStep {
		t.Fatalf("t: cmd %v, top %d", cmd, m.top)
	}
	press(m, 'n')
	if !m.page.opts.Numbers {
		t.Fatal("n did not turn line numbers on")
	}
	press(m, 'n')
	if m.page.opts.Numbers {
		t.Fatal("n did not turn line numbers off")
	}
	// With no selection, Esc and y do nothing, and Esc does not quit.
	if _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape}); cmd != nil || m.top != scrollStep {
		t.Fatalf("esc: cmd %v, top %d", cmd, m.top)
	}
	if cmd := press(m, 'y'); cmd != nil {
		t.Fatal("y with no selection returned a command")
	}
	if cmd := press(m, 'q'); cmd == nil {
		t.Fatal("q returned no command")
	}
}

// newTestViewer is a viewer with a real engine and the sample page at a glyph
// geometry of 100 by 38 cells, drawn once.
func newTestViewer(t *testing.T) *viewer {
	t.Helper()
	ctx := context.Background()
	eng, err := nttypeset.NewEngine(nttypeset.WithFonts(ntfonts.Embedded), nttypeset.WithLook("readable", ""))
	if err != nil {
		t.Fatal(err)
	}
	p, err := open(ctx, "", options{})
	if err != nil {
		t.Fatal(err)
	}
	pic := nt.New(nt.Options{AutoMode: true})
	t.Cleanup(func() { _ = pic.Close() })
	pic.SetSize(100, 38)
	m := &viewer{ctx: ctx, page: p, eng: eng, pic: pic, o: options{font: "readable", width: 800}, width: 100}
	m.redraw()
	if m.err != nil || m.last == nil {
		t.Fatalf("first draw: %v", m.err)
	}
	return m
}

func mouse(m *viewer, msg tea.Msg) tea.Cmd {
	_, cmd := m.Update(msg)
	return cmd
}

func click(x, y int) tea.MouseClickMsg { return tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft} }
func motion(x, y int) tea.MouseMotionMsg {
	return tea.MouseMotionMsg{X: x, Y: y, Button: tea.MouseLeft}
}
func release(x, y int) tea.MouseReleaseMsg {
	return tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft}
}

func TestViewerDragSelectsAndCopies(t *testing.T) {
	m := newTestViewer(t)
	plain := m.last // the raster drawn before any selection
	mouse(m, click(4, 1))
	mouse(m, motion(40, 20))
	if !strings.Contains(m.selected, "nttypeset") {
		t.Fatalf("drag selected %q", m.selected)
	}
	if m.sel == nil || !m.sel.Anchor.Placed || !m.sel.Focus.Placed {
		t.Fatalf("selection not placed: %+v", m.sel)
	}
	if bytes.Equal(m.last.Pix, plain.Pix) {
		t.Fatal("highlight not drawn")
	}
	grown := m.selected
	mouse(m, motion(70, 30))
	if len(m.selected) <= len(grown) {
		t.Fatalf("selection did not grow: %q then %q", grown, m.selected)
	}
	mouse(m, release(70, 30))
	if !strings.Contains(m.View().Content, "selected · y copies") {
		t.Fatalf("status lacks the selection:\n%s", m.View().Content)
	}
	before := m.selected
	press(m, 'j')
	if m.selected != before {
		t.Fatalf("scroll changed the selection %q", m.selected)
	}
	cmd := press(m, 'y')
	if cmd == nil || fmt.Sprint(cmd()) != before {
		t.Fatal("y did not copy the selection")
	}
	if !strings.Contains(m.View().Content, fmt.Sprintf("copied %d characters", utf8.RuneCountInString(before))) {
		t.Fatalf("note missing:\n%s", m.View().Content)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.sel != nil || m.selected != "" {
		t.Fatal("esc kept the selection")
	}
	if press(m, 'y') != nil {
		t.Fatal("y copied nothing")
	}
}

func TestViewerClickAndShiftDragSelectNothing(t *testing.T) {
	m := newTestViewer(t)
	mouse(m, click(4, 1))
	mouse(m, release(4, 1))
	if m.sel != nil {
		t.Fatal("click selected")
	}
	mouse(m, tea.MouseClickMsg{X: 4, Y: 1, Button: tea.MouseLeft, Mod: tea.ModShift})
	mouse(m, tea.MouseMotionMsg{X: 40, Y: 12, Button: tea.MouseLeft, Mod: tea.ModShift})
	if m.sel != nil {
		t.Fatal("shift-drag selected")
	}
	mouse(m, click(4, 38)) // the status row, just below the 38-row picture
	mouse(m, motion(40, 12))
	if m.sel != nil || m.dragging {
		t.Fatal("drag from outside the picture selected")
	}
}

func TestViewerClickClearsSelection(t *testing.T) {
	m := newTestViewer(t)
	mouse(m, click(4, 1))
	mouse(m, motion(40, 12))
	mouse(m, release(40, 12))
	if m.selected == "" {
		t.Fatal("no selection to clear")
	}
	mouse(m, click(60, 20))
	mouse(m, release(60, 20))
	if m.sel != nil || m.selected != "" {
		t.Fatal("click did not clear the selection")
	}
}

func TestViewerDragClampsAtEdge(t *testing.T) {
	m := newTestViewer(t)
	mouse(m, click(4, 1))
	mouse(m, motion(40, 12))
	inside := m.selected
	mouse(m, motion(150, 60)) // past the picture on both axes
	if !m.dragging || len(m.selected) <= len(inside) {
		t.Fatalf("drag past the edge: dragging %v, %q then %q", m.dragging, inside, m.selected)
	}
	// The point is the nearest edge pixel's, not one past it.
	want, ok := m.pageAt(99, 37, true)
	if !ok || m.dragTo != want {
		t.Fatalf("dragTo %v, want %v (ok %v)", m.dragTo, want, ok)
	}
}

func TestViewerWheelScrolls(t *testing.T) {
	m := newTestViewer(t)
	mouse(m, tea.MouseWheelMsg{X: 4, Y: 4, Button: tea.MouseWheelDown})
	if m.top != scrollStep {
		t.Fatalf("top %d after wheel down", m.top)
	}
	mouse(m, tea.MouseWheelMsg{X: 4, Y: 4, Button: tea.MouseWheelUp})
	if m.top != 0 {
		t.Fatalf("top %d after wheel up", m.top)
	}
	mouse(m, tea.MouseWheelMsg{X: 4, Y: 4, Button: tea.MouseWheelUp})
	if m.top != 0 {
		t.Fatalf("top %d after wheel up at the top", m.top)
	}
}

func TestViewerMouseIgnoredWhileStale(t *testing.T) {
	m := newTestViewer(t)
	mouse(m, click(4, 1))
	m.pic.SetSize(80, 30) // geometry changes before any redraw
	mouse(m, motion(40, 12))
	if m.dragging || m.sel != nil {
		t.Fatal("stale geometry still mapped the mouse")
	}
}

func TestViewerSelectionSurvivesLook(t *testing.T) {
	m := newTestViewer(t)
	mouse(m, click(4, 1))
	mouse(m, motion(40, 12))
	mouse(m, release(40, 12))
	before := m.selected
	press(m, 'f')
	press(m, 'd')
	if m.selected != before {
		t.Fatalf("look or palette changed the selection: %q then %q", before, m.selected)
	}
}

func TestViewerStatusFitsWidth(t *testing.T) {
	m := newTestViewer(t)
	mouse(m, click(4, 1))
	mouse(m, motion(40, 12))
	mouse(m, release(40, 12))
	m.width = 40
	rows := strings.Split(m.View().Content, "\n")
	status := rows[len(rows)-2]
	if ansi.StringWidth(status) > 40 || !strings.HasSuffix(status, "…") {
		t.Fatalf("status %q not cut to 40", status)
	}
}
