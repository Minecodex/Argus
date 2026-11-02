package argusdev

import (
	"context"
	"errors"
	"math"
	"reflect"
	"time"

	"github.com/google/uuid"
)

// Check the immutable delivered bytes too: a correct chat conclusion must not
// mask a report containing guessed point counts or requested-window timestamps.
func (a *App) plan11VerifyDeliveredReports(ctx context.Context, env *E2EEnvironment, conversation, run string, expected plan11Conclusion) ([]map[string]any, error) {
	if uuid.Validate(conversation) != nil || uuid.Validate(run) != nil {
		return nil, errors.New("invalid report oracle identity")
	}
	rows, err := a.plan11LoadReportFiles(ctx, env, conversation, run)
	if err != nil {
		return nil, err
	}
	proof := []map[string]any{}
	for _, row := range rows {
		name := "plan11-report-" + row.ID + ".md"
		if err = plan11MatchReportConclusion(row.Body, expected); err != nil {
			return nil, err
		}
		proof = append(proof, map[string]any{"delivery_id": row.ID, "content_hash": row.Hash, "bytes": row.Bytes, "report": name, "conclusion_matches_actual_query": true})
	}
	if len(proof) == 0 {
		return nil, errors.New("no delivered Markdown report to verify")
	}
	return proof, nil
}

func plan11MatchReportConclusion(body []byte, expected plan11Conclusion) error {
	actual, decodeErr := plan11ParseConclusion(string(body))
	if decodeErr != nil || actual.ResourceID != expected.ResourceID || actual.Metric != expected.Metric || len(actual.Limitations) == 0 || expected.PointCount == nil {
		return errors.New("report evidence differs from the observed conclusion")
	}
	if !reflect.DeepEqual(actual.Dashboards, expected.Dashboards) {
		return errors.New("report dashboard panel evidence differs from the independently verified conclusion")
	}
	// The chat conclusion has already passed the independent raw-query oracle.
	first, firstErr := time.Parse(time.RFC3339Nano, expected.FirstPointAt)
	last, lastErr := time.Parse(time.RFC3339Nano, expected.LastPointAt)
	if firstErr != nil || lastErr != nil {
		return errors.New("invalid expected report point window")
	}
	if err := plan11MatchPointConclusion(actual, plan11PointEvidence{Count: *expected.PointCount, First: first, Last: last}); err != nil {
		return err
	}
	if expected.ValueMin == nil || expected.ValueMax == nil {
		return errors.New("expected report numeric evidence is missing")
	}
	if plan11DisplayTransform(actual.Transform) != plan11DisplayTransform(expected.Transform) || plan11DisplayUnit(actual.DisplayUnit) != plan11DisplayUnit(expected.DisplayUnit) {
		return errors.New("report display convention differs from the observed conclusion")
	}
	// Arithmetic was independently checked against raw bytes before this call.
	// Match every checked numeric field; do not impose the Host display policy
	// on a target task that explicitly requested original bytes/status codes.
	close := func(a, b *float64) bool {
		return a != nil && b != nil && !math.IsNaN(*a) && !math.IsNaN(*b) && !math.IsInf(*a, 0) && !math.IsInf(*b, 0) && math.Abs(*a-*b) <= math.Max(1e-12, math.Abs(*b)*1e-9)
	}
	if plan11DisplayUnit(actual.Unit) != plan11DisplayUnit(expected.Unit) || !close(actual.ValueMin, expected.ValueMin) || !close(actual.ValueMax, expected.ValueMax) || !close(actual.DisplayMin, expected.DisplayMin) || !close(actual.DisplayMax, expected.DisplayMax) {
		return errors.New("report units or numeric values differ from independently checked query facts")
	}
	return nil
}
