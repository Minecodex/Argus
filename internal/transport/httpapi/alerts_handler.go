package httpapi

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/alerting"
	api "github.com/kakj-go/Argus/internal/gen/openapi/alertsapi"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"strings"
)

type AlertHandler struct {
	Identity EnterpriseIdentityHandler
	Service  alerting.Service
}

func (h AlertHandler) auth(ctx context.Context, mutation bool, csrf string, permissions ...string) (alerting.Actor, *api.ApiError) {
	p, f := h.Identity.enterprisePrincipalAny(ctx, mutation, csrf, permissions...)
	if f != nil {
		v := dashboardConvert[api.ApiError](f)
		return alerting.Actor{}, &v
	}
	id, e := uuid.Parse(p.ActorID())
	if e != nil {
		v := alertError(ctx, alerting.ErrDenied)
		return alerting.Actor{}, &v
	}
	kind := "user"
	if p.ServiceAccount != nil {
		kind = "service_account"
	}
	return alerting.Actor{EnterpriseID: p.EnterpriseIDValue(), SubjectID: id, SubjectType: kind, AuthorizationVersion: p.AuthorizationVersion()}, nil
}
func alertStatus(e error) int {
	switch {
	case errors.Is(e, alerting.ErrInvalid):
		return 400
	case errors.Is(e, alerting.ErrDenied):
		return 403
	case errors.Is(e, alerting.ErrNotFound):
		return 404
	case errors.Is(e, alerting.ErrConflict), errors.Is(e, postgres.ErrIdempotencyConflict):
		return 409
	default:
		return 503
	}
}
func alertError(ctx context.Context, e error) api.ApiError {
	v := setupErrorBase(ctx, e)
	v.Code = "ALERT_UNAVAILABLE"
	v.MessageKey = "errors.alerts.unavailable"
	v.Retryable = pointer(true)
	for _, known := range []error{alerting.ErrInvalid, alerting.ErrDenied, alerting.ErrNotFound, alerting.ErrConflict, alerting.ErrEvaluator} {
		if errors.Is(e, known) {
			v.Code = known.Error()
			v.MessageKey = "errors.alerts." + strings.ToLower(strings.SplitN(v.Code, "_", 2)[1])
			v.Retryable = pointer(false)
			break
		}
	}
	logMappedError(ctx, v.Code, e)
	return dashboardConvert[api.ApiError](v)
}

func (h AlertHandler) GetAlertSummary(ctx context.Context, r api.GetAlertSummaryRequestObject) (api.GetAlertSummaryResponseObject, error) {
	a, f := h.auth(ctx, false, "", "alert.read")
	if f != nil {
		return api.GetAlertSummarydefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}

	value, e := h.Service.Summary(ctx, a)
	if e != nil {
		return api.GetAlertSummarydefaultJSONResponse{Body: alertError(ctx, e), StatusCode: alertStatus(e)}, nil
	}
	return api.GetAlertSummary200JSONResponse(dashboardConvert[api.AlertSummary](value)), nil
}

func (h AlertHandler) ListAlertConfigurations(ctx context.Context, r api.ListAlertConfigurationsRequestObject) (api.ListAlertConfigurationsResponseObject, error) {
	a, f := h.auth(ctx, false, "", alerting.ConfigurationReadPermission(string(r.Params.Kind)))
	if f != nil {
		return api.ListAlertConfigurationsdefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}

	value, e := h.Service.Configurations(ctx, a, string(r.Params.Kind))
	if e != nil {
		return api.ListAlertConfigurationsdefaultJSONResponse{Body: alertError(ctx, e), StatusCode: alertStatus(e)}, nil
	}
	return api.ListAlertConfigurations200JSONResponse(dashboardConvert[[]api.AlertConfiguration](value)), nil
}

func (h AlertHandler) GetAlertConfiguration(ctx context.Context, r api.GetAlertConfigurationRequestObject) (api.GetAlertConfigurationResponseObject, error) {
	a, f := h.auth(ctx, false, "", "alert.read", "service_account.read")
	if f != nil {
		return api.GetAlertConfigurationdefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}

	value, e := h.Service.Configuration(ctx, a, r.Id)
	if e != nil {
		return api.GetAlertConfigurationdefaultJSONResponse{Body: alertError(ctx, e), StatusCode: alertStatus(e)}, nil
	}
	return api.GetAlertConfiguration200JSONResponse(dashboardConvert[api.AlertConfiguration](value)), nil
}

func (h AlertHandler) ListAlertInstances(ctx context.Context, r api.ListAlertInstancesRequestObject) (api.ListAlertInstancesResponseObject, error) {
	a, f := h.auth(ctx, false, "", "alert.read")
	if f != nil {
		return api.ListAlertInstancesdefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}

	value, e := h.Service.Instances(ctx, a, dashboardConvert[alerting.ListFilter](r.Params), false)
	if e != nil {
		return api.ListAlertInstancesdefaultJSONResponse{Body: alertError(ctx, e), StatusCode: alertStatus(e)}, nil
	}
	return api.ListAlertInstances200JSONResponse(dashboardConvert[api.AlertInstancePage](value)), nil
}

