package argusdev

import (
	"strings"
	"testing"
)

func TestRejectedUndescribedInvocationNeedsActualDescribedRecovery(t *testing.T) {
	recovered := `[{"tool":"tool.search","status":"succeeded","input":{"category":"metric"}},{"tool":"tool.invoke","status":"failed","error_code":"TOOL_DESCRIPTION_REQUIRED","input":{"category":"metric","name":"overview"}},{"tool":"tool.describe","status":"succeeded","input":{"category":"metric","name":"overview"}},{"tool":"tool.invoke","status":"succeeded","input":{"category":"metric","name":"overview"}}]`
	if err := verifyTypedToolDiscoveryOrder(recovered, "metric"); err != nil {
		t.Fatal(err)
	}
	if err := verifyTypedToolDiscoveryOrder(strings.Replace(recovered, "TOOL_DESCRIPTION_REQUIRED", "TOOL_INPUT_INVALID", 1), "metric"); err == nil {
		t.Fatal("an ambiguous failure bypassed discovery evidence")
	}
	noSuccess := `[{"tool":"tool.search","status":"succeeded","input":{"category":"metric"}},{"tool":"tool.invoke","status":"failed","error_code":"TOOL_DESCRIPTION_REQUIRED","input":{"category":"metric","name":"overview"}}]`
	if err := verifyTypedToolDiscoveryOrder(noSuccess, "metric"); err == nil {
		t.Fatal("a rejected attempt counted as successful business invocation")
	}
	unsafe := strings.Replace(recovered, `"status":"failed","error_code":"TOOL_DESCRIPTION_REQUIRED"`, `"status":"succeeded"`, 1)
	if err := verifyTypedToolDiscoveryOrder(unsafe, "metric"); err == nil {
		t.Fatal("successful undescribed dispatch was accepted")
	}
}
