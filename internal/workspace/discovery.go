package workspace

import "github.com/kakj-go/Argus/internal/mcp"

var workspaceDiscovery = map[string]mcp.Discovery{
	"workflow.import_result": {
		SchemaVersion: "argus.tool_discovery/v1", Revision: "1",
		Title:             "导入授权查询结果 / Import authorized tool result",
		Description:       "将当前仍可授权的 Tool result_ref 导入本会话 Workspace 文件，以便离线分析；只导入数据，不包含模板、凭据或私有提交材料。 Import currently authorized result data into a session workspace for offline analysis.",
		Keywords:          []string{"导入", "查询结果", "工作区", "文件", "分析", "import", "result", "workspace", "file"},
		ResultDescription: "返回工作区文件元数据与来源引用，后续目录访问继续受来源授权约束。 Returns workspace file metadata and provenance, with continuing source authorization.",
		Preconditions:     []string{"result_ref 必须来自当前会话可访问结果；path 必须位于当前 Workspace。 Use an accessible result_ref and a path within the current workspace."},
		Examples:          []mcp.DiscoveryExample{{Request: "把刚才的查询结果导入工作区供 Python 分析。 / Import the query result for Python analysis.", Guidance: "使用返回的 result_ref 和相对路径如 data/query.json；不能传模板或私有记录引用。 Use a returned result_ref and a relative data path, never a template/private reference."}},
	},
	"workflow.publish_file": {
		SchemaVersion: "argus.tool_discovery/v1", Revision: "1",
		Title:             "发布文件下载 / Publish workspace file",
		Description:       "核验当前 Workspace 中已有文件，将其发布为不可变会话交付；同名工作文件后续修改不改变历史下载。 Verify an existing workspace file and publish an immutable conversation delivery.",
		Keywords:          []string{"发布", "下载", "导出", "文件", "产物", "交付", "publish", "download", "export", "artifact", "file"},
		ResultDescription: "返回交付 ID、文件名、大小、内容 Hash 与受控下载信息；不是外部 URL。 Returns immutable delivery identity, size, content hash and authorized download metadata.",
		Preconditions:     []string{"文件必须已在当前 Workspace 生成，且仍有来源访问权限；受单文件上限约束。 The file must exist in the current workspace and pass provenance/size checks."},
		Examples:          []mcp.DiscoveryExample{{Request: "将 summary.csv 发布为可下载文件。 / Publish summary.csv for download.", Guidance: "先通过离线工具生成文件，再传 path=summary.csv；可选 name/media_type。 Create the file offline, then pass its workspace path and optional name/media_type."}},
	},
}