func (h AlertHandler) ListAlertHistory(ctx context.Context, r api.ListAlertHistoryRequestObject) (api.ListAlertHistoryResponseObject, error) {
	a, f := h.auth(ctx, false, "", "alert.read")
	if f != nil {
		return api.ListAlertHistorydefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}

	value, e := h.Service.Instances(ctx, a, dashboardConvert[alerting.ListFilter](r.Params), true)
	if e != nil {
		return api.ListAlertHistorydefaultJSONResponse{Body: alertError(ctx, e), StatusCode: alertStatus(e)}, nil
	}
	return api.ListAlertHistory200JSONResponse(dashboardConvert[api.AlertInstancePage](value)), nil
}

func (h AlertHandler) ListAlertDeliveries(ctx context.Context, r api.ListAlertDeliveriesRequestObject) (api.ListAlertDeliveriesResponseObject, error) {
	a, f := h.auth(ctx, false, "", "alert.read")
	if f != nil {
		return api.ListAlertDeliveriesdefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}

	value, e := h.Service.Deliveries(ctx, a, dashboardConvert[alerting.ListFilter](r.Params))
	if e != nil {
		return api.ListAlertDeliveriesdefaultJSONResponse{Body: alertError(ctx, e), StatusCode: alertStatus(e)}, nil
	}
	return api.ListAlertDeliveries200JSONResponse(dashboardConvert[api.AlertDeliveryPage](value)), nil
}

