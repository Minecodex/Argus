package toolgateway

// Public business titles also supply the generated conversation UI names.
var dashboardToolTitles = map[string]string{
	"list":               "仪表盘列表 / List dashboards",
	"get":                "仪表盘获取 / Get dashboard",
	"context.candidates": "筛选候选获取 / Get filter candidates",
	"context.resolve":    "分析条件解析 / Resolve analysis conditions",
	"query":              "仪表盘查询 / Query dashboard",
	"query.get":          "仪表盘查询结果 / Get dashboard query result",
	"query.cancel":       "仪表盘查询取消 / Cancel dashboard query",
	"query.resume":       "仪表盘查询恢复 / Resume dashboard query",
	"catalog":            "遥测数据目录 / Discover telemetry data",
	"catalog.resources":  "可用分析资源 / List analysis resources",
	"draft.create":       "仪表盘草稿创建 / Create dashboard draft",
	"draft.get":          "仪表盘草稿获取 / Get dashboard draft",
	"draft.validate":     "仪表盘草稿校验 / Validate dashboard draft",
	"draft.save":         "仪表盘草稿保存 / Save dashboard draft",
	"publish.preview":    "仪表盘发布预览 / Preview dashboard publication",
}
