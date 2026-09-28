package dashboard

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

const queryChunkBytes = 1 << 20

type queryChunk struct {
	Key   string `json:"key"`
	Hash  string `json:"sha256"`
	Bytes int64  `json:"bytes"`
}

func queryFileView(file db.DashboardQueryFile) QueryFileView {
	return QueryFileView{ID: file.ID, PanelID: file.PanelID, TargetID: file.TargetID, Kind: file.FileKind, Bytes: file.ByteSize, Hash: file.ContentHash, SourceRef: querySourceRef(file.JobID, file.AttemptID), Path: "/workspace/" + queryFilePath(file)}
}
func querySourceRef(job, attempt uuid.UUID) string {
	return QuerySourcePrefix + job.String() + "/" + attempt.String()
}
func queryFilePath(file db.DashboardQueryFile) string {
	name := file.ID.String() + ".json"
	if file.FileKind == "manifest" {
		name = "manifest.json"
	}
	return fmt.Sprintf("queries/%s/%s/%s", file.JobID, file.AttemptID, name)
}
func (jobs QueryJobs) materializeFile(ctx context.Context, job db.DashboardQueryJob, panel, target, kind string, value any, guard func() error) (db.DashboardQueryFile, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return db.DashboardQueryFile{}, err
	}
	if len(data) > 16<<20 {
		return db.DashboardQueryFile{}, ErrInvalid
	}
	file := db.DashboardQueryFile{ID: uuid.New(), EnterpriseID: job.EnterpriseID, JobID: job.ID, AttemptID: job.AttemptID.UUID, PanelID: panel, TargetID: target, FileKind: kind, ByteSize: int64(len(data)), ContentHash: queryHash(data)}
	chunks := []queryChunk{}
	for start := 0; start < len(data); start += queryChunkBytes {
		if err := guard(); err != nil {
			return file, err
		}
		part := data[start:min(start+queryChunkBytes, len(data))]
		digest := queryHash(part)
		key := fmt.Sprintf("enterprises/%s/conversations/%s/dashboard/%s/%s/%s/%06d-%s", job.EnterpriseID, job.ConversationID, job.ID, job.AttemptID.UUID, file.ID, len(chunks), digest)
		if err := jobs.Objects.PutFile(ctx, key, bytes.NewReader(part), int64(len(part)), "application/octet-stream"); err != nil {
			return file, err
		}
		chunks = append(chunks, queryChunk{key, digest, int64(len(part))})
	}
	file.Chunks, err = json.Marshal(chunks)
	return file, err
}

// Open one immutable fragment at a time; neither Tool Result nor the model is a
// bulk-data transport. Verify fragment hashes before exposing bytes and verify
// the whole file before the Workspace upload may atomically commit.
type queryFileReader struct {
	ctx     context.Context
	objects QueryObjectStore
	file    db.DashboardQueryFile
	chunks  []queryChunk
	next    int
	buffer  *bytes.Reader
	digest  hash.Hash
	read    int64
	guard   func() error
	failed  error
}

func newQueryFileReader(ctx context.Context, objects QueryObjectStore, file db.DashboardQueryFile, guard func() error) (*queryFileReader, error) {
	var chunks []queryChunk
	if json.Unmarshal(file.Chunks, &chunks) != nil || len(chunks) == 0 || len(chunks) > 32 {
		return nil, ErrInvalid
	}
	var size int64
	for _, c := range chunks {
		if c.Bytes < 1 || c.Bytes > queryChunkBytes || len(c.Hash) != 64 {
			return nil, ErrInvalid
		}
		size += c.Bytes
	}
	if size != file.ByteSize {
		return nil, ErrInvalid
	}
	return &queryFileReader{ctx: ctx, objects: objects, file: file, chunks: chunks, digest: sha256.New(), guard: guard}, nil
}
func (r *queryFileReader) Read(p []byte) (int, error) {
	if r.failed != nil {
		return 0, r.failed
	}
	if len(p) == 0 {
		return 0, nil
	}
	if err := r.guard(); err != nil {
		r.failed = err
		return 0, err
	}
	if r.buffer != nil && r.buffer.Len() > 0 {
		n, _ := r.buffer.Read(p)
		_, _ = r.digest.Write(p[:n])
		r.read += int64(n)
		return n, nil
	}
	if r.next == len(r.chunks) {
		if r.read != r.file.ByteSize || hex.EncodeToString(r.digest.Sum(nil)) != r.file.ContentHash {
			r.failed = ErrInvalid
			return 0, ErrInvalid
		}
		return 0, io.EOF
	}
	chunk := r.chunks[r.next]
	r.next++
	stream, err := r.objects.ReadFile(r.ctx, chunk.Key, 0, 0)
	if err != nil {
		r.failed = err
		return 0, err
	}
	data, err := io.ReadAll(io.LimitReader(stream, chunk.Bytes+1))
	closeErr := stream.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		r.failed = err
		return 0, err
	}
	if int64(len(data)) != chunk.Bytes || queryHash(data) != chunk.Hash {
		r.failed = ErrInvalid
		return 0, ErrInvalid
	}
	r.buffer = bytes.NewReader(data)
	return r.Read(p)
}
