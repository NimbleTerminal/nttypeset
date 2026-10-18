// Package testfonts is a FaceSource over the Go fonts in golang.org/x/image,
// for tests that need real faces without shipping font files.
//
// It does not import nttypeset, so the nttypeset package's own tests can use
// it. Build the look with FontLook{Name: Name, Body: Body, Code: Code}.
package testfonts

import (
	"fmt"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/gomonobolditalic"
	"golang.org/x/image/font/gofont/gomonoitalic"
	"golang.org/x/image/font/gofont/goregular"
)

// The test look: proportional Go for the body, Go Mono for code.
const (
	// Name is the look name.
	Name = "go"
	// Body is the proportional family.
	Body = "Go"
	// Code is the monospace family.
	Code = "Go Mono"
)

// Source supplies the Go fonts for the families "Go" and "Go Mono". Any other
// family, or any other variant, is an error.
var Source source

type source struct{}

// Face returns the TrueType bytes for family and variant.
func (source) Face(family, variant string) ([]byte, error) {
	var set map[string][]byte
	switch family {
	case Body:
		set = map[string][]byte{
			"Regular": goregular.TTF, "Bold": gobold.TTF,
			"Italic": goitalic.TTF, "BoldItalic": gobolditalic.TTF,
		}
	case Code:
		set = map[string][]byte{
			"Regular": gomono.TTF, "Bold": gomonobold.TTF,
			"Italic": gomonoitalic.TTF, "BoldItalic": gomonobolditalic.TTF,
		}
	default:
		return nil, fmt.Errorf("testfonts: no family %q", family)
	}
	data, ok := set[variant]
	if !ok {
		return nil, fmt.Errorf("testfonts: no variant %q of %q", variant, family)
	}
	return data, nil
}
