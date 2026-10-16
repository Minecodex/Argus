package argusdev

import "testing"

func TestPlan6RejectedDescriptionAttemptCannotCountAsBusinessExecution(t *testing.T) {
	blocked := `[{"tool":"tool.search","status":"succeeded","input":{"category":"alert"}},
{"tool":"tool.invoke","status":"failed","error_code":"TOOL_DESCRIPTION_REQUIRED","input":{"category":"alert","name":"channel.list"}}]`
	if err := verifyPlan6DiscoveryOrder(blocked); err == nil {
		t.Fatal("only a runtime-blocked attempt satisfied business invocation")
	}
	repaired := `[{"tool":"tool.search","status":"succeeded","input":{"category":"alert"}},
{"tool":"tool.invoke","status":"failed","error_code":"TOOL_DESCRIPTION_REQUIRED","input":{"category":"alert","name":"channel.list"}},
{"tool":"tool.describe","status":"succeeded","input":{"category":"alert","name":"channel.list"}},
{"tool":"tool.invoke","status":"succeeded","input":{"category":"alert","name":"channel.list"}}]`
	if err := verifyPlan6DiscoveryOrder(repaired); err != nil {
		t.Fatal(err)
	}
	unguarded := `[{"tool":"tool.search","status":"succeeded","input":{"category":"alert"}},
{"tool":"tool.invoke","status":"succeeded","input":{"category":"alert","name":"channel.list"}}]`
	if err := verifyPlan6DiscoveryOrder(unguarded); err == nil {
		t.Fatal("undescribed successful business invocation passed")
	}
}
