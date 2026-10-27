package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/prometheus/prometheus/promql/parser"
)

func eventLabels(i Instance) map[string]string {
	labels := map[string]string{}
	for k, v := range i.State.Labels {
		labels[k] = v
	}
	labels["severity"] = i.Severity
	labels["policy_id"] = i.PolicyID.String()
	labels["metric"] = i.State.Metric
	if len(i.ResourceIDs) == 1 {
		labels["resource_id"] = i.ResourceIDs[0].String()
		labels["argus_resource_id"] = i.ResourceIDs[0].String()
	}
	return labels
}
func noiseDecision(instance Instance, all []Instance, silences []Configuration, inhibitions []Configuration, at time.Time) (string, []string) {
	labels := eventLabels(instance)
	for _, row := range silences {
		s, err := decodeSpec[SilenceSpec](row.Spec)
		if err != nil || !s.Enabled || at.Before(s.StartsAt) || !at.Before(s.EndsAt) {
			continue
		}
		if len(s.PolicyIDs) > 0 && !slices.Contains(s.PolicyIDs, instance.PolicyID) || len(s.ResourceIDs) > 0 && !containsAll(s.ResourceIDs, instance.ResourceIDs) || s.Metric != "" && !instanceMetricMatches(instance, s.Metric) || !matches(labels, s.Matchers) {
			continue
		}
		return "muted", []string{fmt.Sprintf("%s:v%d: %s", row.ID, row.Version, s.Reason)}
	}
	for _, row := range inhibitions {
		s, err := decodeSpec[InhibitionSpec](row.Spec)
		if err != nil || !s.Enabled || len(s.ResourceIDs) > 0 && !containsAll(s.ResourceIDs, instance.ResourceIDs) || !matches(labels, s.Target) {
			continue
		}
		for _, source := range all {
			if source.ID == instance.ID || source.Lifecycle != "firing" && source.Lifecycle != "recovering" {
				continue
			}
			if len(s.ResourceIDs) > 0 && !containsAll(s.ResourceIDs, source.ResourceIDs) {
				continue
			}
			sourceLabels := eventLabels(source)
			if !matches(sourceLabels, s.Source) {
				continue
			}
			equal := true
			for _, key := range s.Equal {
				if labels[key] == "" || sourceLabels[key] == "" || labels[key] != sourceLabels[key] {
					equal = false
					break
				}
			}
			if equal {
				return "inhibited", []string{fmt.Sprintf("%s:v%d: %s", row.ID, row.Version, s.Reason)}
			}
		}
	}
	return "pending", []string{}
}

