// Package msg holds the body of a message, which never leaves the process in clear but through Reveal (I1).
package msg

import (
	"fmt"
	"io"
)

const redacted = "[REDACTED]"

// Body is the content of a message; every rendering of it is redacted.
type Body struct {
	b []byte
}

// NewBody wraps b, which the caller no longer touches.
func NewBody(b []byte) Body {
	return Body{b: b}
}

// Reveal returns the plaintext; each call site is an exclusion of forbidigo in .golangci.yml, with its reason.
func (b Body) Reveal() []byte {
	return b.b
}

// Format redacts every fmt verb, Body nested in a struct included.
func (Body) Format(f fmt.State, _ rune) {
	_, _ = io.WriteString(f, redacted)
}
