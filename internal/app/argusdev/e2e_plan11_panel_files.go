package argusdev

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboard"
)

func plan11DownloadPanelOriginal(ctx context.Context, client *ScenarioHTTP, env *E2EEnvironment, conversation string, file dashboard.QueryFileView) ([]byte, error) {
	if file.WorkspaceFileID == nil || uuid.Validate(conversation) != nil {
		return nil, errors.New("panel has no authorized Workspace delivery")
	}
	body, _, err := p5FileHTTP(ctx, client, env, http.MethodGet, "/conversations/"+conversation+"/workspace/files/"+file.WorkspaceFileID.String()+"/content", nil, 200, "")
	if err != nil {
		return nil, err
	}
	if int64(len(body)) != file.Bytes || fmt.Sprintf("%x", sha256.Sum256(body)) != file.Hash {
		return nil, fmt.Errorf("%w: panel original failed its sealed hash/size", errPlan11ComparisonBoundary)
	}
	if err := writePrivate(filepath.Join(env.Options.Artifacts, "panel-original-"+file.ID.String()+".json"), body); err != nil {
		return nil, err
	}
	return body, nil
}

// Preserve originals before interpreting the model's reply. Every fetch still
// uses the normal current-subject Workspace authorization; there is no cache
// fallback when access is denied, and later verification rechecks that access.
func (a *App) plan11LoadPanelFiles(ctx context.Context, env *E2EEnvironment, conversation string, jobs []dashboard.QueryJobView) ([]map[string]any, error) {
	client, err := scenarioHTTP(env)
	if err != nil {
		return nil, err
	}
	return plan11CollectPanelOriginals(jobs, func(file dashboard.QueryFileView) ([]byte, error) {
		return plan11DownloadPanelOriginal(ctx, client, env, conversation, file)
	})
}

func plan11CollectPanelOriginals(jobs []dashboard.QueryJobView, fetch func(dashboard.QueryFileView) ([]byte, error)) ([]map[string]any, error) {
	count, size := 0, int64(0)
	seen := map[uuid.UUID]bool{}
	for _, job := range jobs {
		for _, file := range job.Files {
			if file.Kind != "data" && file.Kind != "manifest" {
				continue
			}
			count++
			if count > 512 || file.Bytes < 0 || file.Bytes > 64<<20-size {
				return nil, errors.New("panel original export exceeds file/byte limits")
			}
			size += file.Bytes
			if file.ID == uuid.Nil || seen[file.ID] || file.WorkspaceFileID == nil || *file.WorkspaceFileID == uuid.Nil {
				return nil, errors.New("duplicate or undelivered panel original")
			}
			seen[file.ID] = true
		}
	}
	rows := []map[string]any{}
	var failures error
	for _, job := range jobs {
		for _, file := range job.Files {
			if file.Kind != "data" && file.Kind != "manifest" {
				continue
			}
			body, err := fetch(file)
			if err == nil && (int64(len(body)) != file.Bytes || fmt.Sprintf("%x", sha256.Sum256(body)) != file.Hash) {
				err = fmt.Errorf("%w: exported panel bytes differ from the sealed source", errPlan11ComparisonBoundary)
			}
			if err != nil {
				failures = errors.Join(failures, err)
				continue
			}
			// Manifests are immutable inputs too. Export/check their exact bytes,
			// while the panel fact list continues to describe numeric targets only.
			if file.Kind == "manifest" {
				continue
			}
			rows = append(rows, map[string]any{"id": file.ID, "job_id": job.ID, "panel_id": file.PanelID, "target_id": file.TargetID, "bytes": file.Bytes, "sha256": file.Hash, "sealed_before_conclusion_checks": true})
		}
	}
	return rows, failures
}
