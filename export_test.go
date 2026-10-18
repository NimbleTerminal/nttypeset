package nttypeset

import "fmt"

// shapeForTest runs shape and recovers its panic into err.
func (e *Engine) shapeForTest(spans []Span, size int) (s *shaped, err error) {
	defer func() {
		if p := recover(); p != nil {
			if pe, ok := p.(error); ok {
				err = pe
			} else {
				err = fmt.Errorf("%v", p)
			}
		}
	}()
	return e.shape(spans, size), nil
}
