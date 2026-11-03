package argusdev

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboard"
)

func TestPanelOriginalCollectionRetainsAvailableFilesWithoutWeakeningAuthorization(t *testing.T) {
	body := []byte(`[{"metric":{},"values":[[1,"2"]]}]`)
	workspace := uuid.New()
	file := dashboard.QueryFileView{ID: uuid.New(), Kind: "data", WorkspaceFileID: &workspace, Bytes: int64(len(body)), Hash: fmt.Sprintf("%x", sha256.Sum256(body))}
	other := file
	other.ID = uuid.New()
	jobs := []dashboard.QueryJobView{{ID: uuid.New(), Files: []dashboard.QueryFileView{file, other}}}
	denied := errors.New("current Workspace source authorization denied")
	requests := 0
	rows, err := plan11CollectPanelOriginals(jobs, func(current dashboard.QueryFileView) ([]byte, error) {
		requests++
		if current.ID == file.ID {
			return nil, denied
		}
		return body, nil
	})
	if !errors.Is(err, denied) || requests != 2 || len(rows) != 1 || rows[0]["id"] != other.ID {
		t.Fatal("denial was hidden or available original was lost", rows, requests, err)
	}
	rows, err = plan11CollectPanelOriginals(jobs[:1], func(dashboard.QueryFileView) ([]byte, error) { return []byte("rewritten"), nil })
	if !errors.Is(err, errPlan11ComparisonBoundary) || len(rows) != 0 {
		t.Fatal("rewritten panel was not an integrity boundary failure", err)
	}
}

func TestPanelOriginalExportBoundsAndDuplicateChecksPrecedeRequests(t *testing.T) {
	workspace := uuid.New()
	file := dashboard.QueryFileView{ID: uuid.New(), Kind: "data", WorkspaceFileID: &workspace, Bytes: 1}
	oversized := file
	oversized.Bytes = 64<<20 + 1
	undelivered := file
	undelivered.WorkspaceFileID = nil
	tooMany := make([]dashboard.QueryFileView, 513)
	for i := range tooMany {
		tooMany[i] = file
		tooMany[i].ID = uuid.New()
	}
	for _, files := range [][]dashboard.QueryFileView{{file, file}, {oversized}, {undelivered}, tooMany} {
		calls := 0
		if _, err := plan11CollectPanelOriginals([]dashboard.QueryJobView{{Files: files}}, func(dashboard.QueryFileView) ([]byte, error) { calls++; return nil, nil }); err == nil || calls != 0 {
			t.Fatal("invalid/unbounded export reached the transport", err, calls)
		}
	}
}

func TestPanelExportPreservesManifestBytesWithoutCountingThemAsPanelFacts(t *testing.T) {
	workspace := uuid.New()
	data, manifest := []byte(`[{"metric":{},"values":[[1,"2"]]}]`), []byte(`{"schema_version":"argus.dashboard_query_manifest/v1"}`)
	file := func(kind string, body []byte) dashboard.QueryFileView {
		return dashboard.QueryFileView{ID: uuid.New(), Kind: kind, WorkspaceFileID: &workspace, Bytes: int64(len(body)), Hash: fmt.Sprintf("%x", sha256.Sum256(body))}
	}
	panel, definition := file("data", data), file("manifest", manifest)
	jobs := []dashboard.QueryJobView{{ID: uuid.New(), Files: []dashboard.QueryFileView{panel, definition}}}
	requests := 0
	rows, err := plan11CollectPanelOriginals(jobs, func(current dashboard.QueryFileView) ([]byte, error) {
		requests++
		if current.Kind == "manifest" {
			return manifest, nil
		}
		return data, nil
	})
	if err != nil || requests != 2 || len(rows) != 1 || rows[0]["id"] != panel.ID {
		t.Fatal("manifest not preserved or counted as a numeric panel", requests, rows, err)
	}
	_, err = plan11CollectPanelOriginals(jobs, func(current dashboard.QueryFileView) ([]byte, error) {
		if current.Kind == "manifest" {
			return []byte("changed"), nil
		}
		return data, nil
	})
	if !errors.Is(err, errPlan11ComparisonBoundary) {
		t.Fatal("rewritten manifest did not fail sealed hash/size", err)
	}
}
