package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/uuid"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/remoteexecution/runner"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestResultsNativeProcessToWorkspaceBusinessTree(t *testing.T) {
	f := newResultsFixture(t)
	account, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	binding := &connectorv1.RemoteExecutionBatch{SessionId: uuid.NewString(), SessionGeneration: uuid.NewString(), RequestId: uuid.NewString(), IdentityBinding: uuid.NewString(), ResourceId: uuid.NewString(), ResourceType: "host", ActionRef: "act_test_authorized", ApprovedContentHash: strings.Repeat("a", 64), Username: account.Username, Interpreter: "bash", TimeoutSeconds: 30, MaxOutputBytes: 64 << 10, Sequence: 1}
	binding.Script = `mkdir -p "$ARGUS_OUTPUT_DIR/nginx/before" "$ARGUS_OUTPUT_DIR/nginx/after" && printf 'error_count=7\n' > "$ARGUS_OUTPUT_DIR/nginx/before/status.txt" && printf 'error_count=2\n' > "$ARGUS_OUTPUT_DIR/nginx/after/status.txt"`
	if runtime.GOOS == "windows" {
		binding.Interpreter = "powershell"
		binding.IdentitySid = account.Uid
		binding.Script = `[IO.Directory]::CreateDirectory((Join-Path $env:ARGUS_OUTPUT_DIR 'nginx/before')) | Out-Null; [IO.Directory]::CreateDirectory((Join-Path $env:ARGUS_OUTPUT_DIR 'nginx/after')) | Out-Null; [IO.File]::WriteAllText((Join-Path $env:ARGUS_OUTPUT_DIR 'nginx/before/status.txt'),"error_count=7` + "`n" + `"); [IO.File]::WriteAllText((Join-Path $env:ARGUS_OUTPUT_DIR 'nginx/after/status.txt'),"error_count=2` + "`n" + `")`
	}
	sum := sha256.Sum256([]byte(binding.Script))
	binding.ScriptSha256 = hex.EncodeToString(sum[:])
	manager := runner.New(t.TempDir())
	defer manager.Close()
	if _, err := manager.Open(t.Context(), binding); err != nil {
		t.Fatal(err)
	}
	receipt, err := manager.Execute(t.Context(), binding)
	if err != nil || !receipt.Terminated || receipt.ExitCode != 0 {
		t.Fatalf("native command failed: %+v %v", receipt, err)
	}
	listing, err := manager.File(t.Context(), &connectorv1.RemoteExecutionFile{SessionId: binding.SessionId, SessionGeneration: binding.SessionGeneration, RequestId: binding.RequestId, Operation: "list_output"})
	if err != nil || !listing.Complete || len(listing.Files) != 2 {
		t.Fatalf("output listing: %+v %v", listing, err)
	}
	stepID := uuid.New()
	contents := map[string]string{}
	for _, item := range listing.Files {
		chunk, err := manager.File(t.Context(), &connectorv1.RemoteExecutionFile{SessionId: binding.SessionId, SessionGeneration: binding.SessionGeneration, RequestId: binding.RequestId, Operation: "read_output", Path: item.Path, ByteSize: item.ByteSize, Sha256: item.Sha256})
		if err != nil {
			t.Fatal(err)
		}
		artifact := resultArtifact(item.Path, string(chunk.Data))
		artifact.RequestID = uuid.MustParse(binding.RequestId)
		artifact.SessionID = uuid.MustParse(binding.SessionId)
		artifact.StepID = stepID
		if artifact.ContentHash != item.Sha256 || uint64(artifact.ByteSize) != item.ByteSize {
			t.Fatal("native output integrity changed")
		}
		file, err := f.transfer(artifact, string(chunk.Data))
		if err != nil {
			t.Fatal(err)
		}
		contents[file.Path] = f.read(file.Path)
	}
	if contents["results/nginx/before/status.txt"] != "error_count=7\n" || contents["results/nginx/after/status.txt"] != "error_count=2\n" {
		t.Fatalf("business layout lost native evidence: %+v", contents)
	}
	contents["results/meta.json"] = f.read("results/meta.json")
	files, err := f.service.Store.Queries.ListWorkspaceFiles(t.Context(), db.ListWorkspaceFilesParams{WorkspaceID: f.workspace.ID, EnterpriseID: f.principal.EnterpriseID})
	if err != nil || len(files) != 3 {
		t.Fatalf("file catalog: %d %v", len(files), err)
	}
	// Optional browser fixture is derived from the actual database/RPC results.
	if directory := os.Getenv("ARGUS_RESULTS_TEST_ARTIFACTS"); directory != "" {
		if err := os.MkdirAll(directory, 0700); err != nil {
			t.Fatal(err)
		}
		entries := []map[string]any{}
		for _, file := range files {
			entries = append(entries, map[string]any{"file": fileView(file), "content": contents[file.Path]})
		}
		data, err := json.MarshalIndent(entries, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "native-results.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("native %s -> bounded output collection -> PostgreSQL mapping -> Workspace RPC -> business files and meta.json verified", binding.Interpreter)
}
