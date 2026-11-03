package argusdev

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"
)

func (a *App) plan11WaitNativeSDKHistogram(ctx context.Context, env *E2EEnvironment, application string) error {
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	bounded, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	for {
		result, e := client.JSON(bounded, "plan11-native-histogram-admission", "enterprise", http.MethodPost, "/enterprise/metrics/query", 200, map[string]any{"query": "histogram_count(argus_plan11_sdk_native_latency)", "resource_ids": []string{application}, "time_range": telemetryTimeRange(15 * time.Minute), "budget": telemetryBudget(100)}, enterpriseHeaders(env, ""))
		if e != nil {
			return e
		}
		if result["status"] == "success" && validQueryMeta(result["argus_meta"]) {
			data, _ := result["data"].(map[string]any)
			rows, _ := data["result"].([]any)
			for _, value := range rows {
				row, _ := value.(map[string]any)
				point, _ := row["value"].([]any)
				if len(point) == 2 {
					v, _ := point[1].(string)
					count, e := strconv.ParseFloat(v, 64)
					if e == nil && count > 0 {
						return nil
					}
				}
			}
		}
		if e = waitContext(bounded, time.Second); e != nil {
			return errors.Join(e, errors.New("actual native SDK Histogram count not observed"))
		}
	}
}
