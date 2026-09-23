package conversation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"io"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func (service Service) ReadToolResult(ctx context.Context, enterprise, user uuid.UUID, ref string) (ToolResultView, error) {
	value, err := service.GetToolResult(ctx, enterprise, user, ref)
	if err != nil {
		return value, err
	}
	if value.Artifact.ObjectKey.Valid {
		if service.Objects == nil {
			return value, toolruntime.Error{Kind: "TOOL_RESULT_STORAGE_UNAVAILABLE"}
		}
		reader, err := service.Objects.ReadFile(ctx, value.Artifact.ObjectKey.String, 0, 0)
		if err != nil {
			return value, err
		}
		defer reader.Close()
		data, err := io.ReadAll(io.LimitReader(reader, int64(value.Artifact.ByteSize)+1))
		if err != nil {
			return value, err
		}
		if len(data) != int(value.Artifact.ByteSize) {
			return value, toolruntime.Error{Kind: "TOOL_RESULT_INVALID"}
		}
		value.Artifact.Content = data
	}
	checksum := sha256.Sum256(value.Artifact.Content)
	if !bytes.Equal(checksum[:], value.Artifact.ContentHash) {
		return value, toolruntime.Error{Kind: "TOOL_RESULT_INVALID"}
	}
	return value, nil
}

func (service Service) resultScope(ctx context.Context, enterprise, user uuid.UUID, scope string) error {
	current, err := presentation.Scope(ctx, service.Store, enterprise, user)
	if err != nil {
		return err
	}
	if current != scope {
		return toolruntime.Error{Kind: "TOOL_RESULT_FORBIDDEN"}
	}
	return nil
}
