package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"time"

	"github.com/kakj-go/Argus/internal/dashboard"
)

type plan11DashboardClaim struct {
	JobID     string             `json:"job_id"`
	AttemptID string             `json:"attempt_id"`
	Panels    []plan11PanelClaim `json:"panels"`
}

type plan11PanelClaim struct {
	PanelID  string   `json:"panel_id"`
	TargetID string   `json:"target_id"`
	Hash     string   `json:"input_sha256"`
	Unit     string   `json:"unit"`
	Count    *int     `json:"point_count"`
	First    string   `json:"first_point_at"`
	Last     string   `json:"last_point_at"`
	Min      *float64 `json:"value_min"`
	Max      *float64 `json:"value_max"`
}

func plan11PanelOriginalFacts(body []byte) (plan11PointEvidence, plan11ValueEvidence, error) {
	var rows []struct {
		Values [][]json.RawMessage `json:"values"`
		Value  []json.RawMessage   `json:"value"`
	}
	var points plan11PointEvidence
	values := plan11ValueEvidence{Min: math.Inf(1), Max: math.Inf(-1)}
	if json.Unmarshal(body, &rows) != nil {
		return points, values, errors.New("invalid original panel series")
	}
	for _, row := range rows {
		data := row.Values
		if len(row.Value) != 0 {
			data = append(data, row.Value)
		}
		for _, point := range data {
			var seconds float64
			var text string
			if len(point) != 2 || json.Unmarshal(point[0], &seconds) != nil || json.Unmarshal(point[1], &text) != nil {
				return points, values, errors.New("invalid numeric panel point")
			}
			number, err := strconv.ParseFloat(text, 64)
			if err != nil || math.IsNaN(number) || math.IsInf(number, 0) || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
				return points, values, errors.New("nonfinite panel point")
			}
			at := time.UnixMilli(int64(math.Round(seconds * 1000))).UTC()
			if points.Count == 0 || at.Before(points.First) {
				points.First = at
			}
			if points.Count == 0 || at.After(points.Last) {
				points.Last = at
			}
			points.Count++
			values.Min = math.Min(values.Min, number)
			values.Max = math.Max(values.Max, number)
		}
	}
	if points.Count == 0 {
		return points, values, errors.New("panel has no numeric observations")
	}
	return points, values, nil
}

func plan11MatchPanelClaim(claim plan11PanelClaim, unit, hash string, body []byte) error {
	points, values, err := plan11PanelOriginalFacts(body)
	if err != nil {
		return err
	}
	first, e1 := time.Parse(time.RFC3339Nano, claim.First)
	last, e2 := time.Parse(time.RFC3339Nano, claim.Last)
	close := func(value *float64, expected float64) bool {
		return value != nil && !math.IsNaN(*value) && !math.IsInf(*value, 0) && math.Abs(*value-expected) <= math.Max(1e-12, math.Abs(expected)*1e-9)
	}
	if claim.Hash != hash || claim.Unit != unit || claim.Count == nil || *claim.Count != points.Count || e1 != nil || e2 != nil || !first.Equal(points.First) || !last.Equal(points.Last) || !close(claim.Min, values.Min) || !close(claim.Max, values.Max) {
		return errors.New("panel facts differ from the sealed original output")
	}
	return nil
}

func plan11RecordEvidenceTimes(value any, allowed map[string]bool) {
	switch value := value.(type) {
	case string:
		if at, err := time.Parse(time.RFC3339Nano, value); err == nil {
			allowed[at.UTC().Format("15:04:05")] = true
		}
	case []any:
		for _, item := range value {
			plan11RecordEvidenceTimes(item, allowed)
		}
	case map[string]any:
		for _, item := range value {
			plan11RecordEvidenceTimes(item, allowed)
		}
	}
}

var plan11ClockClaim = regexp.MustCompile(`\b([0-2][0-9]:[0-5][0-9]:[0-5][0-9])(?:\.[0-9]+)?(?:Z)?`)

