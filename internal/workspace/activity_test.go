package workspace

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"strings"
	"testing"

	workspacev1 "github.com/kakj-go/Argus/internal/gen/proto/argus/workspace/v1"
	"github.com/kakj-go/Argus/internal/workspacefs"
	"github.com/kakj-go/Argus/internal/workspaceio"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func snapshotText(text string) activitySnapshot {
	return activitySnapshot{text: text, hash: fmt.Sprintf("%x", sha256.Sum256([]byte(text)))}
}

func TestActivitySnapshotsUseActualLeasedFilesystemRPC(t *testing.T) {
	files, err := workspacefs.Open(t.TempDir(), 8<<20)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = files.Close() })
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	workspacev1.RegisterWorkspaceIOServiceServer(server, &workspaceio.Server{Files: files, WorkspaceID: "owned-activity", Fence: 1})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient("passthrough:///activity-rpc", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	client := &workspaceio.Client{RPC: workspacev1.NewWorkspaceIOServiceClient(conn), Authority: &workspacev1.Authority{WorkspaceId: "owned-activity", FenceToken: 1}}
	ctx := t.Context()
	path := "/workspace/report.md"
	before := readActivitySnapshot(ctx, client, path)
	if !before.missing {
		t.Fatal("actual missing file not recognized", before)
	}
	info, err := client.Write(ctx, path, strings.NewReader("first\nsecond\n"), 13, true)
	if err != nil {
		t.Fatal(err)
	}
	after := readActivitySnapshot(ctx, client, path)
	created := fileChangeActivity("owned-activity", info.Path, info.ContentHash, before, after)
	if !created.Change.Complete || *created.Change.Added != 2 || created.Change.Operation != "create" {
		t.Fatal("actual creation evidence wrong", created)
	}
	edited, err := client.RPC.Edit(ctx, &workspacev1.EditRequest{Authority: client.Authority, Path: path, OldText: "second", NewText: "actual third"})
	if err != nil {
		t.Fatal(err)
	}
	updated := fileChangeActivity("owned-activity", edited.Path, edited.ContentHash, after, readActivitySnapshot(ctx, client, path))
	if !updated.Change.Complete || *updated.Change.Added != 1 || *updated.Change.Removed != 1 {
		t.Fatal("actual edit evidence wrong", updated.Change)
	}
	if created.Change.Lines[1].Content != "second" {
		t.Fatal("later edit mutated earlier preview")
	}
	for _, fixture := range []struct{ path, body, reason string }{{"/workspace/binary", "\x00binary", "binary"}, {"/workspace/large", strings.Repeat("x", activityPreviewBytes+1), "size_limit"}} {
		if _, err := client.Write(ctx, fixture.path, strings.NewReader(fixture.body), int64(len(fixture.body)), true); err != nil {
			t.Fatal(err)
		}
		if snapshot := readActivitySnapshot(ctx, client, fixture.path); snapshot.reason != fixture.reason {
			t.Fatal("preview budget/type ignored", snapshot.reason)
		}
	}
}

func TestFileActivityRecordsActualCreateAndReplace(t *testing.T) {
	after := snapshotText("one\ntwo\n")
	created := fileChangeActivity("workspace", "/workspace/report.md", after.hash, activitySnapshot{missing: true}, after)
	if created.Change.Operation != "create" || *created.Change.Added != 2 || *created.Change.Removed != 0 || !created.Change.Complete {
		t.Fatalf("invalid actual create: %+v", created.Change)
	}
	next := snapshotText("one\nthree\n")
	updated := fileChangeActivity("workspace", "/workspace/report.md", next.hash, after, next)
	if updated.Change.Operation != "update" || *updated.Change.Added != 1 || *updated.Change.Removed != 1 || updated.Change.BeforeHash != after.hash {
		t.Fatalf("invalid actual update: %+v", updated.Change)
	}
	// A later filesystem state cannot mutate this persisted call's preview.
	if created.Change.AfterHash != after.hash || created.Change.Lines[1].Content != "two" {
		t.Fatal("historical diff changed")
	}
}

func TestFileActivityNeverInventsCountsOrCreationOnMissingEvidence(t *testing.T) {
	after := snapshotText("actual\n")
	for _, reason := range []string{"read_unavailable", "binary", "size_limit"} {
		got := fileChangeActivity("w", "file", after.hash, activitySnapshot{reason: reason}, after).Change
		if got.Operation == "create" || got.Complete || got.Added != nil || got.Removed != nil {
			t.Fatalf("invented evidence for %s: %+v", reason, got)
		}
	}
	got := fileChangeActivity("w", "file", "other-hash", snapshotText("old"), after).Change
	if got.Complete || got.Added != nil || got.Reason != "snapshot_changed" {
		t.Fatal("racing snapshot reported as exact")
	}
	large := snapshotText(strings.Repeat("line\n", activityPreviewLines+1))
	if got := fileChangeActivity("w", "file", large.hash, activitySnapshot{missing: true}, large).Change; got.Added != nil || got.Complete {
		t.Fatal("line budget ignored")
	}
}
