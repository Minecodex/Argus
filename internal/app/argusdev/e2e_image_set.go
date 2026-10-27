package argusdev

import "fmt"

func validateE2ECoreImageSet(images []plan6Image, remote bool) error {
	required := map[string]bool{"backend": true, "web": true, "minio": true, "workspace": true, "workspace-egress": true, "otelcol": true, "install-tools": true}
	if remote {
		required["remote-runtime"] = true
	}
	seen := map[string]bool{}
	for _, image := range images {
		if !required[image.Name] || seen[image.Name] {
			return fmt.Errorf("unexpected or duplicate core image %s", image.Name)
		}
		seen[image.Name] = true
	}
	for name := range required {
		if !seen[name] {
			return fmt.Errorf("required core image %s is missing", name)
		}
	}
	return nil
}
