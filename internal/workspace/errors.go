package workspace

import (
	"github.com/kakj-go/Argus/internal/toolruntime"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func fileError(err error) error {
	if err == nil {
		return nil
	}
	switch status.Code(err) {
	case codes.ResourceExhausted:
		return toolruntime.Error{Kind: "WORKSPACE_QUOTA_EXCEEDED"}
	case codes.InvalidArgument:
		return toolruntime.Error{Kind: "WORKSPACE_FILE_INVALID"}
	case codes.AlreadyExists:
		return toolruntime.Error{Kind: "WORKSPACE_FILE_CONFLICT"}
	case codes.NotFound:
		return toolruntime.Error{Kind: "WORKSPACE_FILE_NOT_FOUND"}
	case codes.PermissionDenied, codes.Unauthenticated:
		return toolruntime.Error{Kind: "WORKSPACE_RUNTIME_UNAVAILABLE"}
	case codes.DeadlineExceeded:
		return toolruntime.Error{Kind: "WORKSPACE_OPERATION_TIMEOUT"}
	case codes.Unavailable:
		return toolruntime.Error{Kind: "WORKSPACE_RUNTIME_UNAVAILABLE"}
	default:
		return err
	}
}
