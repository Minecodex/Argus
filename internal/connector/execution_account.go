package connector

import (
	"github.com/kakj-go/Argus/internal/resource"
	"regexp"
	"strings"
)

var executionSID = regexp.MustCompile(`^S-1-[0-9]+(?:-[0-9]+)+$`)

func validExecutionAccount(platform string, account resource.HostExecutionAccount) bool {
	if account.Username == "" || len(account.Username) > 256 || strings.TrimSpace(account.Username) != account.Username || strings.ContainsAny(account.Username, "\x00\r\n") {
		return false
	}
	if platform == "windows" {
		return len(account.SID) <= 184 && executionSID.MatchString(account.SID)
	}
	return platform == "linux" && account.SID == ""
}
