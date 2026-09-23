package workspacesupervisor

import (
	"context"
	"crypto/tls"
	workspacev1 "github.com/kakj-go/Argus/internal/gen/proto/argus/workspace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type Client struct {
	conn      *grpc.ClientConn
	RPC       workspacev1.WorkspaceSupervisorServiceClient
	Authority *workspacev1.Authority
}

func Dial(address string, config *tls.Config, id string, fence int64) (*Client, error) {
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(credentials.NewTLS(config)), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(1<<20), grpc.MaxCallSendMsgSize(1<<20)))
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn, RPC: workspacev1.NewWorkspaceSupervisorServiceClient(conn), Authority: &workspacev1.Authority{WorkspaceId: id, FenceToken: fence}}, nil
}
func (c *Client) Close() error { return c.conn.Close() }
func (c *Client) Bind(ctx context.Context, generation int64) error {
	_, err := c.RPC.BindLease(ctx, &workspacev1.WorkspaceSupervisorServiceBindLeaseRequest{WorkspaceId: c.Authority.WorkspaceId, Generation: generation, FenceToken: c.Authority.FenceToken})
	return err
}
func (c *Client) Release(ctx context.Context) error {
	_, err := c.RPC.ReleaseLease(ctx, &workspacev1.WorkspaceSupervisorServiceReleaseLeaseRequest{Authority: c.Authority})
	return err
}
