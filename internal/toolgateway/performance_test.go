package toolgateway

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

// Record local gateway latency separately from model, database and network
// time. These measurements do not represent end-to-end business task latency.
func TestNativeGatewayLatencyBaseline(t *testing.T) {
	for _, size := range []int{500, 1000} {
		_, set := testGateway(t, size)
		for _, operation := range []string{"search_cold", "search_warm", "describe", "invoke"} {
			t.Run(fmt.Sprintf("%d/%s", size, operation), func(t *testing.T) {
				principal := toolruntime.Principal{EnterpriseID: uuid.New(), UserID: uuid.New(), Permissions: []string{"host.read"}}
				tool := "tool.search"
				args := map[string]any{"category": "host", "query": "inspect"}
				if operation == "describe" || operation == "invoke" {
					tool = "tool." + operation
					args = map[string]any{"category": "host", "name": "inspect_0000"}
					if operation == "invoke" {
						args["arguments"] = map[string]any{"limit": float64(1)}
					}
				}
				samples := make([]time.Duration, 100)
				for index := range samples {
					if operation == "search_cold" {
						principal.AuthorizationVersion = int64(index + 1)
					}
					started := time.Now()
					if _, err := set.Invoke(context.Background(), tool, toolruntime.Invocation{Principal: principal, Arguments: args}); err != nil {
						t.Fatal(err)
					}
					samples[index] = time.Since(started)
				}
				slices.Sort(samples)
				t.Logf("catalog=%d operation=%s samples=%d p50_us=%d p95_us=%d", size, operation, len(samples), samples[49].Microseconds(), samples[94].Microseconds())
			})
		}
	}
}
