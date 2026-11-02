package argusdev

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboard"
)

func TestBusinessWorkflowAuthoringReadbackRecordedRealSamples(t *testing.T) {
	root := os.Getenv("ARGUS_PLAN11_AUTHORING_REPLAY_DIR")
	if root == "" {
		t.Skip("explicit immutable actual native tool traces required")
	}
	for _, file := range []string{"plan11-authoring-run-01-tools.json", "plan11-authoring-run-02-tools.json"} {
		t.Run(file, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(root, file))
			if err != nil {
				t.Fatal(err)
			}
			var calls []struct {
				Tool, Status string
				Input        struct{ Name string }
				Projection   struct {
					Summary struct {
						Preview struct {
							Sample string `json:"sample_json"`
						}
					}
				}
			}
			if json.Unmarshal(raw, &calls) != nil {
				t.Fatal("actual tool trace is invalid")
			}
			checked := false
			for _, call := range calls {
				if call.Tool != "tool.invoke" || call.Input.Name != "publish.preview" || call.Status != "succeeded" {
					continue
				}
				var sample struct{ Execution dashboard.Execution }
				if json.Unmarshal([]byte(call.Projection.Summary.Preview.Sample), &sample) != nil || len(sample.Execution.Resources) != 1 {
					t.Fatal("actual sample unavailable")
				}
				host := sample.Execution.Resources[0].ID.String()
				points, err := plan11AuthoringSamplePoints(sample.Execution, host)
				if err != nil || len(points) == 0 {
					t.Fatal("actual real sample rejected", err)
				}
				if _, err = plan11AuthoringSamplePoints(sample.Execution, uuid.NewString()); err == nil {
					t.Fatal("foreign host accepted")
				}
				sample.Execution.Partial = true
				if _, err = plan11AuthoringSamplePoints(sample.Execution, host); err == nil {
					t.Fatal("partial sample certified")
				}
				checked = true
			}
			if !checked {
				t.Fatal("no actual preview checked")
			}
		})
	}
}
