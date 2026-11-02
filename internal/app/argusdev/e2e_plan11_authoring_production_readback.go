package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/kakj-go/Argus/internal/dashboard"
)

// Re-execute the fixed real sample window after switching every backend to the
// normal image. Existing points must match; later arrivals may add points.
func (a *App) plan11AuthoringProductionReadback(ctx context.Context, env *E2EEnvironment, host *plan6HostLoop) error {
	if env.Options.Plan8Phase != "plan11-authoring-ui-replay" || host == nil {
		return errors.New("normal authoring readback requires its explicit owned phase")
	}
	var production struct {
		Passed bool
		Tags   string `json:"go_build_tags"`
	}
	raw, err := os.ReadFile(filepath.Join(env.Options.Artifacts, "plan6-production-backend.json"))
	if err != nil {
		return err
	}
	if json.Unmarshal(raw, &production) != nil || !production.Passed || production.Tags != "" {
		return errors.New("authoring readback requires the accepted normal production backend")
	}
	var replay struct {
		Passed bool
		Cases  []plan11AuthoringCase
	}
	raw, err = os.ReadFile(filepath.Join(env.Options.Artifacts, "plan11-authoring-ui-replay.json"))
	if err != nil {
		return err
	}
	if json.Unmarshal(raw, &replay) != nil || !replay.Passed || len(replay.Cases) != 2 {
		return errors.New("actual UI decisions are incomplete before production readback")
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	rows := []map[string]any{}
	for _, row := range replay.Cases {
		raw, err = os.ReadFile(filepath.Join(env.Options.Artifacts, "plan11-authoring-ui-original-"+row.Decision+"-sample.json"))
		if err != nil {
			return err
		}
		var original struct{ Execution dashboard.Execution }
		if json.Unmarshal(raw, &original) != nil || original.Execution.From.IsZero() || !original.Execution.To.After(original.Execution.From) {
			return errors.New("original preview lacks its exact real window")
		}
		draft, e := client.JSON(ctx, "normal-readback-draft-"+row.Decision, "enterprise", http.MethodGet, "/dashboard-drafts/"+row.Draft, 200, nil, enterpriseHeaders(env, ""))
		if e != nil {
			return e
		}
		parameters := map[string]any{"resource_mode": "explicit", "resource_types": []string{"host"}, "resource_ids": []string{host.HostID}, "from": original.Execution.From.Format(time.RFC3339Nano), "to": original.Execution.To.Format(time.RFC3339Nano)}
		url := "/dashboard-drafts/" + row.Draft + "/sample"
		body := map[string]any{"expected_version": draft["draft_version"], "parameters": parameters}
		if row.Decision == "confirm" {
			id, err := stringField(draft, "dashboard_id")
			if err != nil || draft["status"] != "published" {
				return errors.Join(err, errors.New("normal backend cannot read the published dashboard"))
			}
			url, body = "/dashboards/"+id+"/execute", parameters
		} else if draft["status"] != "editing" || draft["dashboard_id"] != nil {
			return errors.New("cancelled draft was published")
		}
		result, e := client.JSON(ctx, "normal-readback-sample-"+row.Decision, "enterprise", http.MethodPost, url, 200, body, enterpriseHeaders(env, "normal-readback-"+row.Decision))
		if e != nil {
			return e
		}
		value := any(result)
		if row.Decision == "cancel" {
			sample, ok := result["sample"].(map[string]any)
			if !ok {
				return errors.New("normal draft sample is unavailable")
			}
			value = sample["execution"]
		}
		encoded, e := json.Marshal(value)
		if e != nil {
			return e
		}
		var actual dashboard.Execution
		if json.Unmarshal(encoded, &actual) != nil {
			return errors.New("normal execution is not valid")
		}
		if !actual.From.Equal(original.Execution.From) || !actual.To.Equal(original.Execution.To) || actual.Partial || len(actual.Resources) != 1 || actual.Resources[0].Type != "host" || fmt.Sprint(actual.Resources[0].ID) != host.HostID {
			return errors.New("normal readback widened or changed the actual requested scope/window")
		}
		before, e := plan11AuthoringSamplePoints(original.Execution, host.HostID)
		if e != nil {
			return e
		}
		after, e := plan11AuthoringSamplePoints(actual, host.HostID)
		if e != nil {
			return e
		}
		for key, value := range before {
			found, ok := after[key]
			if !ok || math.Abs(found-value) > 1e-12*math.Max(1, math.Abs(value)) {
				return errors.New("preview point does not match normal production telemetry")
			}
		}
		rows = append(rows, map[string]any{"decision": row.Decision, "original_points": len(before), "normal_points": len(after), "original_points_verified": true, "exact_host_window": true, "normal_go_build_tags": ""})
	}
	encoded, err := json.MarshalIndent(map[string]any{"passed": true, "scope": "normal production readback after controlled local-provider L4; not new autonomous L5", "cases": rows, "external_model_calls": 0, "whole_plan_complete": false}, "", "  ")
	if err != nil {
		return err
	}
	return writePrivate(filepath.Join(env.Options.Artifacts, "plan11-authoring-production-readback.json"), encoded)
}

func plan11AuthoringSamplePoints(execution dashboard.Execution, host string) (map[string]float64, error) {
	points := map[string]float64{}
	if len(execution.Panels) != 2 || execution.Partial {
		return nil, errors.New("authoring sample needs two complete real panels")
	}
	for _, panel := range execution.Panels {
		if panel.Status != "success" || len(panel.Targets) != 1 || len(panel.Sources) != 1 || panel.Sources[0].Type != "hostmetrics" || fmt.Sprint(panel.Sources[0].ResourceID) != host {
			return nil, errors.New("authoring sample source differs from the current Host")
		}
		target := panel.Targets[0]
		if target.Status != "success" || target.Meta.Partial {
			return nil, errors.New("authoring target is incomplete")
		}
		raw, e := json.Marshal(target.Data)
		if e != nil {
			return nil, e
		}
		var series []struct {
			Metric map[string]string
			Values [][2]any
		}
		if json.Unmarshal(raw, &series) != nil || len(series) == 0 {
			return nil, errors.New("authoring target has no numeric data")
		}
		for _, s := range series {
			if id := s.Metric["argus_resource_id"]; id != "" && id != host {
				return nil, errors.New("foreign Host in actual authoring series")
			}
			labels, _ := json.Marshal(s.Metric)
			for _, p := range s.Values {
				timestamp, ok := p[0].(float64)
				if !ok {
					return nil, errors.New("authoring point lacks a timestamp")
				}
				value, err := strconv.ParseFloat(fmt.Sprint(p[1]), 64)
				if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
					return nil, errors.New("authoring point is not finite")
				}
				key := fmt.Sprintf("%s/%s/%s/%g", panel.ID, target.ID, labels, timestamp)
				if previous, duplicate := points[key]; duplicate && previous != value {
					return nil, errors.New("conflicting authoring points")
				}
				points[key] = value
			}
		}
	}
	if len(points) == 0 {
		return nil, errors.New("authoring sample is empty")
	}
	return points, nil
}
