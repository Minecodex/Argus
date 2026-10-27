package configbundle

import (
	"slices"
	"sort"
	"strconv"

	"github.com/kakj-go/Argus/internal/logcollection"
)

func protectedLogExclusions(values []string, platform string) []string {
	values = append(slices.Clone(values), logcollection.ReservedRuntimeExclusions(platform == "windows_amd64")...)
	if platform != "windows_amd64" {
		values = append(values, logcollection.KubernetesContainerExclusions()...)
	}
	sort.Strings(values)
	return collectorLiterals(slices.Compact(values))
}

func protectedLogOperator() map[string]any {
	// File metadata is checked before any user JSON can populate attributes.
	return map[string]any{"id": "argus_runtime_path_guard", "type": "filter", "expr": `attributes["log.file.path_resolved"] == nil || attributes["log.file.path_resolved"] matches ` + strconv.Quote(logcollection.HostFileLogPathPattern), "on_error": "drop_quiet"}
}

func jsonFileLogOperators() []map[string]any {
	return []map[string]any{
		protectedLogOperator(),
		{"id": "argus_capture_file_path", "type": "copy", "from": `attributes["log.file.path"]`, "to": `resource["argus.file_origin.path"]`, "on_error": "drop_quiet"},
		{"id": "argus_capture_resolved_path", "type": "copy", "from": `attributes["log.file.path_resolved"]`, "to": `resource["argus.file_origin.resolved"]`, "on_error": "drop_quiet"},
		{"type": "json_parser", "parse_from": "body", "parse_to": "attributes", "on_error": "send_quiet"},
		{"id": "argus_restore_file_path", "type": "move", "from": `resource["argus.file_origin.path"]`, "to": `attributes["log.file.path"]`, "on_error": "drop_quiet"},
		{"id": "argus_restore_resolved_path", "type": "move", "from": `resource["argus.file_origin.resolved"]`, "to": `attributes["log.file.path_resolved"]`, "on_error": "drop_quiet"},
	}
}
