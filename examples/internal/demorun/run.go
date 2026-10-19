// Package demorun publishes a demo's key bindings to booba hosts and runs it.
package demorun

import (
	"crypto/rand"
	"fmt"
	"os"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	booba "github.com/NimbleMarkets/go-booba"
	"github.com/NimbleMarkets/go-booba/actions"
	"github.com/NimbleMarkets/go-booba/actions/bubblehelp"
	"github.com/charmbracelet/x/ansi"
)

// Keys supplies the primary buttons and the expanded list of shortcuts. The
// last More binding is Quit and is always shown in the terminal help row.
type Keys struct {
	Primary []key.Binding
	More    []key.Binding
}

// ShortHelp returns the primary bindings.
func (k Keys) ShortHelp() []key.Binding { return k.Primary }

// FullHelp returns the rest of the bindings. A host adds the primary bindings
// that no group names, so listing them again would only spend the 2048 bytes
// that booba allows a status message.
func (k Keys) FullHelp() [][]key.Binding { return [][]key.Binding{k.More} }

// View renders Primary followed by More as one help row of at most width
// columns. The rest of the row is cut with an ellipsis to make room, so the
// row always ends with Quit, the last More binding. Below the width of the
// Quit entry alone, the row is that entry cut at width.
func (k Keys) View(width int) string {
	h := help.New()
	shown := append(append([]key.Binding{}, k.Primary...), k.More...)
	var quit string
	if n := len(k.More); n > 0 {
		shown = shown[:len(shown)-1]
		quit = h.Styles.ShortKey.Inline(true).Render(k.More[n-1].Help().Key) + " " +
			h.Styles.ShortDesc.Inline(true).Render(k.More[n-1].Help().Desc)
	}
	separator := h.Styles.ShortSeparator.Inline(true).Render(h.ShortSeparator)
	row := quit
	if room := width - ansi.StringWidth(separator) - ansi.StringWidth(quit); room > 0 {
		// ShortHelpView lets an item through when it does not fit and the
		// ellipsis does not either, so cut its result too.
		h.SetWidth(room)
		if rest := ansi.Truncate(h.ShortHelpView(shown), room, h.Ellipsis); rest != "" {
			row = rest
			if quit != "" {
				row += separator + quit
			}
		}
	}
	return ansi.Truncate(row, width, h.Ellipsis)
}

// Binding matches any of shortcuts and shows the first one in help.
func Binding(label string, shortcuts ...string) key.Binding {
	return key.NewBinding(key.WithKeys(shortcuts...), key.WithHelp(shortcuts[0], label))
}

// Model is a Bubble Tea model that can list its key bindings.
type Model interface {
	tea.Model
	HelpKeys() Keys
}

// adapter shows only the first shortcut of an action that has aliases: that
// key is the one the bar's buttons send, and the model still matches every
// alias. An action with one shortcut needs no preference. Leaving the rest out
// keeps the snapshot within booba's 2048-byte status message.
var adapter = bubblehelp.Options{Customize: func(_ key.Binding, a *actions.Action) error {
	if len(a.Keys) > 1 {
		first := 0
		a.Preferred = &first
		a.Keys = a.Keys[:1]
	}
	return nil
}}

// Check reports whether the status message Run would send for m fits within
// booba's 2048 bytes. It uses an instance as long as the generated ones.
func Check(m Model, context, summary string) error {
	snapshot, _, err := bubblehelp.Snapshot(m.HelpKeys(), rand.Text(), context, 1, adapter)
	if err != nil {
		return err
	}
	_, err = actions.EncodeProgramStatus(snapshot, actions.ProgramStatus{App: "nttypeset", Summary: summary})
	return err
}

// Run advertises the model's key presses as booba actions and runs it. The
// model remains their sole handler.
func Run(m Model, context, summary string) error {
	wrapped, err := bubblehelp.NewModel(m, func(inner tea.Model) bubblehelp.Description {
		return bubblehelp.Description{
			Context: context, Keys: inner.(Model).HelpKeys(),
			Status: actions.ProgramStatus{App: "nttypeset", Summary: summary},
		}
	}, bubblehelp.ModelOptions{Adapter: adapter, OnError: func(err error) { fmt.Fprintln(os.Stderr, err) }})
	if err != nil {
		return err
	}
	return booba.Run(wrapped)
}
