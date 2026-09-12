//go:build windows

package connector

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"unicode/utf16"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"google.golang.org/protobuf/types/known/anypb"
)

func runPrivilegedHelper(context.Context, []string) error {
	return errors.New("Windows Connector runs under LocalSystem and does not use a separate helper")
}

func usePrivilegedCollectorHelper() bool { return false }

func executePrivilegedCollector(context.Context, *anypb.Any) (*connectorv1.CollectorManagementResult, error) {
	return nil, errors.New("Windows privileged helper is unavailable")
}

func finalizeLocalUninstall(_ context.Context, dataDirectory string) error {
	programRoot := filepath.Join(os.Getenv("ProgramFiles"), "Argus", "Connector")
	collectorRoot := filepath.Join(os.Getenv("ProgramFiles"), "Argus", "Collector")
	collectorState := filepath.Join(os.Getenv("ProgramData"), "Argus", "Collector")
	script := `$ErrorActionPreference='SilentlyContinue';Wait-Process -Id ` + strconv.Itoa(os.Getpid()) + `;` +
		`& sc.exe delete ArgusConnector|Out-Null;& sc.exe delete ArgusCollector|Out-Null;` +
		`Remove-Item -LiteralPath ` + psLiteral(programRoot) + `,` + psLiteral(dataDirectory) + `,` + psLiteral(collectorRoot) + `,` + psLiteral(collectorState) + ` -Recurse -Force`
	encoded := encodePowerShell(script)
	return exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-EncodedCommand", encoded).Start()
}

func encodePowerShell(value string) string {
	words := utf16.Encode([]rune(value))
	bytes := make([]byte, len(words)*2)
	for index, word := range words {
		bytes[index*2], bytes[index*2+1] = byte(word), byte(word>>8)
	}
	return base64.StdEncoding.EncodeToString(bytes)
}
