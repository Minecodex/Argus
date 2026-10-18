package remoteexecution

import (
	"context"
	"io"
	"path"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

type InputReader struct {
	io.ReadCloser
	authorize  func() error
	Size       int64
	Name, Hash string
}

func (r InputReader) Read(data []byte) (int, error) {
	if err := r.authorize(); err != nil {
		return 0, err
	}
	return r.ReadCloser.Read(data)
}
func (s Service) ReadInput(ctx context.Context, p toolruntime.Principal, requestID, fileID uuid.UUID) (InputReader, error) {
	view, err := s.Get(ctx, p, requestID)
	if err != nil {
		return InputReader{}, err
	}
	if s.Objects == nil {
		return InputReader{}, Error("REMOTE_TRANSFER_UNAVAILABLE")
	}
	file, err := s.Store.Queries.GetRemoteFile(ctx, db.GetRemoteFileParams{ID: fileID, EnterpriseID: p.EnterpriseID, RequestID: requestID})
	if err != nil || file.Direction != "input" {
		return InputReader{}, Error("REMOTE_INPUT_UNAVAILABLE")
	}
	authorize := func() error {
		if _, e := s.Get(ctx, p, requestID); e != nil {
			return e
		}
		if len(view.Spec.Inputs) > 0 && s.Files != nil {
			return s.Files.ValidateRemoteInputs(ctx, p, view.Spec.Inputs)
		}
		return nil
	}
	if err = authorize(); err != nil {
		return InputReader{}, err
	}
	reader, err := s.Objects.ReadFile(ctx, file.ObjectKey, 0, 0)
	if err != nil {
		return InputReader{}, err
	}
	return InputReader{ReadCloser: reader, authorize: authorize, Size: file.ByteSize, Name: path.Base(file.Path), Hash: file.ContentHash}, nil
}
