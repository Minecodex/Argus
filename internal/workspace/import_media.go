package workspace

import (
	"net/http"
	"path"
	"strings"
)

// Sample the bytes already being verified, including post-rename recovery.
// Classification never consumes a second input stream or buffers the whole file.
type importMediaSample struct{ prefix []byte }

func (s *importMediaSample) Write(p []byte) (int, error) {
	n := len(p)
	if remaining := 512 - len(s.prefix); remaining > 0 {
		s.prefix = append(s.prefix, p[:min(remaining, n)]...)
	}
	return n, nil
}

func importedMediaType(filePath string, prefix []byte) string {
	// Platform query artifacts have a separate, fixed JSON contract.
	if strings.HasPrefix(filePath, "queries/") {
		return "application/json"
	}
	ext := strings.ToLower(path.Ext(filePath))
	textTypes := map[string]string{
		".json": "application/json", ".jsonl": "application/x-ndjson", ".ndjson": "application/x-ndjson",
		".csv": "text/csv", ".tsv": "text/tab-separated-values", ".md": "text/markdown", ".markdown": "text/markdown",
		".txt": "text/plain", ".log": "text/plain", ".yaml": "text/yaml", ".yml": "text/yaml",
		".xml": "application/xml", ".html": "text/html", ".sh": "text/plain", ".ps1": "text/plain",
		".py": "text/plain", ".js": "text/javascript", ".ts": "text/plain", ".sql": "text/plain",
	}
	if len(prefix) == 0 {
		if media := textTypes[ext]; media != "" {
			return media
		}
		return "application/octet-stream"
	}
	detected := http.DetectContentType(prefix)
	// A text extension cannot override a binary signature or control bytes.
	if strings.HasPrefix(detected, "text/") {
		if media := textTypes[ext]; media != "" {
			return media
		}
	}
	return detected
}
