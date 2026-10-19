// Package demorun publishes a demo's key bindings to booba hosts and runs it.
package demorun

import (
	"fmt"
	"os"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	booba "github.com/NimbleMarkets/go-booba"
	"github.com/NimbleMarkets/go-booba/actions"
	"github.com/NimbleMarkets/go-booba/actions/bubblehelp"
)

// Keys supplies the primary buttons and the expanded list of shortcuts.
type Keys struct {
	Primary []key.Binding
	More    []key.Binding
}

// ShortHelp returns the primary bindings.
func (k Keys) ShortHelp() []key.Binding { return k.Primary }

// FullHelp returns the primary bindings, then the rest.
func (k Keys) FullHelp() [][]key.Binding { return [][]key.Binding{k.Primary, k.More} }

// View renders Primary followed by More as one help row, truncated with an
// ellipsis at width.
func (k Keys) View(width int) string {
	h := help.New()
	h.SetWidth(width)
	bindings := make([]key.Binding, 0, len(k.Primary)+len(k.More))
	bindings = append(bindings, k.Primary...)
	bindings = append(bindings, k.More...)
	return h.ShortHelpView(bindings)
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

// Run advertises the model's key presses as booba actions and runs it. The
// model remains their sole handler.
func Run(m Model, context, summary string) error {
	wrapped, err := bubblehelp.NewModel(m, func(inner tea.Model) bubblehelp.Description {
		return bubblehelp.Description{
			Context: context, Keys: inner.(Model).HelpKeys(),
			Status: actions.ProgramStatus{App: "nttypeset", Summary: summary},
		}
	}, bubblehelp.ModelOptions{Adapter: bubblehelp.Options{Customize: func(_ key.Binding, a *actions.Action) error {
		first := 0
		a.Preferred = &first
		return nil
	}}, OnError: func(err error) { fmt.Fprintln(os.Stderr, err) }})
	if err != nil {
		return err
	}
	return booba.Run(wrapped)
}
