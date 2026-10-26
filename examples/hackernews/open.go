package main

import (
	"fmt"
	"net/url"
)

func validLink(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "mailto") {
		return fmt.Errorf("cannot open URL %q", raw)
	}
	return nil
}
