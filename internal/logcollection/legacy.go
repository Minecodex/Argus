package logcollection

import (
	"context"
	"strings"
)

// Legacy profiles have fixed paths. Until migration removes them, their claims
// must participate in the same exclusion checks as individually managed rules.
func ConflictsWithLegacy(ctx context.Context, rule Config, key, platform string, profiles []string) (bool, error) {
	for _, profile := range profiles {
		switch {
		case key == "filelog" && profile == "file-log":
			overlap, err := FileOverlapForPlatform(ctx, rule, Config{Include: []string{"/var/log/*.log"}, Exclude: []string{"/var/log/argus-otelcol*.log"}}, platform == "windows_amd64")
			if err != nil || overlap {
				return overlap, err
			}
		case key == "journald" && profile == "linux-journald":
			return true, nil
		case key == "eventlog" && profile == "windows-event-log":
			if strings.EqualFold(rule.Channel, "Application") || strings.EqualFold(rule.Channel, "System") {
				return true, nil
			}
		}
	}
	return false, nil
}
