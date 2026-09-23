package argusdev

import (
	"debug/elf"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestCollectorImageValidatesSelectedBinaryArchitecture(t *testing.T) {
	root := t.TempDir()
	writeELF := func(arch string, machine elf.Machine) {
		t.Helper()
		path := filepath.Join(root, "build", "otelcol", "dist", "linux-"+arch, "argus-otelcol")
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		header := make([]byte, 64)
		copy(header, []byte{0x7f, 'E', 'L', 'F', byte(elf.ELFCLASS64), byte(elf.ELFDATA2LSB), 1})
		binary.LittleEndian.PutUint16(header[16:], uint16(elf.ET_EXEC))
		binary.LittleEndian.PutUint16(header[18:], uint16(machine))
		binary.LittleEndian.PutUint32(header[20:], 1)
		binary.LittleEndian.PutUint16(header[52:], 64)
		if err := os.WriteFile(path, header, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeELF("amd64", elf.EM_X86_64)
	writeELF("arm64", elf.EM_AARCH64)
	for _, arch := range []string{"amd64", "arm64"} {
		if path, err := e2eCollectorImageDist(root, "linux/"+arch); err != nil || path != "build/otelcol/dist/linux-"+arch {
			t.Fatalf("%s selected %q: %v", arch, path, err)
		}
	}
	writeELF("amd64", elf.EM_AARCH64)
	if _, err := e2eCollectorImageDist(root, "linux/amd64"); err == nil {
		t.Fatal("arm64 binary was accepted in an amd64 image")
	}
	if _, err := e2eCollectorImageDist(root, "linux/../../arm64"); err == nil {
		t.Fatal("unsupported platform was accepted")
	}
}
