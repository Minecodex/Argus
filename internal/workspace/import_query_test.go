package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/config"
	workspacev1 "github.com/kakj-go/Argus/internal/gen/proto/argus/workspace/v1"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"github.com/kakj-go/Argus/internal/workspacefs"
	"github.com/kakj-go/Argus/internal/workspaceio"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func TestQueryFileImportWithRealRPCAndDurableSources(t *testing.T) {
	address := os.Getenv("ARGUS_P5_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("disposable migrated PostgreSQL required")
	}
	ctx := t.Context()
	store, err := postgres.Open(ctx, address)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, e := store.Pool.Exec(ctx, sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	e, d, u, m, c, w := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	owner := uuid.NewString()
	exec(`INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Query import',$2,'UTC')`, e, "import-"+e.String())
	exec(`INSERT INTO departments(id,enterprise_id,name,is_default) VALUES($1,$2,'Default',true)`, d, e)
	exec(`INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Import')`, u, e, d, "import-"+u.String())
	exec(`INSERT INTO ai_models(id,enterprise_id,name,base_url,model_id,api_protocol,context_window_tokens,max_output_tokens,input_price_per_million,output_price_per_million,health_status) VALUES($1,$2,'Import','https://model.example.test','model','chat_completions',32768,1024,0,0,'healthy')`, m, e)
	exec(`INSERT INTO conversations(id,enterprise_id,owner_user_id,title,selected_model_id) VALUES($1,$2,$3,'Import',$4)`, c, e, u, m)
	exec(`INSERT INTO workspaces(id,enterprise_id,conversation_id,pvc_name,namespace,status,capacity_bytes,environment_version,lease_owner,lease_until,fence_token) VALUES($1,$2,$3,$4,'test-query-import','ready',2147483648,'test',$5,now()+interval '10 minutes',1)`, w, e, c, "query-"+w.String(), owner)
	principal := toolruntime.Principal{EnterpriseID: e, UserID: u, ConversationID: c, AuthorizationVersion: 1, Permissions: []string{"workspace.use"}}
	allowed := true
	source := "dashboard-query:" + uuid.NewString()
	service := Service{Store: store, Config: config.Workspace{MaxFileBytes: 256}, ExternalSource: func(_ context.Context, p toolruntime.Principal, ref string) (bool, error) {
		if !allowed || p.UserID != u || p.ConversationID != c || ref != source {
			return true, toolruntime.Error{Kind: "WORKSPACE_FILE_FORBIDDEN"}
		}
		return true, nil
	}}
	root := t.TempDir()
	files, err := workspacefs.Open(root, 128)
	if err != nil {
		t.Fatal(err)
	}
	defer files.Close()
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	workspacev1.RegisterWorkspaceIOServiceServer(server, &workspaceio.Server{Files: files, WorkspaceID: w.String(), Fence: 1})
	go func() { _ = server.Serve(listener) }()
	defer server.Stop()
	conn, err := grpc.NewClient("passthrough:///query-import", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client := &workspaceio.Client{RPC: workspacev1.NewWorkspaceIOServiceClient(conn), Authority: &workspacev1.Authority{WorkspaceId: w.String(), FenceToken: 1}}
	current, err := store.Queries.GetWorkspace(ctx, db.GetWorkspaceParams{ID: w, EnterpriseID: e})
	if err != nil {
		t.Fatal(err)
	}
	makeAccess := func() *Access {
		accessCtx, cancel := context.WithCancel(ctx)
		t.Cleanup(cancel)
		return &Access{Context: accessCtx, Workspace: current, IO: client, service: service, owner: owner, cancel: cancel, principal: &principal}
	}
	digest := func(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
	body := `{"observed":true}`
	transfer := func(path, data, expected string) (db.WorkspaceFile, error) {
		return service.importQueryFile(ctx, principal, source, path, strings.NewReader(data), int64(len(data)), expected, makeAccess())
	}
	first, err := transfer("queries/value.json", body, digest(body))
	if err != nil {
		t.Fatal(err)
	}
	second, err := transfer("queries/value.json", body, digest(body))
	if err != nil || first.ID != second.ID {
		t.Fatalf("replay duplicated metadata: %+v %v", second, err)
	}
	current, err = store.Queries.GetWorkspace(ctx, db.GetWorkspaceParams{ID: w, EnterpriseID: e})
	if err != nil || !slices.Contains(current.SourceResultRefs, source) {
		t.Fatal("source was not registered before file publication")
	}
	if _, err := files.AtomicWrite(ctx, "queries/value.json", strings.NewReader("changed"), 7, true); err != nil {
		t.Fatal(err)
	}
	if _, err := transfer("queries/value.json", body, digest(body)); err == nil {
		t.Fatal("changed user copy overwritten")
	}
	if _, err := files.AtomicWrite(ctx, "queries/orphan.json", strings.NewReader(body), int64(len(body)), false); err != nil {
		t.Fatal(err)
	}
	if _, err := transfer("queries/orphan.json", body, digest(body)); err != nil {
		t.Fatalf("post-rename recovery failed: %v", err)
	}
	if _, err := transfer("queries/corrupt.json", body, digest("wrong")); err == nil {
		t.Fatal("unverified input committed")
	}
	if _, _, err := files.Read("queries/corrupt.json"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("corrupt file published: %v", err)
	}
	if _, err := transfer("queries/too-large.json", strings.Repeat("x", 256), digest(strings.Repeat("x", 256))); err == nil {
		t.Fatal("file budget ignored")
	}
	if _, _, err := files.Read("queries/too-large.json"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed quota write published")
	}
	allowed = false
	if _, err := transfer("queries/denied.json", body, digest(body)); err == nil {
		t.Fatal("revoked source imported")
	}
	if err := service.authorizeWorkspace(ctx, principal, current); err == nil {
		t.Fatal("derived-file directory escaped source revocation")
	}
	if _, err := service.publicationSources(ctx, principal, w); err == nil {
		t.Fatal("revoked directory remained publishable")
	}
}

func TestQueryImportReaderRejectsTruncationAndHashMismatch(t *testing.T) {
	for _, data := range []string{"short", "different", "too long data"} {
		r := &queryImportReader{reader: strings.NewReader(data), digest: sha256.New(), expected: strings.Repeat("0", 64), size: 9}
		if _, err := io.ReadAll(r); err == nil {
			t.Fatal("unverified stream accepted")
		}
	}
}

func TestQueryImportWaitsForLeaseWithoutRetryingQuotaOrRevocation(t *testing.T) {
	for _, code := range []string{"", "WORKSPACE_QUOTA_EXCEEDED", "WORKSPACE_FILE_FORBIDDEN", "WORKSPACE_LEASE_LOST"} {
		t.Run(code, func(t *testing.T) {
			calls := 0
			wanted := &Access{}
			got, err := waitQueryImportAccess(t.Context(), func() (*Access, error) {
				calls++
				if calls < 3 {
					return nil, toolruntime.Error{Kind: "WORKSPACE_BUSY"}
				}
				if code != "" {
					return nil, toolruntime.Error{Kind: code}
				}
				return wanted, nil
			})
			if calls != 3 || (code == "" && (err != nil || got != wanted)) {
				t.Fatalf("lease retry lost outcome: calls=%d error=%v", calls, err)
			}
			if code != "" {
				var coded interface{ Code() string }
				if !errors.As(err, &coded) || coded.Code() != code {
					t.Fatalf("non-busy error changed: %v", err)
				}
			}
		})
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	calls := 0
	_, err := waitQueryImportAccess(ctx, func() (*Access, error) { calls++; return nil, toolruntime.Error{Kind: "WORKSPACE_BUSY"} })
	if !errors.Is(err, context.DeadlineExceeded) || calls != 1 {
		t.Fatalf("cancelled background transfer kept acquiring: %v/%d", err, calls)
	}
}
