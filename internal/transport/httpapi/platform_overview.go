package httpapi

import (
	"context"
	"net/http"

	platformapi "github.com/kakj-go/Argus/internal/gen/openapi/platform"
)

func (handler PlatformHandler) GetPlatformOverview(ctx context.Context, _ platformapi.GetPlatformOverviewRequestObject) (platformapi.GetPlatformOverviewResponseObject, error) {
	if _, response := handler.platformPrincipal(ctx, false, ""); response != nil {
		return platformapi.GetPlatformOverviewdefaultJSONResponse{Body: *response, StatusCode: http.StatusUnauthorized}, nil
	}
	value, err := handler.Enterprise.Overview(ctx)
	if err != nil {
		return platformapi.GetPlatformOverviewdefaultJSONResponse{Body: platformError(ctx, err), StatusCode: http.StatusInternalServerError}, nil
	}
	usage := make([]platformapi.PlatformMonthlySandboxUsage, 0, len(value.MonthlyUsage))
	for _, item := range value.MonthlyUsage {
		usage = append(usage, platformapi.PlatformMonthlySandboxUsage{Month: item.Month, SessionCount: item.SessionCount, SessionSeconds: item.SessionSeconds})
	}
	return platformapi.GetPlatformOverview200JSONResponse{
		SampledAt: value.SampledAt, EnterpriseCount: value.Counts.EnterpriseCount, ActiveEnterpriseCount: value.Counts.ActiveEnterpriseCount,
		ActiveSandboxSessionCount: value.Counts.ActiveSandboxSessionCount, PendingAdminCount: value.Counts.PendingAdminCount,
		UsageFromMonth: value.FromMonth, UsageToMonth: value.ToMonth, MonthlyUsage: usage,
	}, nil
}
