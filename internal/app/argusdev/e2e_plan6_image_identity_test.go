package argusdev

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestPlan6RegistryIdentityVerifiesPlatformAndContent(t *testing.T) {
	hash := func(raw []byte) string {
		sum := sha256.Sum256(raw)
		return "sha256:" + hex.EncodeToString(sum[:])
	}
	config := hash([]byte("accepted config"))
	manifest := []byte(fmt.Sprintf(`{"config":{"digest":%q}}`, config))
	child := hash(manifest)
	index := []byte(fmt.Sprintf(`{"manifests":[{"digest":%q,"platform":{"os":"linux","architecture":"amd64"}},{"digest":%q,"platform":{"os":"unknown","architecture":"unknown"}}]}`, child, hash([]byte("attestation"))))
	root := hash(index)
	fetch := func(digest string) ([]byte, error) {
		if digest == root {
			return index, nil
		}
		if digest == child {
			return manifest, nil
		}
		return nil, fmt.Errorf("unexpected digest")
	}
	selected, actualConfig, err := plan6RegistryIdentity(root, "linux/amd64", fetch)
	if err != nil || selected != child || actualConfig != config {
		t.Fatal(selected, actualConfig, err)
	}
	if _, _, err := plan6RegistryIdentity(root, "linux/arm64", fetch); err == nil {
		t.Fatal("different platform accepted")
	}
	if _, _, err := plan6RegistryIdentity(root, "linux/amd64", func(digest string) ([]byte, error) {
		raw, err := fetch(digest)
		return append(raw, ' '), err
	}); err == nil {
		t.Fatal("altered immutable content accepted")
	}
	var duplicate map[string]any
	_ = json.Unmarshal(index, &duplicate)
	duplicate["manifests"] = []any{duplicate["manifests"].([]any)[0], duplicate["manifests"].([]any)[0]}
	ambiguous, _ := json.Marshal(duplicate)
	if _, _, err := plan6RegistryIdentity(hash(ambiguous), "linux/amd64", func(string) ([]byte, error) { return ambiguous, nil }); err == nil {
		t.Fatal("ambiguous platform accepted")
	}
}

func TestPlan6RuntimeConfigRequiresTheExactPinnedRoot(t *testing.T) {
	image := plan6Image{ImageID: "sha256:" + strings.Repeat("1", 64), PlatformManifestID: "sha256:" + strings.Repeat("2", 64), ConfigID: "sha256:" + strings.Repeat("3", 64)}
	for _, id := range []string{image.ImageID, image.PlatformManifestID, image.ConfigID, "registry/repository@" + image.ImageID} {
		if _, accepted := plan6RuntimeImageIdentity(image, "registry/repository:accepted@"+image.ImageID, id); !accepted {
			t.Fatal("verified identity rejected", id)
		}
	}
	for _, tc := range []struct{ configured, id string }{
		{"registry/repository:accepted", image.ConfigID},
		{"registry/repository:accepted@" + image.PlatformManifestID, image.ConfigID},
		{"registry/repository:accepted@" + image.ImageID, "sha256:" + strings.Repeat("4", 64)},
	} {
		if _, accepted := plan6RuntimeImageIdentity(image, tc.configured, tc.id); accepted {
			t.Fatal("unverified identity accepted", tc)
		}
	}
}

func TestPlan6CandidateReadsImagetoolsDescriptorAndRejectsConflicts(t *testing.T) {
	digest := "sha256:" + strings.Repeat("1", 64)
	for _, raw := range []string{
		fmt.Sprintf(`{"containerimage.descriptor":{"mediaType":"application/vnd.oci.image.index.v1+json","digest":%q,"size":856},"image.name":"127.0.0.1:5001/argus/backend"}`, digest),
		fmt.Sprintf(`{"containerimage.digest":%q}`, digest),
		fmt.Sprintf(`{"containerimage.digest":%q,"containerimage.descriptor":{"digest":%q}}`, digest, digest),
	} {
		actual, err := plan6CandidateManifestDigest([]byte(raw))
		if err != nil || actual != digest {
			t.Fatal(actual, err)
		}
	}
	for _, raw := range []string{
		`{}`, `{"containerimage.descriptor":{"digest":"unknown"}}`,
		fmt.Sprintf(`{"containerimage.digest":%q,"containerimage.descriptor":{"digest":%q}}`, digest, "sha256:"+strings.Repeat("2", 64)),
	} {
		if _, err := plan6CandidateManifestDigest([]byte(raw)); err == nil {
			t.Fatal("invalid or conflicting metadata accepted", raw)
		}
	}
}
