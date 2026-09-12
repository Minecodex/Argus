package connector

import (
	"io"
)

type localTerminal interface {
	io.ReadWriteCloser
	Resize(cols, rows uint32) error
}
