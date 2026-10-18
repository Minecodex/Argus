package workspaceio

import (
	"context"
	workspacev1 "github.com/kakj-go/Argus/internal/gen/proto/argus/workspace/v1"
)

func (s *Server) ListFiles(ctx context.Context, r *workspacev1.ListFilesRequest) (*workspacev1.ListFilesResponse, error) {
	ctx, done, err := s.begin(ctx, r.Authority)
	if err != nil {
		return nil, err
	}
	defer done()
	files, err := s.Files.List(ctx, r.Path, int(r.MaxFiles), int64(r.MaxBytes))
	if err != nil {
		return nil, fileError(err)
	}
	result := &workspacev1.ListFilesResponse{Files: []*workspacev1.WorkspaceFileInfo{}}
	for _, file := range files {
		result.Files = append(result.Files, &workspacev1.WorkspaceFileInfo{Path: file.Path, ByteSize: file.Size})
	}
	return result, nil
}
