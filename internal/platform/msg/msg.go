// Package msg holds the body of a message, which never leaves the process in clear but through Reveal (I1).
package msg

// Body is the content of a message. It holds a func, which fmt renders as an address at any depth and with any verb,
// even reached by reflection through an unexported field; a slice or a pointer to one would print its bytes.
type Body struct {
	plain func() []byte
}

// NewBody wraps b, which the caller no longer touches.
func NewBody(b []byte) Body {
	return Body{plain: func() []byte { return b }}
}

// Reveal returns the plaintext of b; each call site is an exclusion of forbidigo in .golangci.yml, with its reason.
func Reveal(b Body) []byte {
	if b.plain == nil {
		return nil
	}
	return b.plain()
}
