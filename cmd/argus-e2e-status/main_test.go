package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestStatusFixtureReadsActualFileOnEveryRequest(t *testing.T) {
	file := filepath.Join(t.TempDir(), "status.conf")
	handler := statusHandler(file)
	for _, tc := range []struct {
		body string
		code int
	}{{"status=broken\n", 503}, {"status=healthy\n", 200}, {"other-value", 500}, {"status=broken", 503}} {
		if err := os.WriteFile(file, []byte(tc.body), 0600); err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
		if response.Code != tc.code {
			t.Fatalf("actual state %q returned %d, expected%d", tc.body, response.Code, tc.code)
		}
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != 500 {
		t.Fatal("missing file became a healthy or invented observation")
	}
}