func instanceMetricMatches(i Instance, metric string) bool {
	expressions := []string{i.State.Metric}
	for _, member := range i.State.Members {
		expressions = append(expressions, member.Expression)
	}
	for _, expression := range expressions {
		node, err := parser.NewParser(parser.Options{}).ParseExpr(expression)
		if err != nil {
			continue
		}
		for _, selector := range parser.ExtractSelectors(node) {
			for _, matcher := range selector {
				if matcher.Name == "__name__" && matcher.Matches(metric) {
					return true
				}
			}
		}
	}
	return false
}
func loadConfigurations(ctx context.Context, q *db.Queries, enterprise uuid.UUID, kind string) ([]Configuration, error) {
	rows, err := q.ListAlertConfigurations(ctx, db.ListAlertConfigurationsParams{EnterpriseID: enterprise, Kind: kind})
	out := []Configuration{}
	for _, row := range rows {
		out = append(out, configurationDTO(row))
	}
	return out, err
}
func defaultGrouping() GroupingSpec {
	return GroupingSpec{GroupBy: []string{"policy_id", "resource_id"}, WaitSeconds: 30, IntervalSeconds: 300, RepeatSeconds: 3600, Timing: []string{"first", "repeat", "recovery", "severity"}}
}
func groupingConfiguration(ctx context.Context, q *db.Queries, enterprise uuid.UUID) (GroupingSpec, error) {
	rows, err := loadConfigurations(ctx, q, enterprise, "grouping")
	if err != nil {
		return GroupingSpec{}, err
	}
	if len(rows) == 0 {
		return defaultGrouping(), nil
	}
	return decodeSpec[GroupingSpec](rows[0].Spec)
}
func planNotifications(ctx context.Context, q *db.Queries, policy db.AlertConfiguration, p PolicySpec, events []db.AlertEvent, at time.Time) error {
	silences, err := loadConfigurations(ctx, q, policy.EnterpriseID, "silence")
	if err != nil {
		return err
	}
	inhibitions, err := loadConfigurations(ctx, q, policy.EnterpriseID, "inhibition")
	if err != nil {
		return err
	}
	rows, err := q.ListAlertInstances(ctx, policy.EnterpriseID)
	if err != nil {
		return err
	}
	all := []Instance{}
	for _, row := range rows {
		item, e := instanceDTO(row)
		if e != nil {
			return e
		}
		all = append(all, item)
	}
	grouping, err := groupingConfiguration(ctx, q, policy.EnterpriseID)
	if err != nil {
		return err
	}
	templates, err := loadConfigurations(ctx, q, policy.EnterpriseID, "template")
	if err != nil {
		return err
	}
	defaultTemplate := ""
	if len(templates) > 0 {
		v, e := decodeSpec[TemplateSpec](templates[0].Spec)
		if e != nil {
			return e
		}
		defaultTemplate = v.Content
	}
	routes, err := loadConfigurations(ctx, q, policy.EnterpriseID, "route")
	if err != nil {
		return err
	}
	sort.SliceStable(routes, func(i, j int) bool {
		a, _ := decodeSpec[RouteSpec](routes[i].Spec)
		b, _ := decodeSpec[RouteSpec](routes[j].Spec)
		if a.Priority == b.Priority {
			return routes[i].ID.String() < routes[j].ID.String()
		}
		return a.Priority < b.Priority
	})
	// Re-evaluate notification state for every current episode, so a new
	// silence/inhibition becomes visible even without another firing event.
	for _, row := range rows {
		if row.PolicyID != policy.ID || row.Generation != policy.Generation || row.Lifecycle == "normal" || row.Lifecycle == "resolved" || row.Lifecycle == "closed" {
			continue
		}
		instance, e := instanceDTO(row)
		if e != nil {
			return e
		}
		if instance.Health == "not_evaluated" {
			continue
		}
		decision, reasons := noiseDecision(instance, all, silences, inhibitions, at)
		previous := instance.State.NotificationState
		transition := ""
		if decision != previous {
			if decision == "muted" || decision == "inhibited" {
				transition = decision
			} else if previous == "muted" {
				transition = "unmuted"
			} else if previous == "inhibited" {
				transition = "uninhibited"
			}
		}
		if decision != "pending" || previous == "muted" || previous == "inhibited" {
			instance.State.NotificationState = decision
			instance.State.NoiseReasons = reasons
			raw, _ := json.Marshal(instance.State)
			row, e = q.UpdateAlertInstance(ctx, db.UpdateAlertInstanceParams{EnterpriseID: row.EnterpriseID, ID: row.ID, State: raw, Lifecycle: row.Lifecycle, Health: row.Health, Severity: row.Severity, StartedAt: row.StartedAt, ResolvedAt: row.ResolvedAt, LastEvaluatedAt: row.LastEvaluatedAt, Version: row.Version})
			if e != nil {
				return e
			}
			if transition != "" {
				if _, e = appendLifecycleEvent(ctx, q, row, transition, at); e != nil {
					return e
				}
			}
		}
		if row.Lifecycle == "firing" && row.Health == "normal" && decision == "pending" && (len(p.ChannelIDs) > 0 || len(routes) > 0) && (instance.State.LastNotificationAt == nil || at.Sub(*instance.State.LastNotificationAt) >= time.Duration(grouping.RepeatSeconds)*time.Second) {
			has := false
			for _, event := range events {
				if event.InstanceID == row.ID {
					has = true
				}
			}
			if !has && slices.Contains(grouping.Timing, "repeat") {
				event, e := appendLifecycleEvent(ctx, q, row, "repeat", at)
				if e != nil {
					return e
				}
				events = append(events, event)
			}
		}
	}
	for _, event := range events {
		if !slices.Contains(grouping.Timing, event.EventType) {
			continue
		}
		row, e := q.GetAlertInstance(ctx, db.GetAlertInstanceParams{EnterpriseID: policy.EnterpriseID, ID: event.InstanceID})
		if e != nil {
			return e
		}
		instance, e := instanceDTO(row)
		if e != nil {
			return e
		}
		if instance.Health == "not_evaluated" {
			continue
		}
		decision, reasons := noiseDecision(instance, all, silences, inhibitions, at)
		channelIDs := append([]uuid.UUID{}, p.ChannelIDs...)
		routeVersion := int64(1)
		routingSnapshot := map[string]int64{}
		labels := eventLabels(instance)
		for _, route := range routes {
			spec, e := decodeSpec[RouteSpec](route.Spec)
			if e != nil {
				return e
			}
			if !spec.Enabled || len(p.RouteIDs) > 0 && !slices.Contains(p.RouteIDs, route.ID) || !matches(labels, spec.Matchers) {
				continue
			}
			channelIDs = append(channelIDs, spec.ChannelIDs...)
			routingSnapshot[route.ID.String()] = route.Version
			routeVersion = max(routeVersion, route.Version)
			if !spec.Continue {
				break
			}
		}
		sort.Slice(channelIDs, func(i, j int) bool { return channelIDs[i].String() < channelIDs[j].String() })
		channelIDs = slices.Compact(channelIDs)
		for _, id := range channelIDs {
			channel, e := q.GetAlertConfiguration(ctx, db.GetAlertConfigurationParams{EnterpriseID: policy.EnterpriseID, ID: id})
			if e != nil || channel.Kind != "channel" {
				continue
			}
			spec, e := decodeSpec[ChannelSpec](channel.Spec)
			if e != nil || !spec.Enabled {
				continue
			}
			if spec.Template == "" {
				spec.Template = defaultTemplate
			}
			message, e := renderNotification(instance, event.EventType, spec, at)
			if e != nil {
				message = Message{Subject: instance.State.PolicyName, Text: "Notification template failed", EventType: event.EventType, InstanceIDs: []uuid.UUID{instance.ID}, PolicyIDs: []uuid.UUID{policy.ID}, Labels: labels}
			}
			templateFailed := e != nil
			message.RoutingSnapshot = routingSnapshot
			payload, _ := json.Marshal(message)
			parts := []string{event.EventType}
			for _, key := range grouping.GroupBy {
				parts = append(parts, key+"="+labels[key])
			}
			groupKey := fingerprint(id, routeVersion, strings.Join(parts, "\n"))
			status := decision
			if templateFailed {
				status = "failed"
				instance.State.NotificationState = "failed"
			}
			if status == "pending" {
				status = "pending"
			}
			available := at.Add(time.Duration(grouping.WaitSeconds) * time.Second)
			if event.EventType != "first" {
				available = at.Add(time.Duration(grouping.IntervalSeconds) * time.Second)
			}
			delivery, e := q.CreateAlertDelivery(ctx, db.CreateAlertDeliveryParams{ID: uuid.New(), EnterpriseID: policy.EnterpriseID, EventID: nullID(event.ID), InstanceID: nullID(instance.ID), ChannelID: id, ChannelVersion: channel.Version, RouteVersion: routeVersion, DedupKey: event.ID.String() + ":" + id.String() + ":" + fmt.Sprint(channel.Version), GroupKey: groupKey, ResourceIds: instance.ResourceIDs, Payload: payload, Status: status, AvailableAt: timestamp(available)})
			if e != nil {
				return e
			}
			if templateFailed {
				if _, e = q.UpdateAlertDelivery(ctx, db.UpdateAlertDeliveryParams{EnterpriseID: delivery.EnterpriseID, ID: delivery.ID, Status: "failed", Attempt: 0, LastError: "NOTIFICATION_TEMPLATE_FAILED", AvailableAt: delivery.AvailableAt, Version: delivery.Version}); e != nil {
					return e
				}
			}
			if status == "pending" {
				if e = enqueueDelivery(ctx, q, delivery, available); e != nil {
					return e
				}
			}
			if decision != "pending" {
				instance.State.NotificationState = decision
				instance.State.NoiseReasons = reasons
			}
		}
		if decision == "pending" && len(channelIDs) > 0 {
			instance.State.LastNotificationAt = &at
		}
		raw, _ := json.Marshal(instance.State)
		if _, e = q.UpdateAlertInstance(ctx, db.UpdateAlertInstanceParams{EnterpriseID: row.EnterpriseID, ID: row.ID, State: raw, Lifecycle: row.Lifecycle, Health: row.Health, Severity: row.Severity, StartedAt: row.StartedAt, ResolvedAt: row.ResolvedAt, LastEvaluatedAt: row.LastEvaluatedAt, Version: row.Version}); e != nil {
			return e
		}
	}
	return nil
}
func enqueueDelivery(ctx context.Context, q *db.Queries, d db.AlertDelivery, available time.Time) error {
	payload, _ := json.Marshal(map[string]any{"delivery_id": d.ID})
	id := uuid.NewSHA1(d.ID, []byte(fmt.Sprintf("attempt:%d:version:%d", d.Attempt, d.Version)))
	_, err := q.CreateRuntimeTask(ctx, db.CreateRuntimeTaskParams{ID: id, EnterpriseID: nullID(d.EnterpriseID), Queue: NotificationQueue, Payload: payload, MaxAttempts: 3, AvailableAt: timestamp(available)})
	return err
}
func renderNotification(i Instance, event string, channel ChannelSpec, at time.Time) (Message, error) {
	value := "unknown"
	if i.State.Value != nil {
		value = fmt.Sprintf("%g%s", *i.State.Value, i.State.Unit)
	}
	if gap := i.State.DeliveryGap; gap != nil {
		value = fmt.Sprintf("terminal failures: logs=%d spans=%d metric_points=%d; receiver refusal attempts: logs=%d spans=%d metric_points=%d; enqueue failure attempts: logs=%d spans=%d metric_points=%d; exact targets and sample timestamps unknown", gap.TerminalFailures.LogRecords, gap.TerminalFailures.Spans, gap.TerminalFailures.MetricPoints, gap.ReceiverRefusals.LogRecords, gap.ReceiverRefusals.Spans, gap.ReceiverRefusals.MetricPoints, gap.EnqueueFailures.LogRecords, gap.EnqueueFailures.Spans, gap.EnqueueFailures.MetricPoints)
	}
	fields := map[string]any{"severity": i.Severity, "policy": i.State.PolicyName, "resource": strings.Join(uuidStrings(i.ResourceIDs), ", "), "metric": i.State.Metric, "value": value, "threshold": i.State.Threshold, "duration": int64(0), "event": event, "detail_url": "/alerts?instance=" + i.ID.String()}
	if i.StartedAt != nil {
		fields["duration"] = int64(at.Sub(*i.StartedAt).Seconds())
	}
	pattern := channel.Template
	if pattern == "" {
		pattern = "[{{.severity}}] {{.policy}}\n{{.resource}}\n{{.metric}} = {{.value}}; threshold {{.threshold}}\n{{.event}} · {{.detail_url}}"
		if i.State.DeliveryGap != nil {
			pattern = "[{{.severity}}] {{.policy}}\n{{.resource}}\n{{.value}}\n{{.event}} · {{.detail_url}}"
		}
	}
	t, err := template.New("alert").Option("missingkey=error").Parse(normalizeTemplate(pattern))
	if err != nil {
		return Message{}, ErrInvalid
	}
	var out bytes.Buffer
	if err = t.Execute(&out, fields); err != nil {
		return Message{}, ErrInvalid
	}
	return Message{Subject: "[" + i.Severity + "] " + i.State.PolicyName, Text: out.String(), EventType: event, InstanceIDs: []uuid.UUID{i.ID}, PolicyIDs: []uuid.UUID{i.PolicyID}, Labels: eventLabels(i)}, nil
}
func uuidStrings(ids []uuid.UUID) []string {
	out := []string{}
	for _, id := range ids {
		out = append(out, id.String())
	}
	return out
}
