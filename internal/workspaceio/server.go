// Package workspaceio provides fixed file RPCs in a short-lived PVC-mounted Pod.
// It has no model client, database credential, shell, or arbitrary command RPC.
package workspaceio

import (
	"context"
	"errors"
	"io"
	"os"
	"sync"
	"time"

	workspacev1 "github.com/kakj-go/Argus/internal/gen/proto/argus/workspace/v1"
	"github.com/kakj-go/Argus/internal/workspacefs"
	"github.com/kakj-go/Argus/internal/workspacelease"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	workspacev1.UnimplementedWorkspaceIOServiceServer
	Files       *workspacefs.Files
	WorkspaceID string
	Fence       int64
	leaseOnce   sync.Once
	lease       *workspacelease.Lease
}

func (server *Server) IdleFor(duration time.Duration) bool { return server.leases().IdleFor(duration) }

func (server *Server) leases() *workspacelease.Lease {
	server.leaseOnce.Do(func() { server.lease = workspacelease.New(server.WorkspaceID, server.Fence) })
	return server.lease
}

func (server *Server) BindLease(ctx context.Context, req *workspacev1.BindLeaseRequest) (*workspacev1.BindLeaseResponse, error) {
	if err := server.leases().Bind(ctx, req.WorkspaceId, req.Generation, req.FenceToken, nil); err != nil {
		return nil, leaseError(err)
	}
	return &workspacev1.BindLeaseResponse{}, nil
}
func (server *Server) ReleaseLease(ctx context.Context, req *workspacev1.ReleaseLeaseRequest) (*workspacev1.ReleaseLeaseResponse, error) {
	if req.Authority == nil {
		return nil, leaseError(workspacelease.ErrAuthority)
	}
	if err := server.leases().Release(ctx, req.Authority.WorkspaceId, req.Authority.FenceToken, nil); err != nil {
		return nil, leaseError(err)
	}
	return &workspacev1.ReleaseLeaseResponse{}, nil
}
func leaseError(err error) error {
	if errors.Is(err, workspacelease.ErrAuthority) {
		return status.Error(codes.PermissionDenied, "workspace authority mismatch")
	}
	return status.Error(codes.Unavailable, "workspace lease could not drain")
}
func (server *Server) begin(ctx context.Context, a *workspacev1.Authority) (context.Context, func(), error) {
	if a == nil {
		return nil, nil, leaseError(workspacelease.ErrAuthority)
	}
	ctx, done, err := server.leases().Begin(ctx, a.WorkspaceId, a.FenceToken)
	if err != nil {
		return nil, nil, leaseError(err)
	}
	return ctx, done, nil
}

func (server *Server) authorize(authority *workspacev1.Authority) error {
	if authority == nil || server.leases().Check(authority.WorkspaceId, authority.FenceToken) != nil {
		return status.Error(codes.PermissionDenied, "workspace authority mismatch")
	}
	return nil
}

func (server *Server) Upload(stream grpc.ClientStreamingServer[workspacev1.UploadRequest, workspacev1.UploadResponse]) error {
	header, err := stream.Recv()
	if err != nil {
		return err
	}
	ctx, done, err := server.begin(stream.Context(), header.Authority)
	if err != nil {
		return err
	}
	defer done()
	reader := &uploadReader{stream: stream, buffer: header.Data, header: header, server: server, ctx: ctx}
	info, err := server.Files.AtomicWrite(ctx, header.Path, reader, header.ExpectedBytes, header.Replace)
	if err != nil {
		return fileError(err)
	}
	return stream.SendAndClose(&workspacev1.UploadResponse{Path: info.Path, ByteSize: info.Size, ContentHash: info.Hash})
}

type uploadReader struct {
	stream grpc.ClientStreamingServer[workspacev1.UploadRequest, workspacev1.UploadResponse]
	buffer []byte
	header *workspacev1.UploadRequest
	server *Server
	ctx    context.Context
}

func (reader *uploadReader) Read(p []byte) (int, error) {
	for len(reader.buffer) == 0 {
		chunk, err := receiveUpload(reader.ctx, reader.stream)
		if err != nil {
			return 0, err
		}
		if chunk.Authority != nil {
			if err := reader.server.authorize(chunk.Authority); err != nil {
				return 0, err
			}
		}
		if chunk.Path != "" || chunk.ExpectedBytes != 0 || chunk.Replace {
			return 0, status.Error(codes.InvalidArgument, "upload metadata must only appear once")
		}
		reader.buffer = chunk.Data
	}
	n := copy(p, reader.buffer)
	reader.buffer = reader.buffer[n:]
	return n, nil
}

func (server *Server) Download(request *workspacev1.DownloadRequest, stream grpc.ServerStreamingServer[workspacev1.DownloadResponse]) error {
	ctx, done, err := server.begin(stream.Context(), request.Authority)
	if err != nil {
		return err
	}
	defer done()
	file, info, err := server.Files.Read(request.Path)
	if err != nil {
		return fileError(err)
	}
	defer file.Close()
	if request.Offset < 0 || request.Offset > info.Size || request.Length < 0 {
		return status.Error(codes.OutOfRange, "invalid file range")
	}
	length := info.Size - request.Offset
	if request.Length > 0 {
		length = min(length, request.Length)
	}
	if _, err := file.Seek(request.Offset, io.SeekStart); err != nil {
		return fileError(err)
	}
	if err := sendDownload(ctx, stream, &workspacev1.DownloadResponse{TotalBytes: info.Size}); err != nil {
		return err
	}
	reader := io.LimitReader(file, length)
	buffer := make([]byte, 64<<10)
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			if sendErr := sendDownload(ctx, stream, &workspacev1.DownloadResponse{Data: buffer[:n], TotalBytes: info.Size}); sendErr != nil {
				return sendErr
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fileError(err)
		}
	}
}

func (server *Server) Edit(ctx context.Context, request *workspacev1.EditRequest) (*workspacev1.EditResponse, error) {
	ctx, done, err := server.begin(ctx, request.Authority)
	if err != nil {
		return nil, err
	}
	defer done()
	info, err := server.Files.Edit(ctx, request.Path, request.OldText, request.NewText)
	if err != nil {
		return nil, fileError(err)
	}
	return &workspacev1.EditResponse{Path: info.Path, ByteSize: info.Size, ContentHash: info.Hash}, nil
}

func (server *Server) Delete(ctx context.Context, request *workspacev1.DeleteRequest) (*workspacev1.DeleteResponse, error) {
	_, done, err := server.begin(ctx, request.Authority)
	if err != nil {
		return nil, err
	}
	defer done()
	if err := server.Files.Delete(request.Path); err != nil {
		return nil, fileError(err)
	}
	return &workspacev1.DeleteResponse{}, nil
}

func fileError(err error) error {
	switch {
	case errors.Is(err, workspacefs.ErrPath), errors.Is(err, workspacefs.ErrFile):
		return status.Error(codes.InvalidArgument, "invalid workspace file")
	case errors.Is(err, workspacefs.ErrSize), workspacefs.IsQuotaError(err):
		return status.Error(codes.ResourceExhausted, "workspace file limit exceeded")
	case errors.Is(err, workspacefs.ErrConflict), errors.Is(err, os.ErrExist):
		return status.Error(codes.AlreadyExists, "workspace file conflict")
	case errors.Is(err, os.ErrNotExist):
		return status.Error(codes.NotFound, "workspace file not found")
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, "workspace request cancelled")
	default:
		return status.Error(codes.Internal, "workspace file operation failed")
	}
}
