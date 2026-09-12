package installation

import (
	"bytes"
	"testing"
)

func TestCanonicalScriptUsesPlatformLineEndings(t *testing.T) {
	posix, err := CanonicalScript([]byte("#!/bin/sh\r\nset -eu\r\n"), POSIXShell)
	if err != nil || bytes.Contains(posix, []byte{'\r'}) || string(posix) != "#!/bin/sh\nset -eu\n" {
		t.Fatalf("POSIX canonicalization = %q, %v", posix, err)
	}
	powershell, err := CanonicalScript([]byte("$ErrorActionPreference = 'Stop'\nSet-StrictMode -Version Latest\n"), PowerShell)
	if err != nil || bytes.Contains(bytes.ReplaceAll(powershell, []byte("\r\n"), nil), []byte{'\n'}) {
		t.Fatalf("PowerShell canonicalization = %q, %v", powershell, err)
	}
}

func TestCanonicalScriptRejectsBOMAndUnknownShell(t *testing.T) {
	if _, err := CanonicalScript([]byte{0xef, 0xbb, 0xbf, '#'}, POSIXShell); err == nil {
		t.Fatal("UTF-8 BOM was accepted")
	}
	if _, err := CanonicalScript([]byte("script"), "cmd"); err == nil {
		t.Fatal("unknown shell was accepted")
	}
}
