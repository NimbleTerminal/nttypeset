package demorun

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/go-booba/actions/bubblehelp"
	"github.com/charmbracelet/x/ansi"
)

func TestBindingUsesFirstShortcutForHelp(t *testing.T) {
	b := Binding("Down", "j", "down")
	if b.Help().Key != "j" || b.Help().Desc != "Down" || !key.Matches(tea.KeyPressMsg{Code: tea.KeyDown}, b) {
		t.Fatalf("%+v", b.Help())
	}
}

func TestKeysViewTruncates(t *testing.T) {
	k := Keys{
		Primary: []key.Binding{Binding("Down", "j"), Binding("Up", "k"), Binding("Page down", "space")},
		More:    []key.Binding{Binding("Quit", "q")},
	}
	wide := k.View(200)
	if !strings.Contains(wide, "Quit") || strings.Contains(wide, "\n") {
		t.Fatalf("wide: %q", wide)
	}
	narrow := k.View(16)
	if ansi.StringWidth(narrow) > 16 || strings.Contains(narrow, "\n") {
		t.Fatalf("narrow: %q", narrow)
	}
}

func TestKeysSnapshotHasNoDiagnostics(t *testing.T) {
	k := Keys{
		Primary: []key.Binding{Binding("Down", "j", "down")},
		More:    []key.Binding{Binding("Quit", "q", "ctrl+c")},
	}
	snapshot, diagnostics, err := bubblehelp.Snapshot(k, "nttypeset", "demo", 1, bubblehelp.Options{})
	if err != nil || len(diagnostics) != 0 {
		t.Fatal(err, diagnostics)
	}
	if len(snapshot.Actions) != 2 || snapshot.Actions[1].Label != "Quit" {
		t.Fatalf("%+v", snapshot.Actions)
	}
}
