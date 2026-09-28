package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/kakj-go/Argus/internal/dashboard"
)

func (a *App) verifyPlanV2FileAudit(ctx context.Context, env *E2EEnvironment, job dashboard.QueryJobView) error {
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	events := []map[string]any{}
	for _, action := range []string{"dashboard.query.queued", "dashboard.query.materialized", "dashboard.query.delivered"} {
		params := url.Values{"action": {action}, "resource_id": {job.ID.String()}}
		page, err := client.JSON(ctx, action+"-audit", "enterprise", http.MethodGet, "/enterprise/audit-events?"+params.Encode(), 200, nil, enterpriseHeaders(env, ""))
		if err != nil {
			return err
		}
		items := objectItems(page)
		if len(items) != 1 {
			return fmt.Errorf("%s expected one durable audit event, got %d", action, len(items))
		}
		event := items[0]
		details := nestedMap(event, "details")
		specHash, _ := details["spec_hash"].(string)
		if details["query_job_id"] != job.ID.String() || details["revision_id"] != job.RevisionID.String() || len(specHash) != 64 {
			return fmt.Errorf("query audit lost publication identity")
		}
		if action == "dashboard.query.delivered" {
			if details["attempt_id"] != job.Manifest.AttemptID.String() || details["complete"] != true || details["analysis_status"] != "not_analyzed" {
				return fmt.Errorf("delivery audit confused delivery with analysis")
			}
			files, _ := details["files"].([]any)
			if len(files) != 3 {
				return fmt.Errorf("file audit lost original data file identities")
			}
			for _, raw := range files {
				f := raw.(map[string]any)
				found := false
				for _, actual := range job.Files {
					if actual.ID.String() == f["file_id"] && actual.Hash == f["content_hash"] && actual.SourceRef == f["source_ref"] {
						found = true
					}
				}
				if !found {
					return fmt.Errorf("audit file hash/source differs from delivered bytes")
				}
			}
		}
		encoded, _ := json.Marshal(details)
		for _, private := range []string{"context_token", "argus__token", "argus-m7-e2e-planv2"} {
			if strings.Contains(string(encoded), private) {
				return fmt.Errorf("raw query context or data entered audit")
			}
		}
		events = append(events, event)
	}
	raw, err := json.MarshalIndent(events, "", "  ")
	if err != nil {
		return err
	}
	return writePrivate(filepath.Join(env.Options.Artifacts, "planv2-audit-files.json"), raw)
}
