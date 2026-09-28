package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
)

func (runtime Runtime) verifyMetricMetadata(ctx context.Context, actor Actor, target Target, resources []uuid.UUID, keys []string, execution Execution, allocation *queryengine.Budget, ledger *executionBudget) string {
	runtime = runtime.metered(actor)
	builder := target.SourceDefinition.Builder
	if builder == nil || target.Language != queryengine.LanguagePromQL || builder.MetricType == "" {
		return ""
	}
	metric := builder.Metric
	if builder.MetricType == "histogram" {
		metric = strings.TrimSuffix(metric, "_bucket")
	}
	reservation := queryengine.Budget{MaxScanBytes: allocation.MaxScanBytes / 4, MaxRows: min(16, allocation.MaxRows/4), MaxResultBytes: allocation.MaxResultBytes / 4, Timeout: telemetry.DefaultTimeout}
	if reservation.MaxScanBytes <= 0 || reservation.MaxRows < 2 || reservation.MaxResultBytes <= 0 {
		return "QUERY_BUDGET_EXCEEDED"
	}
	result, err := runtime.Backend.DiscoverData(ctx, telemetry.DataCatalogRequest{SubjectID: actor.SubjectID, SubjectType: actor.SubjectType, EnterpriseID: actor.EnterpriseID, ResourceIDs: resources, SourceKeys: keys, AuthorizationVersion: actor.AuthorizationVersion, Signal: "metrics", Kind: "metrics", Metric: metric, From: execution.From, To: execution.To, Limit: 2, Budget: reservation})
	if err != nil {
		ledger.failed(reservation)
		if errors.Is(err, telemetry.ErrQueryBudget) || errors.Is(err, queryengine.ErrBudget) {
			return "QUERY_BUDGET_EXCEEDED"
		}
		if errors.Is(err, telemetry.ErrQueryInvalid) {
			return "QUERY_INVALID"
		}
		return "CATALOG_UNAVAILABLE"
	}
	ledger.record(result.Meta, result)
	encoded, _ := json.Marshal(result)
	allocation.MaxScanBytes -= result.Meta.ScannedBytes
	allocation.MaxRows -= int(result.Meta.ReturnedRows)
	allocation.MaxResultBytes -= int64(len(encoded))
	for _, descriptor := range result.Metrics {
		if descriptor.Type != builder.MetricType {
			return "QUERY_TYPE_ERROR"
		}
	}
	return ""
}
