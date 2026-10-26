//go:build !js

package main

import (
	"runtime"
	"strings"
	"testing"
)

func TestOpenerCommand(t *testing.T) {
	cmd, err := openerCommand("https://example.com/a?b=c")
	if err != nil {
		t.Fatal(err)
	}
	want := "xdg-open"
	switch runtime.GOOS {
	case "darwin":
		want = "open"
	case "windows":
		want = "rundll32"
	}
	if cmd.Path != want && !strings.HasSuffix(cmd.Path, "/"+want) {
		t.Fatalf("opener = %q, want %s", cmd.Path, want)
	}
	if cmd.Args[len(cmd.Args)-1] != "https://example.com/a?b=c" {
		t.Fatalf("args = %v", cmd.Args)
	}
	for _, raw := range []string{"javascript:alert(1)", "file:///etc/passwd", "://bad"} {
		if _, err := openerCommand(raw); err == nil {
			t.Errorf("%q: no error", raw)
		}
	}
}
