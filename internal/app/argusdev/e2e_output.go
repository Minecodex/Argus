package argusdev

import "io"

// Child processes may split credential-bearing lines across arbitrary writes.
// Buffer complete lines before applying the same policy as saved diagnostics.
// Oversized lines are discarded, so an unbounded child output cannot exhaust
// memory or publish a prefix before its sensitive field has arrived.
type diagnosticStream struct {
	destination io.Writer
	line        []byte
	discard     bool
}

const diagnosticLineLimit = 64 << 10

func (w *diagnosticStream) Write(data []byte) (int, error) {
	for i, value := range data {
		if value == '\n' {
			if err := w.flush(true); err != nil {
				return i + 1, err
			}
		} else if !w.discard {
			if len(w.line) == diagnosticLineLimit {
				w.discard = true
				w.line = w.line[:0]
			} else {
				w.line = append(w.line, value)
			}
		}
	}
	return len(data), nil
}

func (w *diagnosticStream) Flush() error { return w.flush(false) }

func (w *diagnosticStream) flush(newline bool) error {
	defer func() { w.line = w.line[:0]; w.discard = false }()
	if w.destination == nil || w.discard {
		return nil
	}
	value := redactDiagnostic(w.line)
	if len(value) == 0 && len(w.line) != 0 {
		return nil
	}
	if newline {
		value = append(value, '\n')
	}
	if len(value) == 0 {
		return nil
	}
	n, err := w.destination.Write(value)
	if err == nil && n != len(value) {
		return io.ErrShortWrite
	}
	return err
}
