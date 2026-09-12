package artifactcheck

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPCheckerUsesExactRequestPathAndReportsMissingObject(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "HEAD" || r.URL.Path != "/bucket/immutable" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	checker := &HTTPChecker{client: server.Client()}
	if err := checker.Check(context.Background(), server.URL+"/bucket/immutable"); err != nil {
		t.Fatal(err)
	}
	if err := checker.Check(context.Background(), server.URL+"/missing"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
}
