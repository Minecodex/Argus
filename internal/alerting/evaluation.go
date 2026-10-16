package alerting

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
)

type Evaluation struct {
	SourcesCaptured bool                       `json:"sources_captured"`
	Outcomes        []EvaluationOutcome        `json:"outcomes"`
	Observations    []Observation              `json:"observations"`
	Health          string                     `json:"health"`
	ErrorCode       string                     `json:"error_code"`
	ResourceIDs     []uuid.UUID                `json:"resource_ids"`
	Sources         []db.AlertMetricSourcesRow `json:"sources"`
	Evaluator       Actor                      `json:"-"`
	Slot            time.Time                  `json:"slot"`
}

type EvaluationOutcome struct {
	RuleID      uuid.UUID   `json:"rule_id"`
	ResourceIDs []uuid.UUID `json:"resource_ids"`
	SourceIDs   []uuid.UUID `json:"source_ids"`
	Health      string      `json:"health"`
	ErrorCode   string      `json:"error_code,omitempty"`
	Complete    bool        `json:"complete"`
}

func (s Service) Trial(ctx context.Context, a Actor, p PolicySpec) (Evaluation, error) {
	if err := authorize(ctx, s.Store.Queries, a, "alert.policy.manage"); err != nil {
		return Evaluation{}, err
	}
	raw, _ := json.Marshal(p)
	m, err := normalizeMutation(Mutation{Kind: "policy", Operation: "create", Name: "Trial", Spec: raw})
	if err != nil {
		return Evaluation{}, err
	}
	p, _ = decodeSpec[PolicySpec](m.Spec)
	eval, err := evaluator(ctx, s.Store.Queries, a.EnterpriseID)
	if err != nil {
		return Evaluation{}, err
	}
	ids, err := selectedResources(ctx, s.Store.Queries, eval, p.Resources)
	if err != nil {
		return Evaluation{}, err
	}
	allowed, err := actorResources(ctx, s.Store.Queries, a)
	if err != nil || !containsAll(allowed, ids) {
		return Evaluation{}, ErrDenied
	}
	return s.evaluate(ctx, eval, p, ids, time.Now().UTC())
}
func (s Service) evaluate(ctx context.Context, a Actor, p PolicySpec, ids []uuid.UUID, slot time.Time) (Evaluation, error) {
	result := Evaluation{Observations: []Observation{}, Outcomes: []EvaluationOutcome{}, Sources: []db.AlertMetricSourcesRow{}, Health: "normal", ResourceIDs: ids, Evaluator: a, Slot: slot}
	if len(ids) == 0 {
		result.Health = "no_data"
		result.ErrorCode = "ALERT_SCOPE_EMPTY"
		return result, nil
	}
	sources, err := metricSources(ctx, s.Store.Queries, a.EnterpriseID, ids, p.Resources.SourceIDs)
	if err != nil {
		return Evaluation{}, err
	}
	result.Sources = sources
	result.SourcesCaptured = true
	if len(sources) == 0 {
		result.Health = "no_data"
		result.ErrorCode = "ALERT_SOURCES_EMPTY"
		for _, rule := range p.Rules {
			result.Outcomes = append(result.Outcomes, EvaluationOutcome{RuleID: rule.ID, ResourceIDs: ids, SourceIDs: []uuid.UUID{}, Health: result.Health, ErrorCode: result.ErrorCode, Complete: false})
		}
		return result, nil
	}
	byRule := make([][]Observation, len(p.Rules))
	for index, rule := range p.Rules {
		for _, id := range ids {
			if !slices.ContainsFunc(sources, func(source db.AlertMetricSourcesRow) bool { return source.ResourceID == id }) {
				if result.Health != "error" {
					result.Health = "no_data"
					result.ErrorCode = "ALERT_RESOURCE_SOURCES_EMPTY"
				}
				result.Outcomes = append(result.Outcomes, EvaluationOutcome{RuleID: rule.ID, ResourceIDs: []uuid.UUID{id}, SourceIDs: []uuid.UUID{}, Health: "no_data", ErrorCode: "ALERT_RESOURCE_SOURCES_EMPTY", Complete: false})
			}
		}
		observations, err := s.queryRule(ctx, a, rule, p.MatchBy, ids, sourceKeys(sources), slot)
		if err != nil {
			result.Health = "error"
			result.ErrorCode = "ALERT_QUERY_FAILED"
			result.Outcomes = append(result.Outcomes, EvaluationOutcome{RuleID: rule.ID, ResourceIDs: ids, SourceIDs: []uuid.UUID{}, Health: "error", ErrorCode: result.ErrorCode, Complete: false})
			continue
		}
		if len(observations) == 0 {
			if result.Health != "error" {
				result.Health = "no_data"
				result.ErrorCode = "ALERT_RULE_NO_DATA"
			}
			result.Outcomes = append(result.Outcomes, EvaluationOutcome{RuleID: rule.ID, ResourceIDs: ids, SourceIDs: []uuid.UUID{}, Health: "no_data", ErrorCode: "ALERT_RULE_NO_DATA", Complete: false})
			continue
		}
		for i := range observations {
			obs := &observations[i]
			participating := []db.AlertMetricSourcesRow{}
			for _, source := range sources {
				if slices.Contains(obs.ResourceIDs, source.ResourceID) && (len(obs.SourceIDs) == 0 || slices.Contains(obs.SourceIDs, source.ID)) {
					participating = append(participating, source)
				}
			}
			if len(obs.SourceIDs) > 0 && len(participating) != len(obs.SourceIDs) {
				return result, ErrDenied
			}
			identity := []string{}
			for _, source := range participating {
				identity = append(identity, source.ID.String()+":"+source.Generation.String())
			}
			obs.Key += "\nsource_scope=" + strings.Join(identity, ",")
			result.Outcomes = append(result.Outcomes, EvaluationOutcome{RuleID: rule.ID, ResourceIDs: obs.ResourceIDs, SourceIDs: obs.SourceIDs, Health: "normal", Complete: true})
		}
		for _, source := range sources {
			covered := false
			for _, obs := range observations {
				if slices.Contains(obs.ResourceIDs, source.ResourceID) && (len(obs.SourceIDs) == 0 || slices.Contains(obs.SourceIDs, source.ID)) {
					covered = true
					break
				}
			}
			if !covered {
				if result.Health != "error" {
					result.Health = "no_data"
					result.ErrorCode = "ALERT_MEMBER_NO_DATA"
				}
				result.Outcomes = append(result.Outcomes, EvaluationOutcome{RuleID: rule.ID, ResourceIDs: []uuid.UUID{source.ResourceID}, SourceIDs: []uuid.UUID{source.ID}, Health: "no_data", ErrorCode: "ALERT_MEMBER_NO_DATA", Complete: false})
			}
		}
		byRule[index] = observations
	}
	result.Observations = combineObservations(p, byRule)
	if len(result.Observations) == 0 && result.Health == "normal" {
		result.Health = "no_data"
		result.ErrorCode = "ALERT_NO_DATA"
	}
	if err := s.Telemetry.ValidateQueryScope(ctx, telemetry.Actor{EnterpriseID: a.EnterpriseID, SubjectID: a.SubjectID, SubjectType: a.SubjectType, AuthorizationVersion: a.AuthorizationVersion}, ids); err != nil {
		return Evaluation{}, ErrDenied
	}
	return result, nil
}
func (s Service) queryRule(ctx context.Context, a Actor, r Rule, matchBy []string, ids []uuid.UUID, sources []string, slot time.Time) ([]Observation, error) {
	if s.Telemetry.Engine == nil {
		return nil, ErrUnavailable
	}
	query := func(expression string) ([]Observation, error) {
		value, err := s.Telemetry.Engine.ExecuteEngineQuery(ctx, queryengine.Request{Language: queryengine.LanguagePromQL, Expression: expression, Instant: true, Start: slot.Add(-5 * time.Minute), End: slot, Scope: queryengine.Scope{EnterpriseID: a.EnterpriseID, SubjectID: a.SubjectID, SubjectType: a.SubjectType, AuthorizationVersion: a.AuthorizationVersion, ResourceIDs: ids, SourceKeys: sources}, Budget: queryengine.Budget{MaxRows: 10000, MaxSeries: 10000, MaxSamples: 1000000, MaxScanBytes: telemetry.DefaultMaxScanBytes, MaxResultBytes: 8 << 20, Timeout: 10 * time.Second}})
		if err != nil || value.Meta.Partial {
			return nil, ErrUnavailable
		}
		observations, e := decodeObservations(value, r, matchBy, ids)
		if e != nil {
			return nil, e
		}
		seen := map[string]bool{}
		for _, obs := range observations {
			if seen[obs.Key] {
				return nil, ErrInvalid
			}
			seen[obs.Key] = true
		}
		return observations, nil
	}
	values, err := query(r.Expression)
	if err != nil {
		return nil, err
	}
	if r.QueryMode == "predicate" {
		fired := map[string]Observation{}
		for index, v := range values {
			v.Firing = true
			v.Recovering = false
			fired[v.Key] = v
			values[index] = v
		}
		if r.PresenceExpression == "" {
			return values, nil
		}
		presence, err := query(r.PresenceExpression)
		if err != nil {
			return nil, err
		}
		for i, v := range presence {
			if matched, ok := fired[v.Key]; ok {
				presence[i] = matched
			} else {
				presence[i].Firing = false
				presence[i].Recovering = true
			}
		}
		return presence, nil
	}
	return values, nil
}
func decodeObservations(result queryengine.Result, r Rule, matchBy []string, ids []uuid.UUID) ([]Observation, error) {
	raw, err := json.Marshal(result.Data)
	if err != nil {
		return nil, err
	}
	var data struct {
		ResultType string          `json:"resultType"`
		Result     json.RawMessage `json:"result"`
		Data       json.RawMessage `json:"data"`
	}
	if len(raw) > 0 && raw[0] == '[' {
		data.ResultType = result.ResultType
		data.Result = raw
	} else if json.Unmarshal(raw, &data) != nil {
		return nil, ErrInvalid
	}
	if len(data.Data) > 0 {
		if json.Unmarshal(data.Data, &data) != nil {
			return nil, ErrInvalid
		}
	}
	if data.ResultType == "" {
		data.ResultType = result.ResultType
	}
	observations := []Observation{}
	add := func(labels map[string]string, value []json.RawMessage) error {
		if len(value) != 2 {
			return ErrInvalid
		}
		var number string
		if json.Unmarshal(value[1], &number) != nil {
			return ErrInvalid
		}
		n, err := strconv.ParseFloat(number, 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return ErrInvalid
		}
		resources := append([]uuid.UUID{}, ids...)
		sources := []uuid.UUID{}
		if text := labels["argus_resource_id"]; text != "" {
			id, err := uuid.Parse(text)
			if err != nil || !slices.Contains(ids, id) {
				return ErrDenied
			}
			resources = []uuid.UUID{id}
			labels["resource_id"] = id.String()
		}
		if text := labels["argus_source_id"]; text != "" {
			id, err := uuid.Parse(text)
			if err != nil {
				return ErrInvalid
			}
			sources = append(sources, id)
		}
		obs := Observation{Labels: labels, Value: n, RuleID: r.ID, ResourceIDs: resources, SourceIDs: sources, Firing: compare(n, r.Operator, r.Threshold), Recovering: recovered(n, r), Severity: r.Severity, Rule: r}
		// Resource and source provenance cannot be removed by user dimensions.
		parts := []string{"resources=" + strings.Join(uuidStrings(resources), ","), "sources=" + strings.Join(uuidStrings(sources), ",")}
		for _, key := range matchBy {
			parts = append(parts, key+"="+labels[key])
		}
		if labels["argus_resource_id"] == "" {
			for _, id := range resources {
				parts = append(parts, id.String())
			}
		}
		obs.Key = strings.Join(parts, "\n")
		observations = append(observations, obs)
		return nil
	}
	switch data.ResultType {
	case "scalar":
		var tuple []json.RawMessage
		if json.Unmarshal(data.Result, &tuple) != nil {
			return nil, ErrInvalid
		}
		if err = add(map[string]string{}, tuple); err != nil {
			return nil, err
		}
	case "vector":
		var values []struct {
			Metric    map[string]string `json:"metric"`
			Value     []json.RawMessage `json:"value"`
			Histogram json.RawMessage   `json:"histogram"`
		}
		if json.Unmarshal(data.Result, &values) != nil {
			return nil, ErrInvalid
		}
		for _, v := range values {
			if len(v.Histogram) > 0 {
				return nil, ErrInvalid
			}
			if v.Metric == nil {
				v.Metric = map[string]string{}
			}
			if err = add(v.Metric, v.Value); err != nil {
				return nil, err
			}
		}
	default:
		return nil, ErrInvalid
	}
	return observations, nil
}
func compare(value float64, op string, threshold float64) bool {
	switch op {
	case ">":
		return value > threshold
	case ">=":
		return value >= threshold
	case "<":
		return value < threshold
	case "<=":
		return value <= threshold
	case "==":
		return value == threshold
	case "!=":
		return value != threshold
	}
	return false
}
func recovered(value float64, r Rule) bool {
	switch r.Operator {
	case ">", ">=":
		return value <= r.RecoveryThreshold
	case "<", "<=":
		return value >= r.RecoveryThreshold
	default:
		return !compare(value, r.Operator, r.Threshold)
	}
}
func combineObservations(p PolicySpec, rules [][]Observation) []Observation {
	out := []Observation{}
	if p.Mode == "independent" {
		for _, values := range rules {
			for _, v := range values {
				v.Key = v.RuleID.String() + ":" + v.Key
				out = append(out, v)
			}
		}
		return out
	}
	groups := map[string][]Observation{}
	invalid := map[string]bool{}
	seen := map[string]map[uuid.UUID]bool{}
	for _, values := range rules {
		for _, v := range values {
			if seen[v.Key] == nil {
				seen[v.Key] = map[uuid.UUID]bool{}
			}
			if seen[v.Key][v.RuleID] {
				invalid[v.Key] = true
			}
			seen[v.Key][v.RuleID] = true
			groups[v.Key] = append(groups[v.Key], v)
		}
	}
	keys := []string{}
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		group := groups[key]
		if invalid[key] || len(group) != len(p.Rules) {
			continue
		}
		primary := group[0]
		for _, obs := range group {
			if obs.Firing && (!primary.Firing || severityRank(obs.Severity) > severityRank(primary.Severity)) {
				primary = obs
			}
		}
		merged := primary
		merged.Members = []MemberValue{}
		merged.RuleID = uuid.Nil
		merged.ResourceIDs = []uuid.UUID{}
		merged.SourceIDs = []uuid.UUID{}
		merged.Firing = p.Mode == "every"
		merged.Recovering = p.Mode == "any"
		for _, obs := range group {
			merged.Members = append(merged.Members, MemberValue{RuleID: obs.RuleID, Name: obs.Rule.Name, Expression: obs.Rule.Expression, Value: obs.Value, Threshold: obs.Rule.Threshold, RecoveryThreshold: obs.Rule.RecoveryThreshold, Severity: obs.Severity, Firing: obs.Firing, Recovering: obs.Recovering, Unit: obs.Rule.Unit})
			merged.ResourceIDs = append(merged.ResourceIDs, obs.ResourceIDs...)
			merged.SourceIDs = append(merged.SourceIDs, obs.SourceIDs...)
			if p.Mode == "every" {
				merged.Firing = merged.Firing && obs.Firing
				merged.Recovering = merged.Recovering || obs.Recovering
			} else {
				merged.Firing = merged.Firing || obs.Firing
				merged.Recovering = merged.Recovering && obs.Recovering
			}
			if (p.Mode == "every" || obs.Firing) && severityRank(obs.Severity) > severityRank(merged.Severity) {
				merged.Severity = obs.Severity
			}
		}
		sort.Slice(merged.ResourceIDs, func(i, j int) bool { return merged.ResourceIDs[i].String() < merged.ResourceIDs[j].String() })
		merged.ResourceIDs = slices.Compact(merged.ResourceIDs)
		sort.Slice(merged.SourceIDs, func(i, j int) bool { return merged.SourceIDs[i].String() < merged.SourceIDs[j].String() })
		merged.SourceIDs = slices.Compact(merged.SourceIDs)
		merged.Key = "composite:" + key
		out = append(out, merged)
	}
	return out
}
func severityRank(value string) int {
	switch value {
	case "critical":
		return 3
	case "warning":
		return 2
	default:
		return 1
	}
}
func fingerprint(policy uuid.UUID, generation int64, key string) string {
	raw, _ := json.Marshal([]any{policy, generation, key})
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}
