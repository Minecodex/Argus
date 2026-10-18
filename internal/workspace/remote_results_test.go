package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/config"
	workspacev1 "github.com/kakj-go/Argus/internal/gen/proto/argus/workspace/v1"
	"github.com/kakj-go/Argus/internal/remoteexecution"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"github.com/kakj-go/Argus/internal/workspacefs"
	"github.com/kakj-go/Argus/internal/workspaceio"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type resultsFixture struct {
	t         *testing.T
	service   Service
	principal toolruntime.Principal
	workspace db.Workspace
	files     *workspacefs.Files
	client    *workspaceio.Client
	owner     string
	allowed   bool
}

func newResultsFixture(t *testing.T) *resultsFixture {
	t.Helper()
	address := os.Getenv("ARGUS_P5_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires disposable migrated PostgreSQL")
	}
	store, err := postgres.Open(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	f := &resultsFixture{t: t, owner: uuid.NewString(), allowed: true}
	e, d, u, m, c, w := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(t.Context(), sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Results fixture',$2,'UTC')`, e, "results-"+e.String())
	exec(`INSERT INTO departments(id,enterprise_id,name,is_default) VALUES($1,$2,'Default',true)`, d, e)
	exec(`INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Results')`, u, e, d, "results-"+u.String())
	exec(`INSERT INTO ai_models(id,enterprise_id,name,base_url,model_id,api_protocol,context_window_tokens,max_output_tokens,input_price_per_million,output_price_per_million,health_status) VALUES($1,$2,'Results','https://model.example.test','model','chat_completions',32768,1024,0,0,'healthy')`, m, e)
	exec(`INSERT INTO conversations(id,enterprise_id,owner_user_id,title,selected_model_id) VALUES($1,$2,$3,'Results',$4)`, c, e, u, m)
	exec(`INSERT INTO workspaces(id,enterprise_id,conversation_id,pvc_name,namespace,status,capacity_bytes,environment_version,lease_owner,lease_until,fence_token) VALUES($1,$2,$3,$4,'results-fixture','ready',2147483648,'test',$5,now()+interval '10 minutes',1)`, w, e, c, "results-"+w.String(), f.owner)
	f.principal = toolruntime.Principal{EnterpriseID: e, UserID: u, ConversationID: c, AuthorizationVersion: 1, Permissions: []string{"workspace.use"}}
	f.service = Service{Store: store, Config: config.Workspace{MaxFileBytes: 1 << 20}, ExternalSource: func(_ context.Context, p toolruntime.Principal, ref string) (bool, error) {
		if !f.allowed || p.UserID != u || p.ConversationID != c || !strings.HasPrefix(ref, "remote:") {
			return true, toolruntime.Error{Kind: "WORKSPACE_FILE_FORBIDDEN"}
		}
		return true, nil
	}}
	f.workspace, err = store.Queries.GetWorkspace(t.Context(), db.GetWorkspaceParams{ID: w, EnterpriseID: e})
	if err != nil {
		t.Fatal(err)
	}
	f.files, err = workspacefs.Open(t.TempDir(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.files.Close() })
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	workspacev1.RegisterWorkspaceIOServiceServer(server, &workspaceio.Server{Files: f.files, WorkspaceID: w.String(), Fence: 1})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient("passthrough:///results-fixture", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	f.client = &workspaceio.Client{RPC: workspacev1.NewWorkspaceIOServiceClient(conn), Authority: &workspacev1.Authority{WorkspaceId: w.String(), FenceToken: 1}}
	return f
}

func (f *resultsFixture) access() *Access {
	ctx, cancel := context.WithCancel(f.t.Context())
	f.t.Cleanup(cancel)
	return &Access{Context: ctx, Workspace: f.workspace, IO: f.client, service: f.service, owner: f.owner, cancel: cancel, principal: &f.principal}
}
func resultArtifact(relative, body string) remoteexecution.OutputArtifact {
	sum := sha256.Sum256([]byte(body))
	return remoteexecution.OutputArtifact{ID: uuid.New(), RequestID: uuid.New(), RunID: uuid.New(), StepID: uuid.New(), SessionID: uuid.New(), Target: remoteexecution.Target{Name: "nginx", ResourceType: "host", ResourceID: uuid.New()}, RequestTitle: "Compare nginx status", StepDescription: "Collect current evidence", SourcePath: relative, ByteSize: int64(len(body)), ContentHash: hex.EncodeToString(sum[:]), CollectedAt: time.Now().UTC().Truncate(time.Microsecond)}
}
func (f *resultsFixture) transfer(a remoteexecution.OutputArtifact, body string) (db.WorkspaceFile, error) {
	f.t.Helper()
	return f.service.importRemoteArtifact(f.t.Context(), f.principal, a, strings.NewReader(body), f.access())
}
func (f *resultsFixture) read(name string) string {
	f.t.Helper()
	reader, _, err := f.files.Read(name)
	if err != nil {
		f.t.Fatal(err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		f.t.Fatal(err)
	}
	return string(data)
}

func TestRemoteResultsBusinessLayoutMetadataAndReplay(t *testing.T) {
	f := newResultsFixture(t)
	a := resultArtifact("nginx/before/status.txt", "status=degraded\n")
	first, err := f.transfer(a, "status=degraded\n")
	if err != nil || first.Path != "results/nginx/before/status.txt" {
		t.Fatalf("business layout: %s %v", first.Path, err)
	}
	repeated, err := f.transfer(a, "status=degraded\n")
	if err != nil || first.ID != repeated.ID {
		t.Fatalf("replay duplicated file: %v", err)
	}
	b := resultArtifact("nginx/after/status.txt", "status=healthy\n")
	if _, err := f.transfer(b, "status=healthy\n"); err != nil {
		t.Fatal(err)
	}
	var meta resultsMetadata
	if err := json.Unmarshal([]byte(f.read(remoteexecution.ResultsMetadataPath)), &meta); err != nil {
		t.Fatal(err)
	}
	if len(meta.Files) != 2 || meta.Files[0].RequestID != a.RequestID || meta.Files[0].StepID != a.StepID || meta.Files[0].WorkspaceFileID != first.ID || meta.Files[0].Path != "/workspace/results/nginx/before/status.txt" {
		t.Fatalf("metadata lost provenance: %+v", meta)
	}
	if strings.Contains(first.Path, a.RequestID.String()) || strings.Contains(first.Path, a.StepID.String()) {
		t.Fatal("UUID directories leaked into business layout")
	}
	files, err := f.service.Store.Queries.ListWorkspaceFiles(t.Context(), db.ListWorkspaceFilesParams{WorkspaceID: f.workspace.ID, EnterpriseID: f.principal.EnterpriseID})
	if err != nil || len(files) != 3 {
		t.Fatalf("expected two business files and one metadata file: %d %v", len(files), err)
	}
	forged := `{"files":[{"request_id":"forged"}]}`
	if _, err := f.files.AtomicWrite(t.Context(), remoteexecution.ResultsMetadataPath, strings.NewReader(forged), int64(len(forged)), true); err != nil {
		t.Fatal(err)
	}
	if _, err := f.transfer(a, "status=degraded\n"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(f.read(remoteexecution.ResultsMetadataPath), "forged") {
		t.Fatal("editable metadata became provenance authority")
	}
	if got := f.read("results/nginx/after/status.txt"); got != "status=healthy\n" {
		t.Fatal(got)
	}
}

func TestRemoteResultsPreserveNameCollisionsAndReservedMetadata(t *testing.T) {
	f := newResultsFixture(t)
	a := resultArtifact("nginx/status.txt", "before")
	if _, err := f.transfer(a, "before"); err != nil {
		t.Fatal(err)
	}
	b := resultArtifact("nginx/status.txt", "after")
	renamed, err := f.transfer(b, "after")
	if err != nil || renamed.Path != "results/nginx/status (2).txt" {
		t.Fatalf("collision: %s %v", renamed.Path, err)
	}
	replay, err := f.transfer(b, "after")
	if err != nil || replay.ID != renamed.ID {
		t.Fatalf("renamed replay: %v", err)
	}
	if f.read("results/nginx/status.txt") != "before" {
		t.Fatal("old evidence overwritten")
	}
	c := resultArtifact("meta.json", "{\"business\":true}")
	imported, err := f.transfer(c, "{\"business\":true}")
	if err != nil || imported.Path != "results/meta (2).json" {
		t.Fatalf("reserved metadata path: %s %v", imported.Path, err)
	}
	if _, err := f.files.AtomicWrite(t.Context(), "results/user.txt", strings.NewReader("user content"), 12, false); err != nil {
		t.Fatal(err)
	}
	d := resultArtifact("user.txt", "collected")
	preserved, err := f.transfer(d, "collected")
	if err != nil || preserved.Path != "results/user (2).txt" || f.read("results/user.txt") != "user content" {
		t.Fatalf("unregistered user copy lost: %s %v", preserved.Path, err)
	}
}

func TestRemoteResultsRecoverReservationButNeverOverwriteEditedEvidence(t *testing.T) {
	f := newResultsFixture(t)
	a := resultArtifact("nginx/error.log", "first")
	access := f.access()
	reserved, err := f.service.reserveResultArtifact(t.Context(), f.principal, a, access)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a crash after atomic rename but before file metadata commits.
	if _, err := f.files.AtomicWrite(t.Context(), reserved.Path, strings.NewReader("first"), 5, false); err != nil {
		t.Fatal(err)
	}
	file, err := f.transfer(a, "first")
	if err != nil || file.Path != reserved.Path {
		t.Fatalf("crash recovery: %v", err)
	}
	if _, err := f.files.AtomicWrite(t.Context(), file.Path, strings.NewReader("user edit"), 9, true); err != nil {
		t.Fatal(err)
	}
	if _, err := f.transfer(a, "first"); err == nil {
		t.Fatal("retry overwrote a user edit")
	}
	if f.read(file.Path) != "user edit" {
		t.Fatal("user bytes were lost")
	}
	altered := a
	altered.ContentHash = strings.Repeat("0", 64)
	if _, err := f.transfer(altered, "first"); err == nil {
		t.Fatal("artifact identity accepted different content")
	}
}

func TestRemoteResultsRejectCorruptionAndRevocation(t *testing.T) {
	f := newResultsFixture(t)
	a := resultArtifact("nginx/status.txt", "verified")
	if _, err := f.transfer(a, "corrupt!"); err == nil {
		t.Fatal("corrupted result published")
	}
	if _, _, err := f.files.Read("results/nginx/status.txt"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("unverified file appeared")
	}
	f.allowed = false
	if _, err := f.transfer(a, "verified"); err == nil {
		t.Fatal("revoked source imported")
	}
}

func TestRemoteResultsRecoverMetadataQuotaFailure(t *testing.T) {
	f := newResultsFixture(t)
	f.files.Limit = 64
	a := resultArtifact("nginx/summary.txt", "small evidence")
	if _, err := f.transfer(a, "small evidence"); err == nil {
		t.Fatal("metadata quota failure hidden")
	}
	if f.read("results/nginx/summary.txt") != "small evidence" {
		t.Fatal("completed business bytes were lost")
	}
	f.files.Limit = 1 << 20
	file, err := f.transfer(a, "small evidence")
	if err != nil || file.Path != "results/nginx/summary.txt" {
		t.Fatalf("metadata retry duplicated file: %s %v", file.Path, err)
	}
	var meta resultsMetadata
	if json.Unmarshal([]byte(f.read("results/meta.json")), &meta) != nil || len(meta.Files) != 1 {
		t.Fatal("metadata recovery failed")
	}
}

func TestRemoteResultsPreserveDirectoryAndParentFileCollisions(t *testing.T) {
	f := newResultsFixture(t)
	if _, err := f.files.AtomicWrite(t.Context(), "results/nginx/report/existing.txt", strings.NewReader("kept"), 4, false); err != nil {
		t.Fatal(err)
	}
	a := resultArtifact("nginx/report", "collected")
	file, err := f.transfer(a, "collected")
	if err != nil || file.Path != "results/nginx/report (2)" || f.read("results/nginx/report/existing.txt") != "kept" {
		t.Fatalf("directory collision: %s %v", file.Path, err)
	}
	if _, err := f.files.AtomicWrite(t.Context(), "results/parent-file", strings.NewReader("kept"), 4, false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.transfer(resultArtifact("parent-file/child.txt", "child"), "child"); err == nil {
		t.Fatal("parent file was replaced by a directory")
	}
	if f.read("results/parent-file") != "kept" {
		t.Fatal("parent file changed")
	}
}

func TestRemoteResultsRejectLostWriterFence(t *testing.T) {
	f := newResultsFixture(t)
	if _, err := f.service.Store.Pool.Exec(t.Context(), "UPDATE workspaces SET fence_token=fence_token+1 WHERE id=$1", f.workspace.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.transfer(resultArtifact("nginx/status.txt", "bytes"), "bytes"); err == nil {
		t.Fatal("stale writer published")
	}
	if _, _, err := f.files.Read("results/nginx/status.txt"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("stale writer wrote file bytes")
	}
}

func TestRemoteArtifactValidationRejectsEscapesAndInvalidHashes(t *testing.T) {
	for _, invalid := range []string{"../escape.txt", "/outside.txt", "a/../b.txt", "a\\b.txt", "a:stream", strings.Repeat("x", 256)} {
		t.Run(invalid, func(t *testing.T) {
			if validRemoteArtifact(resultArtifact(invalid, "x"), 1024) {
				t.Fatal("unsafe business path accepted")
			}
		})
	}
	a := resultArtifact("业务/检查.txt", "x")
	if !validRemoteArtifact(a, 1024) {
		t.Fatal("valid business path rejected")
	}
	a.ContentHash = strings.Repeat("g", 64)
	if validRemoteArtifact(a, 1024) {
		t.Fatal("invalid hash accepted")
	}
}

func TestRemoteResultCollisionPreservesValidUnicodeFileNames(t *testing.T) {
	desired := "results/业务/" + strings.Repeat("检", 83) + ".txt"
	renamed := resultCollisionPath(desired, 2)
	if !utf8.ValidString(renamed) || len(path.Base(renamed)) > 255 || !strings.HasPrefix(renamed, "results/业务/") || !strings.HasSuffix(renamed, " (2).txt") {
		t.Fatalf("invalid collision filename: %s", renamed)
	}
	if got := resultCollisionPath("results/.env", 2); got != "results/.env (2)" {
		t.Fatal(got)
	}
}