func (h AlertHandler) TrialAlertPolicy(ctx context.Context, r api.TrialAlertPolicyRequestObject) (api.TrialAlertPolicyResponseObject, error) {
	a, f := h.auth(ctx, true, r.Params.XCSRFToken, "alert.policy.manage")
	if f != nil {
		return api.TrialAlertPolicydefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	if r.Body == nil {
		return api.TrialAlertPolicydefaultJSONResponse{Body: alertError(ctx, alerting.ErrInvalid), StatusCode: 400}, nil
	}
	value, e := h.Service.Trial(ctx, a, dashboardConvert[alerting.PolicySpec](r.Body))
	if e != nil {
		return api.TrialAlertPolicydefaultJSONResponse{Body: alertError(ctx, e), StatusCode: alertStatus(e)}, nil
	}
	return api.TrialAlertPolicy200JSONResponse(dashboardConvert[api.AlertEvaluation](value)), nil
}

func (h AlertHandler) HandleAlertInstance(ctx context.Context, r api.HandleAlertInstanceRequestObject) (api.HandleAlertInstanceResponseObject, error) {
	a, f := h.auth(ctx, true, r.Params.XCSRFToken, "alert.handle")
	if f != nil {
		return api.HandleAlertInstancedefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	if r.Body == nil {
		return api.HandleAlertInstancedefaultJSONResponse{Body: alertError(ctx, alerting.ErrInvalid), StatusCode: 400}, nil
	}
	value, e := h.Service.Handle(ctx, a, r.Id, dashboardConvert[alerting.HandleInput](r.Body), alertRequestKey(ctx), "admin_ui")
	if e != nil {
		return api.HandleAlertInstancedefaultJSONResponse{Body: alertError(ctx, e), StatusCode: alertStatus(e)}, nil
	}
	return api.HandleAlertInstance200JSONResponse(dashboardConvert[api.AlertInstance](value)), nil
}

func (h AlertHandler) PreviewAlertConfiguration(ctx context.Context, r api.PreviewAlertConfigurationRequestObject) (api.PreviewAlertConfigurationResponseObject, error) {
	a, f := h.auth(ctx, true, r.Params.XCSRFToken, "alert.policy.manage", "alert.silence.manage", "alert.notification.manage", "service_account.manage")
	if f != nil {
		return api.PreviewAlertConfigurationdefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	if r.Body == nil {
		return api.PreviewAlertConfigurationdefaultJSONResponse{Body: alertError(ctx, alerting.ErrInvalid), StatusCode: 400}, nil
	}
	value, e := h.Service.Preview(ctx, a, dashboardConvert[alerting.Mutation](r.Body), r.Params.IdempotencyKey)
	if e != nil {
		return api.PreviewAlertConfigurationdefaultJSONResponse{Body: alertError(ctx, e), StatusCode: alertStatus(e)}, nil
	}
	return api.PreviewAlertConfiguration202JSONResponse(convertPending[api.PendingActionPublicSchema](value)), nil
}

func (h AlertHandler) PreviewAlertCommand(ctx context.Context, r api.PreviewAlertCommandRequestObject) (api.PreviewAlertCommandResponseObject, error) {
	a, f := h.auth(ctx, true, r.Params.XCSRFToken, "alert.handle", "alert.notification.manage")
	if f != nil {
		return api.PreviewAlertCommanddefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	if r.Body == nil {
		return api.PreviewAlertCommanddefaultJSONResponse{Body: alertError(ctx, alerting.ErrInvalid), StatusCode: 400}, nil
	}
	value, e := h.Service.PreviewCommand(ctx, a, dashboardConvert[alerting.Command](r.Body), r.Params.IdempotencyKey)
	if e != nil {
		return api.PreviewAlertCommanddefaultJSONResponse{Body: alertError(ctx, e), StatusCode: alertStatus(e)}, nil
	}
	return api.PreviewAlertCommand202JSONResponse(convertPending[api.PendingActionPublicSchema](value)), nil
}

func (h AlertHandler) GetAlertInstance(ctx context.Context, r api.GetAlertInstanceRequestObject) (api.GetAlertInstanceResponseObject, error) {
	a, f := h.auth(ctx, false, "", "alert.read")
	if f != nil {
		return api.GetAlertInstancedefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	v, events, e := h.Service.Instance(ctx, a, r.Id)
	if e != nil {
		return api.GetAlertInstancedefaultJSONResponse{Body: alertError(ctx, e), StatusCode: alertStatus(e)}, nil
	}
	return api.GetAlertInstance200JSONResponse{Instance: dashboardConvert[api.AlertInstance](v), Events: dashboardConvert[[]api.AlertEvent](events)}, nil
}
func (h AlertHandler) GetAlertDelivery(ctx context.Context, r api.GetAlertDeliveryRequestObject) (api.GetAlertDeliveryResponseObject, error) {
	a, f := h.auth(ctx, false, "", "alert.read")
	if f != nil {
		return api.GetAlertDeliverydefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	v, rows, e := h.Service.Delivery(ctx, a, r.Id)
	if e != nil {
		return api.GetAlertDeliverydefaultJSONResponse{Body: alertError(ctx, e), StatusCode: alertStatus(e)}, nil
	}
	attempts := []api.AlertDeliveryAttempt{}
	for _, row := range rows {
		attempts = append(attempts, api.AlertDeliveryAttempt{Id: row.ID, Attempt: int64(row.Attempt), Status: row.Status, ResultCode: row.ResultCode, ErrorCode: row.ErrorCode, StartedAt: row.StartedAt.Time, FinishedAt: row.FinishedAt.Time})
	}
	return api.GetAlertDelivery200JSONResponse{Delivery: dashboardConvert[api.AlertDelivery](v), Attempts: attempts}, nil
}
func (h AlertHandler) ExportAlertHistory(ctx context.Context, r api.ExportAlertHistoryRequestObject) (api.ExportAlertHistoryResponseObject, error) {
	a, f := h.auth(ctx, false, "", "alert.read")
	if f != nil {
		return api.ExportAlertHistorydefaultJSONResponse{Body: *f, StatusCode: 403}, nil
	}
	filter := dashboardConvert[alerting.ListFilter](r.Params)
	filter.Limit = 500
	filter.Cursor = ""
	count := int64(0)
	var content strings.Builder
	w := csv.NewWriter(&content)
	_ = w.Write([]string{"id", "policy", "metric", "severity", "lifecycle", "health", "started_at", "resolved_at", "resource_ids"})
	for {
		page, e := h.Service.Instances(ctx, a, filter, true)
		if e != nil {
			return api.ExportAlertHistorydefaultJSONResponse{Body: alertError(ctx, e), StatusCode: alertStatus(e)}, nil
		}
		for _, i := range page.Items {
			start, _ := json.Marshal(i.StartedAt)
			end, _ := json.Marshal(i.ResolvedAt)
			ids, _ := json.Marshal(i.ResourceIDs)
			record := []string{i.ID.String(), i.State.PolicyName, i.State.Metric, i.Severity, i.Lifecycle, i.Health, string(start), string(end), string(ids)}
			for n, c := range record {
				if strings.HasPrefix(c, "=") || strings.HasPrefix(c, "+") || strings.HasPrefix(c, "-") || strings.HasPrefix(c, "@") || strings.HasPrefix(c, "\t") {
					record[n] = "'" + c
				}
			}
			if e = w.Write(record); e != nil {
				return nil, e
			}
			count++
		}
		if page.NextCursor == nil {
			break
		}
		filter.Cursor = *page.NextCursor
	}
	w.Flush()
	if e := w.Error(); e != nil {
		return nil, e
	}
	return api.ExportAlertHistory200JSONResponse{Filename: "alert-history.csv", ContentType: "text/csv;charset=utf-8", Content: content.String(), Count: count}, nil
}

func alertRequestKey(ctx context.Context) string {
	r, ok := RequestFromContext(ctx)
	if !ok || r.Request == nil {
		return ""
	}
	return r.Request.Header.Get("Idempotency-Key")
}
