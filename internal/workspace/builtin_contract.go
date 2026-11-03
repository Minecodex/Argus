package workspace

import "github.com/kakj-go/Argus/internal/sandbox"

// Tool contracts are versioned separately from the runtime's RPC identity.
// Existing Run snapshots must not silently acquire changed builtin contracts.
func workspaceBuiltinVersion(identity sandbox.WorkspaceIdentity) string {
	return "workspace-tools/v9:" + identity.Version()
}

func workspaceBuiltinDescription(name string) string {
	switch name {
	case "read":
		return "Read an existing UTF-8 file in the persistent offline /workspace. Select a bounded line range and inspect partial before assuming the complete file was read. No network access."
	case "write":
		return "Create or replace the complete contents of a persistent /workspace file. The result records the saved path, size and content hash; saving does not create a downloadable delivery. When the user requested a report/file export or download, search workflow, describe publish_file, then publish only the requested authorized file and return its actual delivery reference. No network access."
	case "edit":
		return "Replace specified text in an existing persistent /workspace file. Inspect the returned saved hash. This changes the working file; existing immutable deliveries remain separate. For a requested updated file delivery, use the authorized workflow/publish_file capability after normal search and describe. No network access."
	case "bash":
		return "Execute a bounded command in the persistent offline /workspace. Inspect exit_code and partial. For a requested numeric report use argus-evidence-report ORIGINAL_METRIC_FILE --expected-sha256 ORIGINAL_SHA --dashboard-manifest DELIVERED_MANIFEST --locale zh-CN --json-out CONCLUSION_PATH --markdown-out REPORT_PATH. Use en-US for English. Include each executed job manifest, or omit that flag when none was queried; this assembles complete tables and one valid JSON block. Publish the generated Markdown directly. Original query metadata supplies units/type and direct-selector status; --unit and --metric-type are only consistency hints and conflicts fail. Legacy files require a catalog unit and retain unknown type. Supply --expected-sha256 from that query's workspace_file.sha256 and --dashboard-manifest for every dashboard job. A later read/bash result_hash is not a file checksum. Counter readings do not establish interval activity/rate; derived expressions need expression-aware units. Use --transform percent only for confirmed ratios. Optional --notes is an existing UTF-8 notes FILE path. Preserve generated facts/keys/identifiers; never retype opaque IDs. Requested exports still require authorized workflow/publish_file after search and describe. No network access."
	}
	return ""
}

func markWorkspaceFileSaved(result map[string]any) {
	result["file_saved"] = true
	// This is a fact about this operation, not a claim that no historical
	// delivery exists for an earlier version of the same pathname.
	result["delivery_created"] = false
	result["delivery_next_tool"] = map[string]any{"category": "workflow", "name": "publish_file", "when": "user_requested_file_export_or_download", "requires_search_and_describe": true}
}
