//go:build js && wasm

package main

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"syscall/js"
)

// The page owns navigation, including a clickable fallback for popup blockers.
func openLink(raw string) tea.Cmd {
	return func() tea.Msg {
		if err := validLink(raw); err != nil {
			return openedMsg{err: err}
		}
		open := js.Global().Get("nttypesetOpenURL")
		if open.Type() != js.TypeFunction {
			return openedMsg{err: fmt.Errorf("browser link handler is unavailable")}
		}
		open.Invoke(raw)
		return openedMsg{}
	}
}
