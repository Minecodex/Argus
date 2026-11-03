package argusdev

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestHighResolutionAcceptanceKeepsDefaultCohortAndRejectsUnsupportedProfiles(t *testing.T) {
	window := plan11ComparisonWindow{From: time.Unix(100, 0).UTC(), To: time.Unix(400, 0).UTC()}
	host := &plan6HostLoop{HostName: "owned"}
	if !strings.Contains(plan11SummaryDeliveryPrompt(host, window, "catalog", "en"), "60-second") {
		t.Fatal("default historical request changed")
	}
	window.HighResolution = true
	prompt := plan11SummaryDeliveryPrompt(host, window, "catalog", "en")
	if !strings.Contains(prompt, "1-second") || !strings.Contains(prompt, "original system_cpu_utilization") || strings.Contains(prompt, "60-second") {
		t.Fatal("highresolution question lost actualCPUallstates", prompt)
	}
	for _, c := range []struct {
		summary      bool
		suite, phase string
		profiles     []string
		want         bool
	}{{true, "plan8", "plan11-compare", []string{"candidate"}, true}, {false, "plan8", "plan11-compare", []string{"candidate"}, false}, {true, "plan8", "plan11-review", []string{"candidate"}, false}, {true, "plan8", "plan11-compare", []string{"legacy"}, false}, {true, "plan7", "plan11-compare", []string{"candidate"}, false}} {
		if (plan11ValidateHighResolution(true, c.summary, c.suite, c.phase, c.profiles) == nil) != c.want {
			t.Fatal("highresolution escaped standalonecandidate admission", c)
		}
	}
}

func TestHighResolutionRequiresActualLargeSelectedOriginalAndRequestedStep(t *testing.T) {
	window := plan11ComparisonWindow{HighResolution: true}
	id := uuid.New()
	call := plan8ModelToolFact{ID: id.String(), Tool: "tool.invoke", Status: "succeeded"}
	call.Input.Category = "metric"
	call.Input.Name = "query"
	call.Input.Arguments = map[string]any{"step_seconds": float64(1)}
	evidence := map[string]any{"query_call_id": id}
	for _, c := range []struct {
		expression string
		bytes      int
		want       bool
	}{{"system_cpu_utilization", 32769, true}, {`({__name__="system_cpu_utilization"})`, 32769, true}, {`system_cpu_utilization{state=~".*"}`, 32769, true}, {`system_cpu_utilization{state="idle"}`, 32769, false}, {"system_cpu_utilization offset 1m", 32769, false}, {"system_cpu_utilization @ end()", 32769, false}, {"system_cpu_utilization*100", 32769, false}, {"system_cpu_utilization", 32768, false}} {
		files := []plan11QueryFile{{ID: id.String(), Query: c.expression, Body: make([]byte, c.bytes)}}
		if (plan11VerifyResolutionEvidence(window, "catalog", evidence, []plan8ModelToolFact{call}, files) == nil) != c.want {
			t.Fatal("large original substituted", c)
		}
	}
	evidence["query_call_id"] = id.String()
	if err := plan11VerifyResolutionEvidence(window, "metric", evidence, []plan8ModelToolFact{call}, nil); err != nil {
		t.Fatal("serialized UUID identity rejected", err)
	}
	for _, wrong := range []any{uuid.Nil, "bad-id", nil, 123} {
		evidence["query_call_id"] = wrong
		if plan11VerifyResolutionEvidence(window, "metric", evidence, []plan8ModelToolFact{call}, nil) == nil {
			t.Fatal("invalid query identity accepted", wrong)
		}
	}
	evidence["query_call_id"] = id
	call.Input.Arguments["step_seconds"] = float64(60)
	if plan11VerifyResolutionEvidence(window, "metric", evidence, []plan8ModelToolFact{call}, nil) == nil {
		t.Fatal("differentrequestedstepaccepted")
	}
}

func TestRecordedR65ResolutionUsesProductionUUIDAndActualSelectedOriginals(t *testing.T) {
	dir := os.Getenv("ARGUS_PLAN11_R65_REVIEW_DIR")
	if dir == "" {
		t.Skip("requires immutable R65 actual full records")
	}
	var profile struct {
		Cases []struct {
			Question string
			Evidence map[string]any `json:"query_evidence"`
			Calls    []plan8ModelToolFact
			Files    []plan11QueryFile `json:"original_query_files"`
		}
	}
	raw, err := os.ReadFile(filepath.Join(dir, "plan11-profile-comparison.json"))
	if err != nil || json.Unmarshal(raw, &profile) != nil || len(profile.Cases) != 6 {
		t.Fatal("incomplete original R65 cohort", err)
	}
	for _, c := range profile.Cases {
		for i := range c.Files {
			f := &c.Files[i]
			f.Body, err = os.ReadFile(filepath.Join(dir, "comparison-original-"+f.ID+".json"))
			if err != nil || len(f.Body) != f.Bytes || fmt.Sprintf("%x", sha256.Sum256(f.Body)) != f.Hash {
				t.Fatal("immutable actual original mismatch", f.ID, err)
			}
		}
		id, ok := c.Evidence["query_call_id"].(string)
		if !ok {
			t.Fatal("recorded selected identity missing")
		}
		for _, value := range []any{id, uuid.MustParse(id)} {
			c.Evidence["query_call_id"] = value
			if err := plan11VerifyResolutionEvidence(plan11ComparisonWindow{HighResolution: true}, c.Question, c.Evidence, c.Calls, c.Files); err != nil {
				t.Fatal("actual selected original rejected", c.Question, err)
			}
		}
	}
	t.Log("All six actual selected R65 originals pass both serialized/production UUID shapes; no provider calls or original machine/prose rescore")
}
