package sshtarget

import (
	"encoding/base64"
	"encoding/binary"
	"testing"
	"unicode/utf16"
)

func TestPowerShellCommandUsesUTF16LE(t *testing.T) {
	command := PowerShellCommand("Write-Output x64")
	const prefix = "powershell.exe -NoProfile -NonInteractive -EncodedCommand "
	raw, err := base64.StdEncoding.DecodeString(command[len(prefix):])
	if err != nil {
		t.Fatal(err)
	}
	values := make([]uint16, len(raw)/2)
	for index := range values {
		values[index] = binary.LittleEndian.Uint16(raw[index*2:])
	}
	if got := string(utf16.Decode(values)); got != "Write-Output x64" {
		t.Fatalf("decoded command = %q", got)
	}
}

func TestParseLinuxEvidenceRequiresPrivilegeServiceAndDisk(t *testing.T) {
	evidence, err := parseLinuxEvidence([]byte("architecture=x86_64\ndistribution_version=ubuntu:24.04\nservice_manager=systemd\nprivileged=true\nfree_disk_bytes=1073741824\n"))
	if err != nil {
		t.Fatal(err)
	}
	evidence.Platform, evidence.Architecture = "linux", "amd64"
	if _, err = validateEvidence(evidence); err != nil {
		t.Fatalf("complete Linux preflight evidence rejected: %v", err)
	}
	for _, mutate := range []func(*Evidence){
		func(value *Evidence) { value.Privileged = false },
		func(value *Evidence) { value.ServiceManager = "unavailable" },
		func(value *Evidence) { value.FreeDiskBytes = MinimumInstallFreeBytes - 1 },
	} {
		candidate := evidence
		mutate(&candidate)
		if _, err = validateEvidence(candidate); err == nil {
			t.Fatalf("invalid preflight evidence accepted: %+v", candidate)
		}
	}
}
