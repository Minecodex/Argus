package argusdev

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPlan8RawfileImportedIndexRequiresOriginalPlatformDescriptorBytes(t *testing.T) {
	const imported = "sha256:4f34a5dc828fb3ea57f3c0e6854b4fe9997b6ae986f65e7acc7739af2b72c20b"
	files := map[string]string{plan8RawfileIndexDigest: "upstream-index.json", plan8RawfileAMD64Digest: "linux-amd64.json", imported: "imported-index.json"}
	read := func(digest string) ([]byte, error) {
		name := files[digest]
		if name == "" {
			return nil, errors.New("unverified descriptor")
		}
		return os.ReadFile(filepath.Join("testdata", "rawfile-v0151", name))
	}
	for _, digest := range []string{plan8RawfileIndexDigest, plan8RawfileAMD64Digest, imported} {
		valid, err := plan8RawfileDescriptorIdentity(digest, "linux/amd64", read)
		if err != nil || !valid {
			t.Fatalf("original descriptor chain was not verified: %v", err)
		}
	}
	if valid, _ := plan8RawfileDescriptorIdentity(imported, "linux/arm64", read); valid {
		t.Fatal("another platform borrowed amd64 acceptance")
	}
	if valid, _ := plan8RawfileDescriptorIdentity("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "linux/amd64", read); valid {
		t.Fatal("another runtime index borrowed this image identity")
	}
	for _, damaged := range []string{plan8RawfileIndexDigest, plan8RawfileAMD64Digest, imported} {
		valid, err := plan8RawfileDescriptorIdentity(imported, "linux/amd64", func(digest string) ([]byte, error) {
			raw, e := read(digest)
			if digest == damaged {
				raw = append(raw, '\n')
			}
			return raw, e
		})
		if valid || err == nil {
			t.Fatal("descriptor bytes no longer matched SHA256 but were accepted", damaged)
		}
	}
}
