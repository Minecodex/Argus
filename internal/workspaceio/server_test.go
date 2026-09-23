package workspaceio

import (
	"context"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	workspacev1 "github.com/kakj-go/Argus/internal/gen/proto/argus/workspace/v1"
	"github.com/kakj-go/Argus/internal/workspacefs"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestFileRPCRejectsStaleFenceAndPreservesInterruptedTarget(t *testing.T) {
	files, err := workspacefs.Open(t.TempDir(), 1024)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = files.Close() })
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	workspacev1.RegisterWorkspaceIOServiceServer(server, &Server{Files: files, WorkspaceID: "workspace-a", Fence: 2})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient("passthrough:///file-rpc", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	client := &Client{conn: conn, RPC: workspacev1.NewWorkspaceIOServiceClient(conn), Authority: &workspacev1.Authority{WorkspaceId: "workspace-a", FenceToken: 2}}
	if _, err := client.Write(t.Context(), "data.csv", strings.NewReader("value\n10\n"), 9, false); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []*workspacev1.Authority{{WorkspaceId: "workspace-b", FenceToken: 2}, {WorkspaceId: "workspace-a", FenceToken: 1}} {
		_, err := client.RPC.Delete(t.Context(), &workspacev1.DeleteRequest{Authority: invalid, Path: "data.csv"})
		if status.Code(err) != codes.PermissionDenied {
			t.Fatalf("invalid authority accepted: %v", err)
		}
	}
	stream, err := client.RPC.Upload(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(&workspacev1.UploadRequest{Authority: client.Authority, Path: "data.csv", ExpectedBytes: 9, Replace: true, Data: []byte("short")}); err != nil {
		t.Fatal(err)
	}
	if _, err := stream.CloseAndRecv(); status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("incomplete stream accepted: %v", err)
	}
	reader, size, err := client.Read(t.Context(), "data.csv", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil || size != 9 || string(data) != "value\n10\n" {
		t.Fatalf("interrupted upload changed original: %q / %v", data, err)
	}
	reader, _, err = client.Read(t.Context(), "data.csv", 6, 2)
	if err != nil {
		t.Fatal(err)
	}
	data, err = io.ReadAll(reader)
	_ = reader.Close()
	if err != nil || string(data) != "10" {
		t.Fatalf("range = %q / %v", data, err)
	}
	if _, _, err := client.Read(t.Context(), "../outside", 0, 0); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("path escape accepted: %v", err)
	}
}

func TestTakeoverDrainsStalledUploadBeforeAcceptingNewFence(t *testing.T) {
	root := t.TempDir()
	files, err := workspacefs.Open(root, 1024)
	if err != nil {
		t.Fatal(err)
	}
	defer files.Close()
	service := &Server{Files: files, WorkspaceID: "workspace-a", Fence: 7}
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	workspacev1.RegisterWorkspaceIOServiceServer(server, service)
	go func() { _ = server.Serve(listener) }()
	defer server.Stop()
	conn, err := grpc.NewClient("passthrough:///takeover", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	old := &Client{conn: conn, RPC: workspacev1.NewWorkspaceIOServiceClient(conn), Authority: &workspacev1.Authority{WorkspaceId: "workspace-a", FenceToken: 7}}
	if _, err := old.Write(t.Context(), "business.csv", strings.NewReader("original"), 8, false); err != nil {
		t.Fatal(err)
	}
	stream, err := old.RPC.Upload(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(&workspacev1.UploadRequest{Authority: old.Authority, Path: "business.csv", Replace: true, ExpectedBytes: 100, Data: []byte("incomplete")}); err != nil {
		t.Fatal(err)
	}
	// Wait for AtomicWrite's temporary file, proving the RPC is blocked in Recv.
	deadline := time.Now().Add(2 * time.Second)
	for {
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) > 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("upload never entered AtomicWrite")
		}
		time.Sleep(time.Millisecond)
	}
	next := &Client{conn: conn, RPC: old.RPC, Authority: &workspacev1.Authority{WorkspaceId: "workspace-a", FenceToken: 8}}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := next.Bind(ctx, 7); err != nil {
		t.Fatalf("stalled writer did not drain: %v", err)
	}
	if _, err := stream.CloseAndRecv(); status.Code(err) != codes.Canceled {
		t.Fatalf("old upload remained active: %v", err)
	}
	if _, err := old.RPC.Delete(ctx, &workspacev1.DeleteRequest{Authority: old.Authority, Path: "business.csv"}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("old fence accepted: %v", err)
	}
	reader, _, err := next.Read(ctx, "business.csv", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil || string(data) != "original" {
		t.Fatalf("takeover changed old target: %q / %v", data, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 {
		t.Fatalf("partial upload leaked: %v / %v", entries, err)
	}
	if err := next.Release(ctx); err != nil {
		t.Fatal(err)
	}
	if err := next.Bind(ctx, 7); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("released fence reactivated: %v", err)
	}
}
