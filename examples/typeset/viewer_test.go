package main

import (
	"strings"
	"testing"

	"github.com/NimbleMarkets/go-booba/actions/bubblehelp"
	nt "nimbleterminal.dev/ntpicture"
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
	for _, want := range []string{"Down", "Up", "Page down", "Page up", "Top", "End", "Dark / Light", "Next look", "Mono", "Line numbers", "Chop lines", "Keep screen & quit", "Quit"} {
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
	if err := demorun.Check(stubModel{m, keys(true)}, "typeset", "Typeset a page"); err != nil {
		t.Fatal(err)
	}
}

// stubModel is a viewer whose help is fixed.
type stubModel struct {
	*viewer
	k demorun.Keys
}

func (s stubModel) HelpKeys() demorun.Keys { return s.k }
