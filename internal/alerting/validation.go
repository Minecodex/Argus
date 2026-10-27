package alerting

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"regexp"
	"slices"
	"strings"
	"text/template"

	"github.com/google/uuid"
	"github.com/prometheus/prometheus/promql/parser"
)

var labelKey = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_.\-/]*$`)

func decodeSpec[T any](raw json.RawMessage) (T, error) {
	var value T
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if len(raw) == 0 || d.Decode(&value) != nil {
		return value, ErrInvalid
	}
	if d.Decode(new(any)) != io.EOF {
		return value, ErrInvalid
	}
	return value, nil
}
func normalizeMutation(m Mutation) (Mutation, error) {
	m.Name = strings.TrimSpace(m.Name)
	if !slices.Contains([]string{"policy", "silence", "inhibition", "channel", "route", "settings", "template", "grouping"}, m.Kind) || !slices.Contains([]string{"create", "update", "delete", "enable", "disable"}, m.Operation) {
		return m, ErrInvalid
	}
	if m.Operation != "create" && (m.ID == uuid.Nil || m.ExpectedVersion < 1) {
		return m, ErrInvalid
	}
	if m.Operation == "delete" || m.Operation == "enable" || m.Operation == "disable" {
		return m, nil
	}
	if m.Name == "" || len(m.Name) > 200 {
		return m, ErrInvalid
	}
	var normalized any
	switch m.Kind {
	case "policy":
		p, err := decodeSpec[PolicySpec](m.Spec)
		if err != nil {
			return m, err
		}
		if p.Mode == "" {
			p.Mode = "independent"
		}
		if p.Resources.Mode == "" {
			p.Resources.Mode = "dynamic"
		}
		if len(p.Resources.Types) == 0 {
			p.Resources.Types = []string{"host", "kubernetes_cluster"}
		}
		if len(p.MatchBy) == 0 {
			p.MatchBy = []string{"argus_resource_id", "argus_source_id"}
		}
		if p.IntervalSeconds == 0 {
			p.IntervalSeconds = 30
		}
		if p.NoDataBehavior == "" {
			p.NoDataBehavior = "report"
		}
		if p.ErrorBehavior == "" {
			p.ErrorBehavior = "keep"
		}
		if !slices.Contains([]string{"independent", "any", "every"}, p.Mode) || !slices.Contains([]string{"dynamic", "explicit"}, p.Resources.Mode) || p.IntervalSeconds < 10 || p.IntervalSeconds > 3600 || p.ForSeconds < 0 || p.ForSeconds > 86400 || p.RecoverySeconds < 0 || p.RecoverySeconds > 86400 || len(p.Rules) == 0 || len(p.Rules) > 32 || !slices.Contains([]string{"report", "keep"}, p.NoDataBehavior) || !slices.Contains([]string{"report", "keep"}, p.ErrorBehavior) {
			return m, ErrInvalid
		}
		if p.Resources.Mode == "explicit" && len(p.Resources.IDs) == 0 {
			return m, ErrInvalid
		}
		for _, kind := range p.Resources.Types {
			if kind != "host" && kind != "kubernetes_cluster" && kind != "application" && kind != "monitoring_target" {
				return m, ErrInvalid
			}
		}
		if err := validateMatchers(p.Resources.Matchers); err != nil {
			return m, err
		}
		if len(p.MatchBy) > 12 {
			return m, ErrInvalid
		}
		for _, key := range p.MatchBy {
			if !labelKey.MatchString(key) {
				return m, ErrInvalid
			}
		}
		ruleIDs := map[uuid.UUID]bool{}
		for i := range p.Rules {
			r := &p.Rules[i]
			if r.ID == uuid.Nil {
				// A full update must preserve explicit rule identities. Missing
				// identity cannot safely mean either an existing or a new rule.
				if m.Operation == "update" {
					return m, ErrInvalid
				}
				r.ID = uuid.New()
			}
			if ruleIDs[r.ID] {
				return m, ErrInvalid
			}
			ruleIDs[r.ID] = true
			if r.QueryMode == "" {
				r.QueryMode = "numeric"
			}
			if r.Name == "" {
				r.Name = r.Expression
			}
			if err := validateExpression(r.Expression); err != nil {
				return m, err
			}
			if !slices.Contains([]string{"numeric", "predicate"}, r.QueryMode) || !slices.Contains([]string{"critical", "warning", "info"}, r.Severity) || !slices.Contains([]string{">", ">=", "<", "<=", "==", "!="}, r.Operator) || math.IsNaN(r.Threshold) || math.IsInf(r.Threshold, 0) || math.IsNaN(r.RecoveryThreshold) || math.IsInf(r.RecoveryThreshold, 0) {
				return m, ErrInvalid
			}
			if r.QueryMode == "predicate" && p.NoDataBehavior == "report" && r.PresenceExpression == "" {
				return m, ErrInvalid
			}
			if r.PresenceExpression != "" {
				if err := validateExpression(r.PresenceExpression); err != nil {
					return m, err
				}
			}
			if r.QueryMode == "numeric" && ((strings.HasPrefix(r.Operator, ">") && r.RecoveryThreshold >= r.Threshold) || (strings.HasPrefix(r.Operator, "<") && r.RecoveryThreshold < r.Threshold)) {
				return m, ErrInvalid
			}
		}
		if p.ChannelIDs == nil {
			p.ChannelIDs = []uuid.UUID{}
		}
		if p.RouteIDs == nil {
			p.RouteIDs = []uuid.UUID{}
		}
		if p.Resources.IDs == nil {
			p.Resources.IDs = []uuid.UUID{}
		}
		if p.Resources.SourceIDs == nil {
			p.Resources.SourceIDs = []uuid.UUID{}
		}
		if p.Resources.Matchers == nil {
			p.Resources.Matchers = []Matcher{}
		}
		if p.Labels == nil {
			p.Labels = map[string]string{}
		}
		for k, v := range p.Labels {
			if !labelKey.MatchString(k) || strings.HasPrefix(k, "argus_") || len(v) > 256 {
				return m, ErrInvalid
			}
		}
		normalized = p
	case "silence":
		var scope map[string]json.RawMessage
		if json.Unmarshal(m.Spec, &scope) != nil || len(scope["resource_ids"]) == 0 || string(scope["resource_ids"]) == "null" {
			return m, ErrInvalid
		}
		v, err := decodeSpec[SilenceSpec](m.Spec)
		if err != nil {
			return m, err
		}
		if v.EndsAt.IsZero() || !v.EndsAt.After(v.StartsAt) || strings.TrimSpace(v.Reason) == "" || len(v.Reason) > 2000 {
			return m, ErrInvalid
		}
		if err := validateMatchers(v.Matchers); err != nil {
			return m, err
		}
		if v.ResourceIDs == nil {
			v.ResourceIDs = []uuid.UUID{}
		}
		if v.PolicyIDs == nil {
			v.PolicyIDs = []uuid.UUID{}
		}
		if v.Matchers == nil {
			v.Matchers = []Matcher{}
		}
		normalized = v
	case "inhibition":
		var scope map[string]json.RawMessage
		if json.Unmarshal(m.Spec, &scope) != nil || len(scope["resource_ids"]) == 0 || string(scope["resource_ids"]) == "null" {
			return m, ErrInvalid
		}
		v, err := decodeSpec[InhibitionSpec](m.Spec)
		if err != nil {
			return m, err
		}
		if len(v.Source) == 0 || len(v.Target) == 0 || len(v.Equal) == 0 || strings.TrimSpace(v.Reason) == "" {
			return m, ErrInvalid
		}
		if !slices.Contains(v.Equal, "resource_id") && !slices.Contains(v.Equal, "argus_resource_id") {
			return m, ErrInvalid
		}
		if err := validateMatchers(v.Source); err != nil {
			return m, err
		}
		if err := validateMatchers(v.Target); err != nil {
			return m, err
		}
		if v.ResourceIDs == nil {
			v.ResourceIDs = []uuid.UUID{}
		}
		normalized = v
	case "channel":
		v, err := decodeSpec[ChannelSpec](m.Spec)
		if err != nil {
			return m, err
		}
		if !slices.Contains([]string{"dingtalk", "wecom", "feishu", "smtp"}, v.Provider) || v.CredentialID == uuid.Nil || len(v.MentionIDs) > 100 {
			return m, ErrInvalid
		}
		if len(v.Template) > 16384 {
			return m, ErrInvalid
		}
		if v.Template != "" {
			if _, err := template.New("notification").Option("missingkey=error").Parse(normalizeTemplate(v.Template)); err != nil {
				return m, ErrInvalid
			}
		}
		// A display address is descriptive only and must never contain tokens.
		if strings.Contains(v.DisplayAddress, "?") || strings.Contains(v.DisplayAddress, "/hook/") || strings.Contains(v.DisplayAddress, "SEC") {
			return m, ErrInvalid
		}
		if v.MentionIDs == nil {
			v.MentionIDs = []string{}
		}
		normalized = v
	case "route":
		v, err := decodeSpec[RouteSpec](m.Spec)
		if err != nil {
			return m, err
		}
		if len(v.ChannelIDs) == 0 || len(v.ChannelIDs) > 32 {
			return m, ErrInvalid
		}
		if err := validateMatchers(v.Matchers); err != nil {
			return m, err
		}
		if v.Matchers == nil {
			v.Matchers = []Matcher{}
		}
		normalized = v
	case "settings":
		v, err := decodeSpec[SettingsSpec](m.Spec)
		if err != nil || v.EvaluatorID == uuid.Nil {
			return m, ErrInvalid
		}
		normalized = v
	case "grouping":
		v, err := decodeSpec[GroupingSpec](m.Spec)
		if err != nil {
			return m, err
		}
		if len(v.GroupBy) > 12 || v.WaitSeconds < 0 || v.WaitSeconds > 3600 || v.IntervalSeconds < 1 || v.IntervalSeconds > 86400 || v.RepeatSeconds < 1 || v.RepeatSeconds > 604800 {
			return m, ErrInvalid
		}
		for _, key := range v.GroupBy {
			if !labelKey.MatchString(key) {
				return m, ErrInvalid
			}
		}
		for _, key := range v.Timing {
			if !slices.Contains([]string{"first", "repeat", "recovery", "severity"}, key) {
				return m, ErrInvalid
			}
		}
		normalized = v
	case "template":
		v, err := decodeSpec[TemplateSpec](m.Spec)
		if err != nil || len(v.Content) > 16384 || strings.TrimSpace(v.Content) == "" {
			return m, ErrInvalid
		}
		if _, err := template.New("notification").Option("missingkey=error").Parse(normalizeTemplate(v.Content)); err != nil {
			return m, ErrInvalid
		}
		normalized = v
	}
	var err error
	m.Spec, err = json.Marshal(normalized)
	return m, err
}
func validateExpression(expr string) error {
	if len(expr) == 0 || len(expr) > 8192 {
		return ErrInvalid
	}
	node, err := parser.NewParser(parser.Options{}).ParseExpr(expr)
	if err != nil || node.Type() != parser.ValueTypeVector && node.Type() != parser.ValueTypeScalar {
		return ErrInvalid
	}
	return nil
}
func validateMatchers(ms []Matcher) error {
	if len(ms) > 32 {
		return ErrInvalid
	}
	for _, m := range ms {
		if !labelKey.MatchString(m.Key) || len(m.Value) > 512 || !slices.Contains([]string{"=", "!=", "=~", "!~"}, m.Operator) {
			return ErrInvalid
		}
		if strings.Contains(m.Operator, "~") {
			if _, err := regexp.Compile("^(?:" + m.Value + ")$"); err != nil {
				return ErrInvalid
			}
		}
	}
	return nil
}
func matches(labels map[string]string, ms []Matcher) bool {
	for _, m := range ms {
		v := labels[m.Key]
		hit := v == m.Value
		if strings.Contains(m.Operator, "~") {
			re, err := regexp.Compile("^(?:" + m.Value + ")$")
			if err != nil {
				return false
			}
			hit = re.MatchString(v)
		}
		if m.Operator == "!=" || m.Operator == "!~" {
			hit = !hit
		}
		if !hit {
			return false
		}
	}
	return true
}
func requiredPermission(kind string) string {
	switch kind {
	case "policy":
		return "alert.policy.manage"
	case "settings":
		return "service_account.manage"
	case "silence", "inhibition":
		return "alert.silence.manage"
	default:
		return "alert.notification.manage"
	}
}

func ConfigurationReadPermission(kind string) string {
	if kind == "settings" {
		return "service_account.read"
	}
	return "alert.read"
}

var templateVariable = regexp.MustCompile(`\{\{\s*([a-z_]+)\s*\}\}`)

func normalizeTemplate(pattern string) string {
	return templateVariable.ReplaceAllString(pattern, "{{.$1}}")
}
