package workspaceio

import (
	"context"
	"crypto/tls"
	"io"

	workspacev1 "github.com/kakj-go/Argus/internal/gen/proto/argus/workspace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type Client struct {
	conn      *grpc.ClientConn
	RPC       workspacev1.WorkspaceIOServiceClient
	Authority *workspacev1.Authority
}

func Dial(endpoint string, config *tls.Config, workspaceID string, fence int64) (*Client, error) {
	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(credentials.NewTLS(config)), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(1<<20), grpc.MaxCallSendMsgSize(1<<20)))
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn, RPC: workspacev1.NewWorkspaceIOServiceClient(conn), Authority: &workspacev1.Authority{WorkspaceId: workspaceID, FenceToken: fence}}, nil
}

func (client *Client) Close() error { return client.conn.Close() }

func (client *Client) Bind(ctx context.Context, generation int64) error {
	_, err := client.RPC.BindLease(ctx, &workspacev1.BindLeaseRequest{WorkspaceId: client.Authority.WorkspaceId, Generation: generation, FenceToken: client.Authority.FenceToken})
	return err
}
func (client *Client) Release(ctx context.Context) error {
	_, err := client.RPC.ReleaseLease(ctx, &workspacev1.ReleaseLeaseRequest{Authority: client.Authority})
	return err
}

func (client *Client) Write(ctx context.Context, path string, reader io.Reader, size int64, replace bool) (*workspacev1.UploadResponse, error) {
	stream, err := client.RPC.Upload(ctx)
	if err != nil {
		return nil, err
	}
	if err := stream.Send(&workspacev1.UploadRequest{Authority: client.Authority, Path: path, ExpectedBytes: size, Replace: replace}); err != nil {
		return nil, err
	}
	buffer := make([]byte, 64<<10)
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			if sendErr := stream.Send(&workspacev1.UploadRequest{Data: buffer[:n]}); sendErr != nil {
				if _, cause := stream.CloseAndRecv(); cause != nil {
					return nil, cause
				}
				return nil, sendErr
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	return stream.CloseAndRecv()
}

func (client *Client) Read(ctx context.Context, path string, offset, length int64) (io.ReadCloser, int64, error) {
	ctx, cancel := context.WithCancel(ctx)
	stream, err := client.RPC.Download(ctx, &workspacev1.DownloadRequest{Authority: client.Authority, Path: path, Offset: offset, Length: length})
	if err != nil {
		cancel()
		return nil, 0, err
	}
	first, err := stream.Recv()
	if err != nil {
		cancel()
		return nil, 0, err
	}
	return &downloadReader{stream: stream, buffer: first.Data, cancel: cancel}, first.TotalBytes, nil
}

type downloadReader struct {
	stream grpc.ServerStreamingClient[workspacev1.DownloadResponse]
	buffer []byte
	cancel context.CancelFunc
}

func (reader *downloadReader) Close() error { reader.cancel(); return nil }
func (reader *downloadReader) Read(p []byte) (int, error) {
	for len(reader.buffer) == 0 {
		chunk, err := reader.stream.Recv()
		if err != nil {
			return 0, err
		}
		reader.buffer = chunk.Data
	}
	n := copy(p, reader.buffer)
	reader.buffer = reader.buffer[n:]
	return n, nil
}
