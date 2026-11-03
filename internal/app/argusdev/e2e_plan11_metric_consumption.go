package argusdev

import (
	"encoding/json"
	"path"
	"regexp"
	"strings"

	"github.com/kakj-go/Argus/internal/toolruntime"
)

var offlinePythonLoad = regexp.MustCompile(`(?m)^\s*[A-Za-z_][A-Za-z0-9_]*\s*=\s*json\.load\(open\(`)
var offlinePythonCommand = regexp.MustCompile(`(?:^|[;\n&])\s*python3?\s+-c\s+["']`)

// Test evidence accepts the demonstrated offline analysis route in addition to
// read. This is not the product's complete_read_verified state for arbitrary
// bash: it checks a bounded successful process receipt against downloaded bytes.
func plan11MetricOriginalConsumed(trace, filePath, expectedSHA string, expectedBytes int) bool {
	var calls []struct {
		Tool, Status, Source string
		Input                struct{ Path, Command string }
		Projection           struct {
			Partial bool
			Summary struct {
				Path, Content, Stdout string
				ByteSize              int  `json:"byte_size"`
				StartLine             int  `json:"start_line"`
				ExitCode              *int `json:"exit_code"`
				Partial               bool
			}
		}
	}
	if json.Unmarshal([]byte(trace), &calls) != nil {
		return false
	}
	for _, call := range calls {
		if call.Status != "succeeded" || call.Source != "sandbox" || call.Projection.Partial || call.Projection.Summary.Partial {
			continue
		}
		if call.Tool == "read" && strings.TrimPrefix(call.Input.Path, "/workspace/") == strings.TrimPrefix(filePath, "/workspace/") && strings.TrimPrefix(call.Projection.Summary.Path, "/workspace/") == strings.TrimPrefix(call.Input.Path, "/workspace/") && toolruntime.QueryFileReadMatches(call.Projection.Summary.Content, call.Projection.Summary.ByteSize, call.Projection.Summary.StartLine, false, expectedSHA, expectedBytes) {
			return true
		}
		if call.Tool != "bash" || call.Projection.Summary.ExitCode == nil || *call.Projection.Summary.ExitCode != 0 {
			continue
		}
		command := call.Input.Command
		// UUID and canonical directory are explicit in the demonstrated loop;
		// a checksum claim, echo or a directory listing alone is insufficient.
		if !strings.Contains(command, strings.TrimSuffix(path.Base(filePath), ".json")) || !strings.Contains(command, path.Dir(filePath)+"/") || !strings.Contains(command, "sha256sum ") || !offlinePythonCommand.MatchString(command) || !offlinePythonLoad.MatchString(command) {
			continue
		}
		if strings.Contains(command, "alias ") || strings.Contains(command, "function ") || strings.Contains(command, "PATH=") {
			continue
		}
		checksumMatched, analysisReturned := false, false
		id := strings.TrimSuffix(path.Base(filePath), ".json")
		for _, line := range strings.Split(call.Projection.Summary.Stdout, "\n") {
			fields := strings.Fields(line)
			if len(fields) == 2 && fields[0] == expectedSHA && strings.TrimPrefix(fields[1], "*") == filePath {
				checksumMatched = true
			}
			if strings.HasPrefix(line, id+" ") && (strings.Contains(line, "[") || strings.Contains(line, "{")) {
				analysisReturned = true
			}
		}
		if checksumMatched && analysisReturned {
			return true
		}
	}
	return false
}
