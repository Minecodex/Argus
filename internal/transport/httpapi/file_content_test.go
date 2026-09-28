package httpapi

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type leaseFileReader struct {
	io.Reader
	closed  bool
	closes  int
	failure error
}

func (r *leaseFileReader) Close() error { r.closed = true; r.closes++; return r.failure }

type completionWriter struct {
	*httptest.ResponseRecorder
	reader               *leaseFileReader
	expected             int
	streamedWhileOpen    bool
	completedBeforeClose bool
}

func (w *completionWriter) WriteHeader(code int) {
	if w.expected == 0 && !w.reader.closed {
		w.completedBeforeClose = true
	}
	w.ResponseRecorder.WriteHeader(code)
}
func (w *completionWriter) Write(p []byte) (int, error) {
	if len(p) > 0 && !w.reader.closed {
		w.streamedWhileOpen = true
	}
	if w.Body.Len()+len(p) >= w.expected && !w.reader.closed {
		w.completedBeforeClose = true
	}
	return w.ResponseRecorder.Write(p)
}

func TestDownloadCompletesOnlyAfterLeaseReleaseAndStillStreams(t *testing.T) {
	for _, size := range []int{0, 10, 128 << 10} {
		reader := &leaseFileReader{Reader: strings.NewReader(strings.Repeat("x", size))}
		w := &completionWriter{ResponseRecorder: httptest.NewRecorder(), reader: reader, expected: size}
		if err := (fileContentResponse{reader: reader, size: int64(size), name: "query.json"}).write(w); err != nil {
			t.Fatal(err)
		}
		if w.completedBeforeClose || reader.closes != 1 || w.Body.Len() != size {
			t.Fatalf("file completed before lease release: size=%d closes=%d", size, reader.closes)
		}
		if size > 64<<10 && !w.streamedWhileOpen {
			t.Fatal("large download was buffered instead of streamed")
		}
	}
}

func TestDownloadCleanupFailureDoesNotCompleteTheFile(t *testing.T) {
	for _, size := range []int{10, 128 << 10} {
		failure := errors.New("lease release failed")
		reader := &leaseFileReader{Reader: strings.NewReader(strings.Repeat("x", size)), failure: failure}
		w := httptest.NewRecorder()
		err := (fileContentResponse{reader: reader, size: int64(size), name: "query.json"}).write(w)
		if !errors.Is(err, failure) || reader.closes != 1 || w.Body.Len() >= size {
			t.Fatal("cleanup failure was hidden by a complete HTTP body")
		}
		if size < 32<<10 && w.Header().Get("Content-Length") != "" {
			t.Fatal("uncommitted error response retained a successful file length")
		}
	}
}

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
		request.Header.Set("X-CSRF-Token", strings.Repeat("a", 64))
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
