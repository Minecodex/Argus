// Package containerimage handles the exact containerd references owned by an
// image-loading operation. It never prunes layers or unrelated repositories.
package containerimage

import "strings"

// RemovalReferences includes the tag and its loader-created digest aliases.
// A canonical digest shared by another tag is retained for that tag's users.
func RemovalReferences(requested []string, inventory string, protected []string) []string {
	entries := map[string]string{}
	for _, line := range strings.Split(inventory, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && strings.HasPrefix(fields[2], "sha256:") {
			entries[fields[0]] = fields[2]
		}
	}
	owned := map[string]bool{}
	for _, ref := range requested {
		owned[ref] = true
	}
	inUse := map[string]bool{}
	for _, ref := range protected {
		inUse[ref] = true
	}
	seen := map[string]bool{}
	var result []string
	add := func(ref string) {
		if !seen[ref] {
			seen[ref] = true
			result = append(result, ref)
		}
	}
	for _, ref := range requested {
		add(ref)
		digest := entries[ref]
		if digest == "" || strings.Contains(ref, "@") {
			continue
		}
		colon := strings.LastIndex(ref, ":")
		if colon <= strings.LastIndex(ref, "/") {
			continue
		}
		repository := ref[:colon]
		add(ref + "@" + digest)
		shared := false
		for other, otherDigest := range entries {
			if otherDigest == digest && !owned[other] && !strings.Contains(other, "@") && strings.HasPrefix(other, repository+":") {
				shared = true
				break
			}
		}
		if !shared && !inUse[repository+"@"+digest] {
			add(repository + "@" + digest)
		}
	}
	return result
}
