// Package runtimewire carries typed commands and credential-free Kubernetes
// proxy requests over the private SSH or pods/exec stream.
package runtimewire

import (
	"encoding/json"
	"net/http"
)

const MaxFrameBytes = 16 << 20
const MaxRequestBytes = 1 << 20
const MaxResponseBytes = 8 << 20

type Message struct {
	ID            string          `json:"id"`
	Kind          string          `json:"kind,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
	Error         string          `json:"error,omitempty"`
	ProxyRequest  *ProxyRequest   `json:"proxy_request,omitempty"`
	ProxyResponse *ProxyResponse  `json:"proxy_response,omitempty"`
}
type ProxyRequest struct {
	BatchID string      `json:"batch_id"`
	Method  string      `json:"method"`
	URI     string      `json:"uri"`
	Header  http.Header `json:"header"`
	Body    []byte      `json:"body,omitempty"`
}
type ProxyResponse struct {
	Status int         `json:"status"`
	Header http.Header `json:"header"`
	Body   []byte      `json:"body,omitempty"`
}
