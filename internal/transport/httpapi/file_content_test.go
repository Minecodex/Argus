package httpapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFileContentRangeAndPrivateDisposition(t *testing.T) {
	for _, test := range []struct {
		value, body string
		status      int
	}{
		{"bytes=2-4", "cde", 206}, {"bytes=-2", "ef", 206}, {"bytes=4-", "ef", 206},
		{"bytes=0-999", "abcdef", 206}, {"bytes=6-", "", 416}, {"bytes=4-2", "", 416}, {"bytes=0-1,3-4", "", 416}, {"bytes=+1-3", "", 416},
	} {
		t.Run(test.value, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := fileContentResponse{reader: io.NopCloser(strings.NewReader("abcdef")), size: 6, name: "业务数据.csv", requestedRange: &test.value, hash: strings.Repeat("a", 64)}
			if err := r.write(w); err != nil {
				t.Fatal(err)
			}
			if w.Code != test.status || w.Body.String() != test.body {
				t.Fatalf("response %d %q", w.Code, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "private, no-store" || !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment;") {
				t.Fatal("unsafe file response")
			}
		})
	}
}

type unreadBody struct{ read bool }

func (body *unreadBody) Read([]byte) (int, error) { body.read = true; return 0, io.EOF }
func (body *unreadBody) Close() error             { return nil }
func TestWorkspaceBinaryValidationDoesNotReadContent(t *testing.T) {
	for _, media := range []string{"application/octet-stream", "application/json"} {
		body := &unreadBody{}
		request := httptest.NewRequest(http.MethodPut, "/api/v1/conversations/11111111-1111-4111-8111-111111111111/workspace/uploads/22222222-2222-4222-8222-222222222222/content", body)
		request.Header.Set("Content-Type", media)
		request.Header.Set("X-CSRF-Token", strings.Repeat("a",64))
		reached := false
		w := httptest.NewRecorder()
		openAPIRequestValidationMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true })).ServeHTTP(w, request)
		if body.read {
			t.Fatal("binary validation buffered user data")
		}
		if reached != (media == "application/octet-stream") {
			t.Fatalf("validation reached=%v status=%d body=%s", reached, w.Code, w.Body.String())
		}
	}
}
