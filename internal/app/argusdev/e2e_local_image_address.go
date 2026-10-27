package argusdev

import (
	"net"
	"strconv"
	"strings"
)

// This is address translation only; fixture selection and cleanup ownership are
// separate concerns. Core runtime images use the same local registry transport.
func localE2ERegistryImage(image string) (string, bool) {
	registry, remainder, ok := strings.Cut(image, "/")
	if !ok || remainder == "" {
		return "", false
	}
	host, port, err := net.SplitHostPort(registry)
	if err != nil {
		return "", false
	}
	if host != "host.docker.internal" && host != "localhost" && host != "127.0.0.1" {
		return "", false
	}
	value, err := strconv.Atoi(port)
	if err != nil || value < 1 || value > 65535 {
		return "", false
	}
	return "localhost:" + port + "/" + remainder, true
}
