// argus-e2e-status is a disposable acceptance fixture, not a product service.
package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:18431", "owned loopback listener")
	file := flag.String("status-file", "", "absolute owned fixture status.conf path")
	flag.Parse()
	host, _, err := net.SplitHostPort(*listen)
	ip := net.ParseIP(host)
	if err != nil || ip == nil || !ip.IsLoopback() || !filepath.IsAbs(*file) || filepath.Base(*file) != "status.conf" {
		fmt.Fprintln(os.Stderr, "requires a loopback listener and absolute fixture status.conf")
		os.Exit(2)
	}
	server := &http.Server{Addr: *listen, Handler: statusHandler(*file), ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 5 * time.Second}
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, "owned status fixture could not serve requests")
		os.Exit(1)
	}
}

func statusHandler(file string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		body, err := os.ReadFile(file)
		if err != nil {
			http.Error(w, "fixture status unavailable", http.StatusInternalServerError)
			return
		}
		state := strings.TrimSpace(string(body))
		code := http.StatusInternalServerError
		switch state {
		case "status=broken":
			code = http.StatusServiceUnavailable
		case "status=healthy":
			code = http.StatusOK
		default:
			http.Error(w, "fixture status unavailable", code)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(code)
		_, _ = w.Write([]byte(state))
	})
}
