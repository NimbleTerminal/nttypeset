package main

import "testing"

func TestValidLink(t *testing.T) {
	for _, raw := range []string{"https://example.com/story", "http://example.com/", "mailto:reader@example.com"} {
		if err := validLink(raw); err != nil {
			t.Fatal(err)
		}
	}
	for _, raw := range []string{"javascript:alert(1)", "file:///tmp/story", "/relative", "://bad"} {
		if err := validLink(raw); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
}
