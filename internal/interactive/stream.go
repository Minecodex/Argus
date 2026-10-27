// Package interactive contains transport frames shared by independently
// authorized host and Kubernetes interactive sessions.
package interactive

import "context"

type Input struct {
	Protocol string `json:"protocol"`
	Type     string `json:"type"`
	Sequence uint64 `json:"sequence"`
	Ticket   string `json:"ticket,omitempty"`
	Nonce    string `json:"nonce,omitempty"`
	Cols     int    `json:"cols,omitempty"`
	Rows     int    `json:"rows,omitempty"`
	Data     string `json:"data,omitempty"`
	Reason   string `json:"reason,omitempty"`
}
type Frame struct {
	Type, Stream   string
	Data           []byte
	Status, Reason string
}
type Session interface {
	Send(context.Context, Input) error
	Receive(context.Context) (Frame, error)
	Close(context.Context, string) error
}
