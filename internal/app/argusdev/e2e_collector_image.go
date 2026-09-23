package argusdev

import (
	"debug/elf"
	"fmt"
	"path/filepath"
	"strings"
)

func e2eCollectorImageDist(root, platform string) (string, error) {
	var expected elf.Machine
	switch platform {
	case "linux/amd64":
		expected = elf.EM_X86_64
	case "linux/arm64":
		expected = elf.EM_AARCH64
	default:
		return "", fmt.Errorf("%w: unsupported Collector image platform %q", errCapability, platform)
	}
	dist := "build/otelcol/dist/" + strings.ReplaceAll(platform, "/", "-")
	binary, err := elf.Open(filepath.Join(root, filepath.FromSlash(dist), "argus-otelcol"))
	if err != nil {
		return "", fmt.Errorf("%w: inspect Collector %s ELF: %v", errCapability, platform, err)
	}
	defer binary.Close()
	if binary.Class != elf.ELFCLASS64 || binary.Machine != expected {
		return "", fmt.Errorf("%w: Collector %s binary has %s/%s", errCapability, platform, binary.Class, binary.Machine)
	}
	return dist, nil
}
