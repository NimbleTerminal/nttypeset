//go:build !js

package main

import (
	"os/exec"
	"runtime"

	tea "charm.land/bubbletea/v2"
)

func openLink(raw string) tea.Cmd {
	cmd, err := openerCommand(raw)
	if err != nil {
		return func() tea.Msg { return openedMsg{err: err} }
	}
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return openedMsg{err: err} })
}

func openerCommand(raw string) (*exec.Cmd, error) {
	if err := validLink(raw); err != nil {
		return nil, err
	}
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", raw), nil
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", raw), nil
	default:
		return exec.Command("xdg-open", raw), nil
	}
}
