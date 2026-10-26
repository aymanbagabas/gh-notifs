//go:build !unix

package event

type resizeSource struct{}

func newResizeSource() resizeSource { return resizeSource{} }

func (resizeSource) C() <-chan struct{} { return nil }
