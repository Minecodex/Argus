package workspace

import (
	"io"
	"strings"
	"testing"
)

func TestFileClassificationSupportsUploadsAndRemoteOutputs(t *testing.T) {
	for _, prefix := range []string{"uploads/id/", "results/business/"} {
		for _, tc := range []struct{ name, body, media string }{
			{"host.log", "host healthy\n", "text/plain"},
			{"report.md", "# 调查结果\n", "text/markdown"},
			{"trace.jsonl", "{\"ok\":true}\n", "application/x-ndjson"},
			{"unknown", "", "application/octet-stream"},
			{"binary.txt", "\x00\x01\xff", "application/octet-stream"},
		} {
			var sample importMediaSample
			reader := io.TeeReader(strings.NewReader(tc.body), &sample)
			body, err := io.ReadAll(reader)
			if err != nil || string(body) != tc.body {
				t.Fatal("classification changed stream", err)
			}
			if got := importedMediaType(prefix+tc.name, sample.prefix); !strings.HasPrefix(got, tc.media) {
				t.Errorf("%s%s classified as %s", prefix, tc.name, got)
			}
		}
	}
	var sample importMediaSample
	if _, err := io.Copy(&sample, strings.NewReader(strings.Repeat("x", 8192))); err != nil || len(sample.prefix) != 512 {
		t.Fatal("classification buffered unbounded input")
	}
}
