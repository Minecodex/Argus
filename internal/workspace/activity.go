package workspace

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/kakj-go/Argus/internal/toolruntime"
	"github.com/kakj-go/Argus/internal/workspaceio"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const activityPreviewBytes = 64 << 10
const activityPreviewLines = 512

type activitySnapshot struct {
	text, hash, reason string
	missing            bool
}

// Best-effort bounded evidence, under the existing workspace lease. A preview
// failure never converts a successful file operation into a retryable failure.
func readActivitySnapshot(ctx context.Context, client *workspaceio.Client, path string) activitySnapshot {
	reader, size, err := client.Read(ctx, path, 0, 0)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return activitySnapshot{missing: true}
		}
		return activitySnapshot{reason: "read_unavailable"}
	}
	defer reader.Close()
	if size > activityPreviewBytes {
		return activitySnapshot{reason: "size_limit"}
	}
	data, err := io.ReadAll(io.LimitReader(reader, activityPreviewBytes+1))
	if err != nil {
		return activitySnapshot{reason: "read_unavailable"}
	}
	if len(data) > activityPreviewBytes {
		return activitySnapshot{reason: "size_limit"}
	}
	if !utf8.Valid(data) || strings.ContainsRune(string(data), 0) {
		return activitySnapshot{reason: "binary"}
	}
	return activitySnapshot{text: string(data), hash: fmt.Sprintf("%x", sha256.Sum256(data))}
}

func activityLines(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

func fileChangeActivity(workspaceID, path, afterHash string, before, after activitySnapshot) *toolruntime.Activity {
	change := &toolruntime.FileChange{Operation: "update", BeforeHash: before.hash, AfterHash: afterHash}
	if before.missing {
		change.Operation = "create"
	}
	if before.reason != "" {
		change.Operation = "write"
		change.Reason = before.reason
	}
	activity := &toolruntime.Activity{Schema: "argus.tool_activity/v1", Kind: "file_change", Path: path, WorkspaceID: workspaceID, Change: change}
	if after.reason != "" {
		change.Reason = after.reason
	}
	if after.missing || after.hash != afterHash {
		change.Reason = "snapshot_changed"
	}
	left, right := activityLines(before.text), activityLines(after.text)
	if len(left) > activityPreviewLines || len(right) > activityPreviewLines {
		change.Reason = "size_limit"
	}
	if change.Reason != "" {
		return activity
	}
	// Bounded LCS preserves true inserted/deleted line counts without depending
	// on model arguments. Limit both the work and the durable preview size.
	width := len(right) + 1
	lcs := make([]uint16, (len(left)+1)*width)
	for i := len(left) - 1; i >= 0; i-- {
		for j := len(right) - 1; j >= 0; j-- {
			if left[i] == right[j] {
				lcs[i*width+j] = 1 + lcs[(i+1)*width+j+1]
			} else {
				lcs[i*width+j] = max(lcs[(i+1)*width+j], lcs[i*width+j+1])
			}
		}
	}
	added, removed, i, j, bytes := 0, 0, 0, 0, 0
	for i < len(left) || j < len(right) {
		line := toolruntime.DiffLine{}
		if i < len(left) && j < len(right) && left[i] == right[j] {
			line.Type = "context"
			line.Content = left[i]
			i++
			j++
		} else if j < len(right) && (i == len(left) || lcs[i*width+j+1] > lcs[(i+1)*width+j]) {
			line.Type = "add"
			line.Content = right[j]
			added++
			j++
		} else {
			line.Type = "remove"
			line.Content = left[i]
			removed++
			i++
		}
		bytes += len(line.Content)
		if bytes <= activityPreviewBytes && len(change.Lines) < activityPreviewLines {
			change.Lines = append(change.Lines, line)
		} else {
			change.Reason = "preview_limit"
		}
	}
	change.Added, change.Removed, change.Complete = &added, &removed, change.Reason == ""
	return activity
}
