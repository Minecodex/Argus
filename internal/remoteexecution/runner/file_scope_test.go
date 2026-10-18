package runner

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"google.golang.org/protobuf/proto"
)

func TestSeparateCommandsRetainExplicitEvidencePathsNotOutputDirectory(t *testing.T) {
	manager := New(t.TempDir())
	defer manager.Close()
	first := testBinding(t)
	if _, err := manager.Open(t.Context(), first); err != nil {
		t.Fatal(err)
	}
	scriptFor(first, `$evidenceFile=Join-Path $env:ARGUS_OUTPUT_DIR 'before.txt'; [IO.File]::WriteAllText($evidenceFile,'original')`, `evidence_file="$ARGUS_OUTPUT_DIR/before.txt"; printf original > "$evidence_file"`)
	result, err := manager.Execute(t.Context(), first)
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("create evidence: %v %#v", err, result)
	}
	second := proto.Clone(first).(*connectorv1.RemoteExecutionBatch)
	second.RequestId = uuid.NewString()
	second.ContextRevision = 1
	if _, err = manager.Open(t.Context(), second); err != nil {
		t.Fatal(err)
	}
	scriptFor(second, `if (Test-Path (Join-Path $env:ARGUS_OUTPUT_DIR 'before.txt')) { throw 'output scope reused' }; Get-Content $evidenceFile; Copy-Item $evidenceFile (Join-Path $env:ARGUS_OUTPUT_DIR 'comparison.txt')`, `test ! -e "$ARGUS_OUTPUT_DIR/before.txt" && cat "$evidence_file" && cp "$evidence_file" "$ARGUS_OUTPUT_DIR/comparison.txt"`)
	result, err = manager.Execute(t.Context(), second)
	if err != nil || result.ExitCode != 0 || result.SessionLost || !strings.Contains(string(result.Stdout), "original") {
		t.Fatalf("shared evidence reference: %v %#v", err, result)
	}
	files, err := manager.File(t.Context(), &connectorv1.RemoteExecutionFile{SessionId: second.SessionId, SessionGeneration: second.SessionGeneration, RequestId: second.RequestId, Operation: "list_output"})
	if err != nil {
		t.Fatalf("new command output collection: %v %#v", err, files)
	}
	found := false
	for _, file := range files.Files {
		if file.Path == "before.txt" {
			t.Fatal("previous output was implicitly merged")
		}
		if file.Path == "comparison.txt" && file.ByteSize == 8 {
			found = true
		}
	}
	if !found {
		t.Fatal("comparison file was not collected")
	}
}
