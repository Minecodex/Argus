package argusdev

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/kakj-go/Argus/internal/telemetry/datapolicy"
)

type plan8SourceFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}
type plan8SourceSnapshot struct {
	Head              string            `json:"head"`
	Fingerprint       string            `json:"fingerprint"`
	Files             []plan8SourceFile `json:"files"`
	ReportFingerprint string            `json:"report_fingerprint,omitempty"`
	ReportFiles       []plan8SourceFile `json:"report_files,omitempty"`
}

func (a *App) plan8Snapshot(ctx context.Context) (plan8SourceSnapshot, error) {
	out := plan8SourceSnapshot{Files: []plan8SourceFile{}}
	record := func(path, hash string) {
		file := plan8SourceFile{path, hash}
		if plan8MutableStatusReport(path) {
			out.ReportFiles = append(out.ReportFiles, file)
		} else {
			out.Files = append(out.Files, file)
		}
	}
	head, err := a.runner.Output(ctx, nil, "git", "rev-parse", "HEAD")
	if err != nil {
		return out, err
	}
	out.Head = strings.TrimSpace(head)
	paths, err := a.runner.Output(ctx, nil, "git", "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return out, err
	}
	files := strings.Split(strings.TrimRight(paths, "\x00"), "\x00")
	files = append(files, "build/otelcol/dist/linux-amd64/argus-otelcol", "build/otelcol/dist/linux-arm64/argus-otelcol", "build/otelcol/dist/windows-amd64/argus-otelcol.exe")
	sort.Strings(files)
	seen := map[string]bool{}
	for _, path := range files {
		if path == "" || seen[path] || strings.HasPrefix(path, "artifacts/") || strings.Contains(filepath.ToSlash(path), "/node_modules/") {
			continue
		}
		seen[path] = true
		if filepath.IsAbs(path) || path == ".." || strings.HasPrefix(filepath.ToSlash(path), "../") {
			return out, errors.New("source manifest path escaped the checkout")
		}
		full := filepath.Join(a.root, filepath.FromSlash(path))
		info, e := os.Lstat(full)
		if os.IsNotExist(e) {
			record(path, "missing")
			continue
		}
		if e != nil {
			return out, e
		}
		if info.IsDir() {
			continue
		}
		hash := sha256.New()
		if info.Mode()&os.ModeSymlink != 0 {
			link, e := os.Readlink(full)
			if e != nil {
				return out, e
			}
			_, _ = io.WriteString(hash, link)
		} else {
			file, e := os.Open(full)
			if e != nil {
				return out, e
			}
			_, e = io.Copy(hash, file)
			closeErr := file.Close()
			if e != nil || closeErr != nil {
				return out, errors.Join(e, closeErr)
			}
		}
		record(path, hex.EncodeToString(hash.Sum(nil)))
	}
	raw, err := json.Marshal([]any{"argus.plan8_source/v2", out.Head, out.Files})
	if err != nil {
		return out, err
	}
	hash := sha256.Sum256(raw)
	out.Fingerprint = hex.EncodeToString(hash[:])
	if len(out.ReportFiles) > 0 {
		reports, err := json.Marshal([]any{out.Head, out.ReportFiles})
		if err != nil {
			return out, err
		}
		reportHash := sha256.Sum256(reports)
		out.ReportFingerprint = hex.EncodeToString(reportHash[:])
	}
	return out, nil
}

// Recording acceptance must not change the identity of the implementation it
// describes. Only these three status/navigation reports are independent. The
// original acceptance matrix, decisions, code, tests and material contracts
// remain in the strict source fingerprint; reports retain their own hashes.
func plan8MutableStatusReport(path string) bool {
	switch filepath.ToSlash(path) {
	case "docs/plan8/progress.json", "docs/plan8/57-completion-delivery.md", "docs/plan8/README.md":
		return true
	default:
		return false
	}
}

// Logs are sanitized before touching disk. Only named, actually passed Go
// tests satisfy a selected check; package success and skipped tests cannot.
type plan8EventLog struct {
	mu       sync.Mutex
	buffer   bytes.Buffer
	writer   io.Writer
	required []string
	tests    map[string]string
	err      error
}

func newPlan8EventLog(writer io.Writer, tests []string) *plan8EventLog {
	return &plan8EventLog{writer: writer, required: tests, tests: map[string]string{}}
}
func (l *plan8EventLog) Write(data []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := len(data)
	_, _ = l.buffer.Write(data)
	for {
		line, e := l.buffer.ReadBytes('\n')
		if e == io.EOF {
			_, _ = l.buffer.Write(line)
			break
		}
		if e != nil {
			return 0, e
		}
		if e = l.line(bytes.TrimSuffix(line, []byte{'\n'})); e != nil {
			l.err = e
			return 0, e
		}
	}
	if l.buffer.Len() > 16<<20 {
		l.err = errors.New("oversized test output line")
		return 0, l.err
	}
	return n, nil
}
func (l *plan8EventLog) line(raw []byte) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var event map[string]any
	if json.Unmarshal(raw, &event) != nil {
		event = map[string]any{"Action": "diagnostic", "Output": string(raw)}
	}
	if test, ok := event["Test"].(string); ok && test != "" {
		switch event["Action"] {
		case "pass":
			l.tests[test] = "passed"
		case "skip":
			l.tests[test] = "skipped"
		case "fail":
			l.tests[test] = "failed"
		}
	}
	public, _, err := datapolicy.Project(event)
	if err != nil {
		return err
	}
	line, err := json.Marshal(public)
	if err != nil {
		return err
	}
	_, err = l.writer.Write(append(line, '\n'))
	return err
}
func (l *plan8EventLog) finish() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.err != nil {
		return l.err
	}
	if l.buffer.Len() > 0 {
		raw := append([]byte{}, l.buffer.Bytes()...)
		l.buffer.Reset()
		if err := l.line(raw); err != nil {
			return err
		}
	}
	for _, test := range l.required {
		if l.tests[test] != "passed" {
			return fmt.Errorf("required named test %s was %s; absent/skip is not a pass", test, l.tests[test])
		}
	}
	return nil
}
