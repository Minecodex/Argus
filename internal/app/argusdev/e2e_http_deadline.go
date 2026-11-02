package argusdev

import (
	"maps"
	"time"
)

// A slow, explicit prerequisite can use a bounded deadline without changing
// other requests or replacing the authenticated session/cookie jar.
func (h *ScenarioHTTP) withClientTimeout(name string, timeout time.Duration) *ScenarioHTTP {
	client := *h.Client(name)
	client.Timeout = timeout
	copy := *h
	copy.Clients = maps.Clone(h.Clients)
	copy.Clients[name] = &client
	return &copy
}
