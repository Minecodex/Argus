package argusdev

import (
	"encoding/json"
	"errors"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"
)

var plan11JSONEvidenceBlock = regexp.MustCompile("(?s)```json\\s*(.*?)```")

// Collect known evidence shapes instead of choosing the last JSON block.
// Repeated facts must agree; missing data and contradictory attempts are never
// filled from prose or silently selected from an arbitrary metric-name map.
func plan11ConclusionFromBlocks(text string) (plan11Conclusion, error) {
	var primary *plan11Conclusion
	dashboards := map[string]plan11DashboardClaim{}
	mergeDashboard := func(claim plan11DashboardClaim) error {
		if claim.JobID == "" || claim.AttemptID == "" {
			return errors.New("dashboard JSON lacks job/attempt identity")
		}
		claim.Panels = append([]plan11PanelClaim{}, claim.Panels...)
		slices.SortFunc(claim.Panels, func(a, b plan11PanelClaim) int {
			return strings.Compare(a.PanelID+"/"+a.TargetID, b.PanelID+"/"+b.TargetID)
		})
		for i := range claim.Panels {
			panel := &claim.Panels[i]
			if i > 0 && panel.PanelID == claim.Panels[i-1].PanelID && panel.TargetID == claim.Panels[i-1].TargetID {
				return errors.New("duplicate panel target in evidence JSON")
			}
			panel.Unit = plan11DisplayUnit(panel.Unit)
			panel.First = plan11EvidenceInstant(panel.First)
			panel.Last = plan11EvidenceInstant(panel.Last)
		}
		if previous, exists := dashboards[claim.JobID]; exists && !reflect.DeepEqual(previous, claim) {
			return errors.New("conflicting dashboard JSON facts or attempts")
		}
		dashboards[claim.JobID] = claim
		return nil
	}
	for _, block := range plan11JSONEvidenceBlock.FindAllStringSubmatch(text, -1) {
		raw := []byte(strings.TrimSpace(block[1]))
		var value any
		if json.Unmarshal(raw, &value) != nil {
			return plan11Conclusion{}, errors.New("invalid JSON evidence block")
		}
		switch value := value.(type) {
		case map[string]any:
			_, nested := value["metric_evidence"]
			_, resource := value["resource_id"]
			_, metric := value["metric"]
			_, panels := value["panels"]
			_, job := value["job_id"]
			if nested || resource && metric {
				candidate, err := plan11DecodeConclusion(raw)
				if err != nil {
					return plan11Conclusion{}, err
				}
				for _, dashboard := range candidate.Dashboards {
					if err := mergeDashboard(dashboard); err != nil {
						return plan11Conclusion{}, err
					}
				}
				candidate.Dashboards = nil
				if primary != nil && !plan11SamePrimaryEvidence(*primary, candidate) {
					return plan11Conclusion{}, errors.New("conflicting primary metric JSON facts")
				}
				primary = &candidate
			} else if panels && job {
				var claim plan11DashboardClaim
				if json.Unmarshal(raw, &claim) != nil {
					return plan11Conclusion{}, errors.New("invalid dashboard JSON object")
				}
				if err := mergeDashboard(claim); err != nil {
					return plan11Conclusion{}, err
				}
			} else if body, exists := value["dashboard_evidence"]; exists {
				encoded, _ := json.Marshal(body)
				var claims []plan11DashboardClaim
				if json.Unmarshal(encoded, &claims) != nil {
					return plan11Conclusion{}, errors.New("invalid dashboard evidence array")
				}
				for _, claim := range claims {
					if err := mergeDashboard(claim); err != nil {
						return plan11Conclusion{}, err
					}
				}
			}
		case []any:
			known := false
			for _, entry := range value {
				if object, ok := entry.(map[string]any); ok && (object["job_id"] != nil || object["attempt_id"] != nil || object["panels"] != nil) {
					known = true
				}
			}
			if !known {
				continue
			}
			for _, entry := range value {
				object, ok := entry.(map[string]any)
				if !ok || object["job_id"] == nil || object["attempt_id"] == nil || object["panels"] == nil {
					return plan11Conclusion{}, errors.New("unidentified JSON evidence array entry")
				}
				encoded, _ := json.Marshal(object)
				var claim plan11DashboardClaim
				if json.Unmarshal(encoded, &claim) != nil {
					return plan11Conclusion{}, errors.New("invalid dashboard JSON facts")
				}
				if err := mergeDashboard(claim); err != nil {
					return plan11Conclusion{}, err
				}
			}
		}
	}
	if primary == nil {
		return plan11Conclusion{}, errors.New("missing primary metric JSON conclusion")
	}
	ids := make([]string, 0, len(dashboards))
	for id := range dashboards {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		primary.Dashboards = append(primary.Dashboards, dashboards[id])
	}
	return *primary, nil
}

func plan11EvidenceInstant(value string) string {
	if at, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return at.UTC().Format(time.RFC3339Nano)
	}
	return value
}

func plan11SamePrimaryEvidence(a, b plan11Conclusion) bool {
	if len(a.Limitations) == 0 || len(b.Limitations) == 0 {
		return false
	}
	a.Limitations, b.Limitations = nil, nil
	a.FirstPointAt, b.FirstPointAt = plan11EvidenceInstant(a.FirstPointAt), plan11EvidenceInstant(b.FirstPointAt)
	a.LastPointAt, b.LastPointAt = plan11EvidenceInstant(a.LastPointAt), plan11EvidenceInstant(b.LastPointAt)
	a.Unit, b.Unit = plan11DisplayUnit(a.Unit), plan11DisplayUnit(b.Unit)
	a.DisplayUnit, b.DisplayUnit = plan11DisplayUnit(a.DisplayUnit), plan11DisplayUnit(b.DisplayUnit)
	a.Transform, b.Transform = plan11DisplayTransform(a.Transform), plan11DisplayTransform(b.Transform)
	return reflect.DeepEqual(a, b)
}
