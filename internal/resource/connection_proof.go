package resource

import (
	"crypto/sha256"
	"encoding/json"
	"slices"

	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// DecodeConnectionTestProof verifies the complete typed plan using the same
// serialization as its writer. PostgreSQL jsonb changes whitespace and key order;
// hashing returned storage bytes directly would reject genuine stored proofs.
// Callers additionally enforce status, expiry, subject, target and versions.
func DecodeConnectionTestProof(test db.ConnectionTest) (ConnectionTestPlan, ConnectionTestResult, error) {
	var plan ConnectionTestPlan
	var result ConnectionTestResult
	if err := json.Unmarshal(test.RequestPlan, &plan); err != nil {
		return plan, result, ErrConnectionTestNeeded
	}
	encoded, err := json.Marshal(plan)
	if err != nil {
		return plan, result, ErrConnectionTestNeeded
	}
	digest := sha256.Sum256(encoded)
	if !subtleEqual(digest[:], test.RequestHash) || json.Unmarshal(test.Result, &result) != nil {
		return plan, result, ErrConnectionTestNeeded
	}
	if len(plan.ResolvedIPs) > 0 && !slices.Equal(plan.ResolvedIPs, result.ResolvedIPs) {
		return plan, result, ErrConnectionTestNeeded
	}
	return plan, result, nil
}
