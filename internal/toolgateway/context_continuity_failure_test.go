package toolgateway

import (
	"errors"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"testing"
)

func TestValidatedBusinessIdentitySurvivesFailureWithoutDisclosingHiddenTools(t *testing.T) {
	_, set := testGateway(t, 1)
	principal := toolruntime.Principal{EnterpriseID: uuid.New(), UserID: uuid.New(), Permissions: []string{"host.read"}}
	input := map[string]any{"category": "host", "name": "inspect_0000", "arguments": map[string]any{}}
	result, err := set.Invoke(t.Context(), "tool.invoke", toolruntime.Invocation{Principal: principal, Arguments: input})
	var invalid toolruntime.Error
	if !errors.As(err, &invalid) || invalid.Kind != "TOOL_INPUT_INVALID" || result.ToolID != "host.inspect_0000" {
		t.Fatal("validated failure lost its identity", result, err)
	}
	principal.Permissions = nil
	result, err = set.Invoke(t.Context(), "tool.invoke", toolruntime.Invocation{Principal: principal, Arguments: input})
	if err == nil || result.ToolID != "" {
		t.Fatal("unauthorized requested identity was promoted", result, err)
	}
}
