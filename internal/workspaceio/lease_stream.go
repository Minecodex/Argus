package workspaceio

import (
	"context"
	workspacev1 "github.com/kakj-go/Argus/internal/gen/proto/argus/workspace/v1"
	"google.golang.org/grpc"
)

// Returning the RPC handler closes gRPC's transport stream and releases the
// pending Recv/Send. Lease cancellation can therefore drain a stalled peer.
func receiveUpload(ctx context.Context, stream grpc.ClientStreamingServer[workspacev1.UploadRequest, workspacev1.UploadResponse]) (*workspacev1.UploadRequest, error) {
	type response struct {
		value *workspacev1.UploadRequest
		err   error
	}
	done := make(chan response, 1)
	go func() { value, err := stream.Recv(); done <- response{value, err} }()
	select {
	case value := <-done:
		return value.value, value.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func sendDownload(ctx context.Context, stream grpc.ServerStreamingServer[workspacev1.DownloadResponse], value *workspacev1.DownloadResponse) error {
	done := make(chan error, 1)
	go func() { done <- stream.Send(value) }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
