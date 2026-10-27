package kuberneteshelm

import (
	"slices"
	"strconv"
	"time"

	"k8s.io/apimachinery/pkg/util/validation"
)

// ValidateReleases treats Connector metadata as an observation, never as a new
// resource authorization root. No response may move to another Namespace.
func ValidateReleases(rows []Release, r Request) error {
	if len(rows) > 200 {
		return ErrBudget
	}
	seen := map[string]bool{}
	for _, v := range rows {
		if v.Namespace != r.Namespace.Name || len(v.Name) > 53 || len(validation.IsDNS1123Label(v.Name)) != 0 || r.Operation == "history" && v.Name != r.Name || v.Revision < 1 || len(v.Chart) > 253 || len(v.ChartVersion) > 128 || !slices.Contains([]string{"deployed", "failed", "uninstalled", "superseded", "uninstalling", "pending-install", "pending-upgrade", "pending-rollback", "unknown"}, v.Status) || v.UpdatedAt.After(time.Now().Add(5*time.Minute)) {
			return ErrInvalid
		}
		key := v.Name
		if r.Operation == "history" {
			key += "/" + strconv.Itoa(v.Revision)
		}
		if seen[key] {
			return ErrInvalid
		}
		seen[key] = true
	}
	return nil
}
