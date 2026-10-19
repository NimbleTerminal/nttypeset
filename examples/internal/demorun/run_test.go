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
	var primary []key.Binding
	labels := []string{"Down", "Up", "Page down", "Page up", "Top", "End", "Dark / Light", "Next look", "Mono", "Glyph / Kitty"}
	for i, l := range labels {
		primary = append(primary, Binding(l, string(rune('a'+i))))
	}
	k := Keys{Primary: primary, More: []key.Binding{
		Binding("Line numbers", "n"), Binding("Chop lines", "c"), Binding("Keep screen & quit", "Q"), Binding("Quit", "q", "ctrl+c"),
	}}
	full := ansi.StringWidth(k.View(1000))
	for _, width := range []int{40, 60, 80, 81, 100, 120, 200} {
		view := k.View(width)
		plain := ansi.Strip(view)
		if w := ansi.StringWidth(view); w > width {
			t.Errorf("width %d: view is %d columns: %q", width, w, plain)
		}
		if strings.Contains(view, "\n") {
			t.Errorf("width %d: newline in %q", width, plain)
		}
		if !strings.HasSuffix(plain, "q Quit") {
			t.Errorf("width %d: does not end with Quit: %q", width, plain)
		}
		if width < full && !strings.Contains(plain, "…") {
			t.Errorf("width %d: no ellipsis: %q", width, plain)
		}
		if width >= full {
			if strings.Contains(plain, "…") {
				t.Errorf("width %d: ellipsis in %q", width, plain)
			}
			for _, l := range labels {
				if !strings.Contains(plain, l) {
					t.Errorf("width %d: lacks %q in %q", width, l, plain)
				}
			}
		}
	}
	if full > 200 {
		t.Fatalf("full row is %d columns, the 200 case needs it whole", full)
	}
}

func TestKeysViewQuitAlone(t *testing.T) {
	k := Keys{Primary: []key.Binding{Binding("Down", "j")}, More: []key.Binding{Binding("Quit", "q")}}
	if got := ansi.Strip(k.View(6)); got != "q Quit" {
		t.Fatalf("%q", got)
	}
	if got := k.View(3); ansi.StringWidth(got) > 3 || !strings.Contains(got, "…") {
		t.Fatalf("%q", ansi.Strip(got))
	}
	if got := ansi.Strip(Keys{Primary: k.Primary}.View(40)); got != "j Down" {
		t.Fatalf("%q", got)
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
