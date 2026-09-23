package toolruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
)

// SkillContext is an immutable context extension, not a tool registration or
// authorization mechanism. This release deliberately has no editor/marketplace.
type SkillContext struct {
	SchemaVersion string `json:"schema_version"`
	ID            string `json:"id"`
	Revision      string `json:"revision"`
	ContentHash   string `json:"content_hash"`
	Text          string `json:"text"`
}
type SkillContextSource interface {
	LoadContext(context.Context, Principal) ([]SkillContext, error)
}

func ValidateSkillContexts(values []SkillContext) error {
	seen := map[string]bool{}
	size := 0
	for _, value := range values {
		hash := sha256.Sum256([]byte(value.Text))
		size += len(value.Text)
		if value.SchemaVersion != "argus.skill_context/v1" || value.ID == "" || value.Revision == "" || seen[value.ID] || value.ContentHash != hex.EncodeToString(hash[:]) || size > 64<<10 {
			return Error{Kind: "SKILL_CONTEXT_INVALID"}
		}
		seen[value.ID] = true
	}
	return nil
}
