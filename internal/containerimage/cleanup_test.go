package containerimage

import (
	"slices"
	"testing"
)

func TestCleanupPreservesAnotherReleaseDigest(t *testing.T) {
	inventory := "registry/repo:ours type sha256:123\nregistry/repo:other type sha256:123\nregistry/repo@sha256:123 type sha256:123\nregistry/unrelated:tag type sha256:456"
	refs := RemovalReferences([]string{"registry/repo:ours"}, inventory, nil)
	if !slices.Contains(refs, "registry/repo:ours@sha256:123") || slices.Contains(refs, "registry/repo@sha256:123") || slices.Contains(refs, "registry/repo:other") {
		t.Fatalf("unsafe shared digest cleanup: %v", refs)
	}
	refs = RemovalReferences([]string{"registry/repo:ours", "registry/repo:other"}, inventory, nil)
	if !slices.Contains(refs, "registry/repo@sha256:123") || slices.Contains(refs, "registry/unrelated:tag") {
		t.Fatalf("owned aliases were not cleaned: %v", refs)
	}
	refs = RemovalReferences([]string{"registry/repo:ours", "registry/repo:other"}, inventory, []string{"registry/repo@sha256:123"})
	if slices.Contains(refs, "registry/repo@sha256:123") {
		t.Fatal("removed a digest still used by a Pod")
	}
}
