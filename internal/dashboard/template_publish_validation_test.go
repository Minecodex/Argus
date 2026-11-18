package dashboard

import (
	"strings"
	"testing"
)

func TestPublicManagedTemplatesRemainPublishableAsPersonalCopies(t *testing.T) {
	for _, definition := range BuiltinTemplates() {
		if privateSpec(definition.Spec) {
			continue
		}
		t.Run(definition.Key, func(t *testing.T) {
			report := Validate(definition.Spec)
			if !report.Valid {
				t.Fatalf("managed copy cannot pass publication validation: %+v", report.Issues)
			}
		})
	}
}

func TestTemplateRegistryRejectsUnpublishablePanelTitles(t *testing.T) {
	definition := redisTemplate()
	definition.Spec.Panels[0].Title = strings.Repeat("x", 241)
	if _, _, err := validateTemplate(definition); err == nil {
		t.Fatal("registry accepted a definition that personal publication rejects")
	}
}
