package argusdev

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestExplicitPointClaimsPreserveTimeUnitAndRoundedValueAssociation(t *testing.T) {
	body := []byte(`{"data":{"result":[{"metric":{"__name__":"memory"},"values":[[1793465311,"0.78049236068"],[1793465371,"0.76853265959"]]}]}}`)
	c := plan11Conclusion{Metric: "memory", Unit: "1", Transform: "percent"}
	for _, text := range []string{
		"78.05% at 16:48:31Z and 76.85% at 16:49:31Z.",
		"16:48:31Z = 0.78049236068; 16:49:31Z: 0.76853265959",
		"16:48:31 为 0.78049236068（78.049%）。",
		"16:49:31Z（0.76853265959）",
		"固定 UTC 窗口：2026-10-31T16:44:31Z 至 2026-10-31T16:49:31Z（60 秒评估步长）",
		"Window ends at 16:49:31Z (60-second evaluation step).",
		"窗口末点 16:49:31Z（评测步长 60 秒）",
	} {
		if err := plan11VerifyExplicitPointClaims(body, c, text); err != nil {
			t.Fatal("correct original/rounded point rejected", text, err)
		}
	}
	for _, text := range []string{
		"76.85% at 16:48:31Z and 78.05% at 16:49:31Z.",
		"Values: 0.76853265959 (≈76.85%) at 16:48:31Z.",
		"16:48:31Z = 0.76853265959",
		"16:48:31Z（0.76853265959）",
		"78.05% at 16:47:31Z",
		"0.78049 GiB at 16:48:31Z",
	} {
		if err := plan11VerifyExplicitPointClaims(body, c, text); err == nil {
			t.Fatal("time/value/unit error accepted", text)
		}
	}
	bytes := []byte(`{"data":{"result":[{"metric":{"__name__":"bytes"},"values":[[1793465311,"12716535808"]]}]}}`)
	if err := plan11VerifyExplicitPointClaims(bytes, plan11Conclusion{Metric: "bytes", Unit: "By", Transform: "bytes_to_gib"}, "11.843 GiB at 16:48:31Z; 12,716,535,808 By at 16:48:31Z"); err != nil {
		t.Fatal("binary conversion or grouped source integer rejected", err)
	}
}

func TestRecordedR35PointClaimReview(t *testing.T) {
	directory := os.Getenv("ARGUS_PLAN11_R35_POINT_REVIEW_DIR")
	if directory == "" {
		t.Skip("requires explicit owned immutable R35 evidence")
	}
	var calls []struct {
		RunID      string                          `json:"run_id"`
		Tool       string                          `json:"tool"`
		Status     string                          `json:"status"`
		Input      struct{ Category, Name string } `json:"input"`
		Projection struct {
			Hash    string                                 `json:"result_hash"`
			Summary struct{ Artifact struct{ ID string } } `json:"summary"`
		} `json:"projection"`
	}
	trace, err := os.ReadFile(filepath.Join(directory, "comparison-tool-projections-final.json"))
	if err != nil || json.Unmarshal(trace, &calls) != nil {
		t.Fatal("actual full projections unavailable", err)
	}
	for name, wantFailure := range map[string]bool{"metric-en-2": true, "metric-zh-1": false, "catalog-en-2": false} {
		var record struct {
			Reply  string `json:"reply"`
			Sample struct {
				RunID string `json:"run_id"`
			} `json:"sample"`
		}
		raw, err := os.ReadFile(filepath.Join(directory, "plan11-compare-candidate-"+name+"-evidence.json"))
		if err != nil || json.Unmarshal(raw, &record) != nil {
			t.Fatal("actual case unavailable", name, err)
		}
		conclusion, err := plan11ParseConclusion(record.Reply)
		if err != nil {
			t.Fatal(err)
		}
		var digest, delivery string
		for _, call := range calls {
			if call.RunID != record.Sample.RunID || call.Tool != "tool.invoke" || call.Status != "succeeded" {
				continue
			}
			if call.Input.Category == "metric" && call.Input.Name == "query" {
				digest = call.Projection.Hash
			}
			if call.Input.Category == "workflow" && call.Input.Name == "publish_file" {
				delivery = call.Projection.Summary.Artifact.ID
			}
		}
		var original []byte
		paths, _ := filepath.Glob(filepath.Join(directory, "comparison-original-*.json"))
		for _, path := range paths {
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if fmt.Sprintf("%x", sha256.Sum256(body)) == digest {
				original = body
			}
		}
		report, err := os.ReadFile(filepath.Join(directory, "plan11-report-"+delivery+".md"))
		if err != nil || len(original) == 0 {
			t.Fatal("sealed original or immutable report missing", err)
		}
		for label, text := range map[string]string{"reply": record.Reply, "report": string(report)} {
			err := plan11VerifyExplicitPointClaims(original, conclusion, text)
			if (err != nil) != wantFailure {
				t.Fatal("actual recorded association classification differs", name, label, err)
			}
		}
		t.Logf("actual sealed source / reply / immutable report checked: %s (expected failure=%v)", name, wantFailure)
	}
}
