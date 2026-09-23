package hostremoval

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/kakj-go/Argus/internal/resource"
)

// Use the PendingAction representation for comparison, persistence and Hash
// creation. PostgreSQL JSONB is free to reorder every embedded JSON object.
func canonicalPlan(plan Plan) ([]byte, error) {
	encoded, err := json.Marshal(plan)
	if err != nil {
		return nil, err
	}
	return resource.CanonicalJSON(encoded)
}

func plansEqual(left, right Plan) bool {
	left.CreatedAt, right.CreatedAt = time.Time{}, time.Time{}
	a, err := canonicalPlan(left)
	if err != nil {
		return false
	}
	b, err := canonicalPlan(right)
	return err == nil && bytes.Equal(a, b)
}