// These controlled tasks request UTC facts. A second-precision clock in chat
// or the delivered report must occur in an original point/window/manifest fact.
func plan11MatchEvidenceClockClaims(text string, allowed map[string]bool, emptyPrefixes ...plan11ComparisonWindow) error {
	for _, match := range plan11ClockClaim.FindAllStringSubmatchIndex(text, -1) {
		clock := text[match[2]:match[3]]
		if !allowed[clock] && !plan11VerifiedEmptyPrefixClock(text, match[2], match[3], emptyPrefixes) {
			return fmt.Errorf("unverified UTC time claim %s in dashboard narrative", clock)
		}
	}
	return nil
}

func (a *App) plan11VerifyAllDashboardClaims(ctx context.Context, env *E2EEnvironment, conversation string, jobs []dashboard.QueryJobView, claims []plan11DashboardClaim, emptyPrefix plan11ComparisonWindow, texts ...string) error {
	if len(claims) != len(jobs) {
		return errors.New("dashboard evidence must cover every executed job")
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	allowedTimes := map[string]bool{}
	for _, job := range jobs {
		if job.Manifest == nil {
			return errors.New("dashboard manifest unavailable for panel verification")
		}
		encoded, _ := json.Marshal(job.Manifest)
		var manifest any
		if json.Unmarshal(encoded, &manifest) != nil {
			return errors.New("invalid dashboard time evidence")
		}
		plan11RecordEvidenceTimes(manifest, allowedTimes)
		var claim *plan11DashboardClaim
		for i := range claims {
			if claims[i].JobID == job.ID.String() {
				if claim != nil {
					return errors.New("duplicate dashboard evidence")
				}
				claim = &claims[i]
			}
		}
		if claim == nil || claim.AttemptID != job.Manifest.AttemptID.String() {
			return errors.New("dashboard evidence selected another attempt")
		}
		files := 0
		for _, file := range job.Files {
			if file.Kind != "data" {
				continue
			}
			files++
			key := job.ID.String() + "/" + file.PanelID + "/" + file.TargetID
			if seen[key] || file.WorkspaceFileID == nil {
				return errors.New("duplicate or undelivered panel file")
			}
			seen[key] = true
			var panel *plan11PanelClaim
			for i := range claim.Panels {
				if claim.Panels[i].PanelID == file.PanelID && claim.Panels[i].TargetID == file.TargetID {
					if panel != nil {
						return errors.New("duplicate panel evidence")
					}
					panel = &claim.Panels[i]
				}
			}
			if panel == nil {
				return errors.New("dashboard evidence omitted a panel target")
			}
			body, err := plan11DownloadPanelOriginal(ctx, client, env, conversation, file)
			if err != nil {
				return err
			}
			unit := ""
			for _, definition := range job.Manifest.Definition.Panels {
				if definition.ID == file.PanelID {
					unit = definition.Unit
				}
			}
			if err = plan11MatchPanelClaim(*panel, unit, file.Hash, body); err != nil {
				return err
			}
			var original []struct {
				Values [][]json.RawMessage `json:"values"`
				Value  []json.RawMessage   `json:"value"`
			}
			if json.Unmarshal(body, &original) != nil {
				return errors.New("invalid panel point times")
			}
			for _, row := range original {
				points := row.Values
				if len(row.Value) > 0 {
					points = append(points, row.Value)
				}
				for _, point := range points {
					var seconds float64
					if len(point) != 2 || json.Unmarshal(point[0], &seconds) != nil {
						return errors.New("invalid original point timestamp")
					}
					at := time.UnixMilli(int64(math.Round(seconds * 1000))).UTC()
					allowedTimes[at.Format("15:04:05")] = true
					if at.Before(emptyPrefix.To) {
						emptyPrefix.To = at
					}
				}
			}
		}
		if files != len(claim.Panels) {
			return errors.New("dashboard evidence contains unobserved panel targets")
		}
	}
	for _, text := range texts {
		if err := plan11MatchEvidenceClockClaims(text, allowedTimes, emptyPrefix); err != nil {
			return err
		}
	}
	return nil
}
