package kql

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPipelineDoesNotReorderOperations(t *testing.T) {
	for _, input := range []string{"limit 2 | stats count()", "stats count() | where service_name = api", "sort timestamp asc | stats count()", "parse json | parse logfmt", "where body : x | parse json", "limit 2 | limit 1", "stats count() | stats count()", "stats count() by service_name | sort timestamp asc"} {
		if _, err := ParsePipeline(input, 100); err == nil {
			t.Fatalf("silently reordered pipeline %q", input)
		}
	}
}

func TestQuotedPipeAndAggregateResultShape(t *testing.T) {
	request := Request{Expression: `body : "a|b" | stats count() by bin(timestamp, 5m), service_name | limit 10`, Start: time.Unix(0, 0), End: time.Unix(3600, 0), Scope: Scope{ResourceIDs: []uuid.UUID{uuid.New()}}, Budget: Budget{MaxRows: 100}}
	plan, err := CompileQuery("tenant_logs", request)
	if err != nil {
		t.Fatal(err)
	}
	if plan.ResultType != "timeseries" || plan.Pipeline.Aggregate.Bucket != 5*time.Minute || len(plan.Columns) != 3 {
		t.Fatalf("wrong aggregation shape: %+v", plan)
	}
	if strings.Contains(plan.SQL, "a|b") || !strings.Contains(plan.SQL, "resource_id IN (?)") || plan.Args[len(plan.Args)-1] != 10 {
		t.Fatalf("unsafe or incorrect plan: %+v", plan)
	}
	request.Expression = `* | stats avg(json.duration) by service_name`
	if _, err := CompileQuery("tenant_logs", request); err == nil {
		t.Fatal("JSON fields require an explicit parser")
	}
	request.Expression = `* | parse json | stats avg(json.duration) by service_name`
	if plan, err = CompileQuery("tenant_logs", request); err != nil || plan.ResultType != "table" {
		t.Fatalf("numeric aggregation: %+v %v", plan, err)
	}
}

func TestSystemLimitDiffersFromExplicitLimit(t *testing.T) {
	request := Request{Expression: "*", Start: time.Unix(0, 0), End: time.Unix(1, 0), Scope: Scope{ResourceIDs: []uuid.UUID{uuid.New()}}, Budget: Budget{MaxRows: 2}}
	plan, err := CompileQuery("logs", request)
	if err != nil || plan.Args[len(plan.Args)-1] != 3 {
		t.Fatalf("system limit needs a truncation sentinel: %+v %v", plan, err)
	}
	request.Pipeline = "limit 2"
	plan, err = CompileQuery("logs", request)
	if err != nil || plan.Args[len(plan.Args)-1] != 2 {
		t.Fatalf("explicit semantic limit changed: %+v %v", plan, err)
	}
	request.Scope.ResourceIDs = nil
	if _, err := CompileQuery("logs", request); err == nil {
		t.Fatal("empty scope became unrestricted")
	}
}

func TestLiteralsPreserveEscapesEmptyAndNumericLookingStrings(t *testing.T) {
	for _, test := range []struct {
		input string
		value any
	}{
		{`body=""`, ""}, {`service_name="001"`, "001"}, {`body : "a\"b|c"`, "a\"b|c"}, {`severity_number>=3`, int64(3)},
	} {
		expr, err := Parse(test.input)
		if err != nil {
			t.Fatal(err)
		}
		compiler := Compiler{}
		if _, err = compiler.Compile(expr); err != nil {
			t.Fatal(err)
		}
		if len(compiler.Args) != 1 || compiler.Args[0] != test.value {
			t.Fatalf("%s: %#v", test.input, compiler.Args)
		}
	}
}
