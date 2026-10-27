package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
)

type plan8ConfigProbeState struct {
	DesiredRevision int64 `json:"desired_revision"`
	Drafts          int64 `json:"drafts"`
	Targets         int64 `json:"targets"`
	Collections     int64 `json:"collections"`
}

// Exercise the actual creation endpoint with the same authorized executor and
// valid credentials, changing only a valid name and an invalid interval. The
// following positive creation still has to converge through the real Collector.
func (a *App) plan8DedicatedConfigAdmission(ctx context.Context, env *E2EEnvironment, collector uuid.UUID, integration string, input map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	var negative map[string]any
	if err = json.Unmarshal(raw, &negative); err != nil {
		return nil, err
	}
	collection, ok := negative["collection"].(map[string]any)
	if !ok {
		return nil, errors.New("configuration probe requires a collection")
	}
	name := "plan8-invalid-" + uuid.NewString()
	collection["name"], collection["target_name"] = name, name
	config, ok := collection["config"].(map[string]any)
	if !ok {
		return nil, errors.New("configuration probe requires a typed configuration")
	}
	config["interval"] = "invalid-duration"
	observe := func() (plan8ConfigProbeState, error) {
		var state plan8ConfigProbeState
		// Only generated UUIDs enter these literals; no names, addresses or
		// authentication inputs enter SQL or the evidence record.
		query := `SELECT json_build_object('desired_revision',c.desired_revision,
'drafts',(SELECT count(*) FROM monitoring_credential_drafts d WHERE d.enterprise_id=c.enterprise_id AND d.collector_id=c.id),
'targets',(SELECT count(*) FROM monitoring_targets t WHERE t.enterprise_id=c.enterprise_id AND t.name='` + name + `'),
'collections',(SELECT count(*) FROM monitoring_collections m WHERE m.enterprise_id=c.enterprise_id AND m.name='` + name + `')) FROM collector_instances c WHERE c.id='` + collector.String() + `'`
		value, err := a.postgresQuery(ctx, env, query)
		if err != nil {
			return state, err
		}
		err = json.Unmarshal([]byte(value), &state)
		return state, err
	}
	before, err := observe()
	if err != nil || before.DesiredRevision < 1 || before.Targets != 0 || before.Collections != 0 {
		return nil, errors.New("configuration probe did not establish its owned baseline")
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return nil, err
	}
	result, err := client.JSON(ctx, "plan8-invalid-config-"+integration, "enterprise", http.MethodPost, "/monitoring/commands/preview", 400, negative, enterpriseHeaders(env, "plan8-invalid-"+uuid.NewString()))
	if err != nil {
		return nil, err
	}
	if result["code"] != "MONITORING_INVALID" {
		return nil, errors.New("configuration probe failed for a different prerequisite")
	}
	after, err := observe()
	if err != nil || after.DesiredRevision != before.DesiredRevision || after.Targets != 0 || after.Collections != 0 || after.Drafts > before.Drafts {
		return nil, errors.New("invalid configuration changed the executor or left partial monitoring state")
	}
	return map[string]any{"schema_rejected": true, "response_code": result["code"], "collector_revision_unchanged": true, "no_target_or_collection_created": true, "private_draft_count_not_increased": true}, nil
}
