package argusctl

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// The upstream hook concatenates repository:tag. Preserve a digest-qualified
// installation-tools reference. The restricted remote-runtime kubectl cannot
// authenticate with the Hook's ServiceAccount and must not be substituted.
func pinnedCRDHookImage(reference string) (string, string, error) {
	tagged, digest, ok := strings.Cut(reference, "@sha256:")
	if !ok || len(digest) != 64 {
		return "", "", fmt.Errorf("CRD hook requires a frozen installation-tools image digest")
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return "", "", fmt.Errorf("CRD hook image digest invalid")
	}
	index := strings.LastIndex(tagged, ":")
	if index <= strings.LastIndex(tagged, "/") || index == len(tagged)-1 {
		return "", "", fmt.Errorf("CRD hook requires a versioned installation-tools reference")
	}
	repository := tagged[:index]
	if repository[strings.LastIndex(repository, "/")+1:] != "argus-install-tools" {
		return "", "", fmt.Errorf("CRD hook requires the dedicated installation-tools image")
	}
	return repository, tagged[index+1:] + "@sha256:" + digest, nil
}
