package toolruntime

import (
	"crypto/sha256"
	"encoding/hex"
)

// QueryFileReadMatches verifies actual full read bytes. It is not a proof for
// arbitrary scripts, a metadata lookup or a model-authored checksum claim.
func QueryFileReadMatches(content string, byteSize, startLine int, partial bool, expectedSHA string, expectedBytes int) bool {
	if partial || startLine != 1 || byteSize != expectedBytes || expectedBytes < 0 {
		return false
	}
	data := []byte(content)
	if len(data) == expectedBytes+1 && data[len(data)-1] == '\n' {
		data = data[:len(data)-1]
	}
	if len(data) != expectedBytes {
		return false
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]) == expectedSHA
}
