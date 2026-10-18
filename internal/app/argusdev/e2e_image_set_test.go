package argusdev

import "testing"

func TestRemoteRuntimeMustBeIncludedWithoutWaivingOtherCoreImages(t *testing.T) {
	images := []plan6Image{{Name: "backend"}, {Name: "web"}, {Name: "minio"}, {Name: "workspace"}, {Name: "workspace-egress"}, {Name: "otelcol"}}
	if err := validateE2ECoreImageSet(images, false); err != nil {
		t.Fatal(err)
	}
	if err := validateE2ECoreImageSet(images, true); err == nil {
		t.Fatal("remote runtime was optional")
	}
	images = append(images, plan6Image{Name: "remote-runtime"})
	if err := validateE2ECoreImageSet(images, true); err != nil {
		t.Fatal(err)
	}
	images[0].Name = "remote-runtime"
	if err := validateE2ECoreImageSet(images, true); err == nil {
		t.Fatal("duplicate image hid the missing backend")
	}
}
