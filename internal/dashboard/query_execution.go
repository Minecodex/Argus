package dashboard

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
)

type queryScope struct {
	From, To  time.Time
	Resources []ResourceScope
	Sources   []ResolvedSource
}

func (runtime Runtime) checkQueryScope(ctx context.Context, actor Actor, scope queryScope) error {
	if runtime.Store == nil {
		return ErrUnavailable
	}
	if err := authorize(ctx, runtime.Store.Queries, actor, "telemetry.dashboard.read"); err != nil {
		return err
	}
	ids := []uuid.UUID{}
	for _, r := range scope.Resources {
		ids = append(ids, r.ID)
	}
	if len(ids) == 0 {
		return ErrDenied
	}
	_, err := resolveResources(ctx, runtime.Store.Queries, actor, ids)
	return err
}

func (runtime Runtime) executeTarget(ctx context.Context, actor Actor, spec Spec, panel Panel, target Target, variables, locals, inputs map[string]Selection, scope queryScope, ledger *executionBudget) (TargetExecution, error) {
	runtime = runtime.metered(actor)
	item := TargetExecution{ID: target.ID, Status: "no_data"}
	if err := runtime.checkQueryScope(ctx, actor, scope); err != nil {
		return item, err
	}
	bound, err := bindTargetInputs(spec, panel, target, variables, locals, inputs)
	if err != nil {
		return item, err
	}
	compiled, err := compileConcreteTarget(bound, false)
	if err != nil {
		return item, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	item.QueryHash, item.ResultType = compiled.Hash, compiled.ResultType
	allocation, err := ledger.next(ctx)
	if err != nil {
		item.Status, item.Code = "skipped_budget", "QUERY_BUDGET_EXCEEDED"
		return item, nil
	}
	if len(scope.Sources) == 0 {
		return item, nil
	}
	ids, keys := []uuid.UUID{}, []string{}
	for _, r := range scope.Resources {
		ids = append(ids, r.ID)
	}
	for _, s := range scope.Sources {
		keys = append(keys, s.Key())
	}
	if code := runtime.verifyMetricMetadata(ctx, actor, bound, ids, keys, Execution{From: scope.From, To: scope.To}, &allocation, ledger); code != "" {
		item.Status, item.Code = "error", code
		return item, nil
	}
	if allocation.MaxScanBytes <= 0 || allocation.MaxRows <= 0 || allocation.MaxResultBytes <= 0 {
		item.Status, item.Code = "skipped_budget", "QUERY_BUDGET_EXCEEDED"
		return item, nil
	}
	step := time.Duration(0)
	if target.Language == queryengine.LanguagePromQL && target.QueryMode == "range" {
		step, err = target.RangeStepPolicy.Resolve(scope.From, scope.To)
		if err != nil {
			return item, err
		}
	}
	data, err := runtime.Backend.ExecuteEngineQuery(ctx, queryengine.Request{CacheNamespace: runtime.targetCacheNamespace(panel, target, variables, locals, inputs, scope), Language: target.Language, Expression: compiled.Query.Expression, Pipeline: compiled.Query.Pipeline, Operation: compiled.Query.Operation, Variables: compiled.Query.Variables, Instant: target.QueryMode == "instant", Start: scope.From, End: scope.To, Step: step, Scope: queryengine.Scope{SubjectID: actor.SubjectID, SubjectType: actor.SubjectType, EnterpriseID: actor.EnterpriseID, ResourceIDs: ids, SourceKeys: keys, AuthorizationVersion: actor.AuthorizationVersion}, Budget: allocation})
	item.StepSeconds = int(step / time.Second)
	if err != nil {
		ledger.failed(allocation)
		item.Status, item.Code = "error", "QUERY_UNAVAILABLE"
		if errors.Is(err, telemetry.ErrQueryInvalid) || errors.Is(err, queryengine.ErrUnsupportedLanguage) {
			item.Code = "QUERY_INVALID"
		}
		if errors.Is(err, telemetry.ErrQueryBudget) || errors.Is(err, queryengine.ErrBudget) {
			item.Code = "QUERY_BUDGET_EXCEEDED"
		}
		return item, nil
	}
	ledger.record(data.Meta, data.Data)
	if data.ResultType != compiled.ResultType {
		item.Status, item.Code = "error", "QUERY_TYPE_ERROR"
		return item, nil
	}
	item.Data, item.Meta, item.ResultType = data.Data, data.Meta, data.ResultType
	item.Status = "success"
	if emptyTypedResult(data.ResultType, data.Data) {
		item.Status = "no_data"
	}
	if data.Meta.Partial {
		item.Status = "partial"
	}
	return item, nil
}

func targetFailure(item TargetExecution) error {
	switch item.Code {
	case "QUERY_BUDGET_EXCEEDED":
		return telemetry.ErrQueryBudget
	case "QUERY_INVALID", "QUERY_TYPE_ERROR":
		return ErrInvalid
	case "QUERY_UNAVAILABLE", "CATALOG_UNAVAILABLE":
		return ErrUnavailable
	}
	if item.Status == "error" || item.Status == "skipped_budget" {
		return ErrUnavailable
	}
	return nil
}
