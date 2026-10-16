package tooltemplatefixture

import (
	"testing"

	"github.com/kakj-go/Argus/internal/presentation"
)

func TestEveryRegisteredToolHasReviewedPresentationCoverageAndSource(t *testing.T) {
	catalog, err := Build()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, binding := range catalog.Bindings {
		if seen[binding.Tool] {
			t.Fatal("duplicate tool binding", binding.Tool)
		}
		seen[binding.Tool] = true
		if _, err = presentation.ToolBinding(binding.Tool); err != nil {
			t.Fatal(err)
		}
		if binding.Mode != "template" {
			if binding.Hash != "" {
				t.Fatal("compact/file delivery duplicates a card", binding.Tool)
			}
			continue
		}
		if catalog.Assets[binding.Hash] == "" || len(binding.Reads) == 0 || len(binding.Data) == 0 || len(catalog.Cases[binding.Entry]) == 0 {
			t.Fatal("incomplete template coverage", binding.Tool)
		}
	}
	if len(seen) != 106 {
		t.Fatalf("registry coverage changed; review the declaration and fixtures: %d", len(seen))
	}
}
