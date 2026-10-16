package alerting

import (
	"strings"

	"github.com/kakj-go/Argus/internal/mcp"
)

// Metadata supplies authored business vocabulary to the shared conjunctive
// search. It does not route model requests or change discovery permissions.
func alertDiscovery(id string) mcp.Discovery {
	parts := strings.Split(id, ".")
	kind, operation := parts[1], parts[2]
	noun := map[string]string{"policy": "告警策略与指标规则", "event": "告警事件", "history": "告警历史", "silence": "告警屏蔽", "inhibition": "告警抑制规则", "channel": "通知渠道", "route": "通知路由", "notification": "告警通知", "settings": "告警评估账号", "grouping": "告警合并设置", "template": "告警消息模板"}[kind]
	english := map[string]string{"policy": "alert policy and metric rules", "event": "alert event", "history": "alert history", "silence": "temporary alert silence / maintenance window", "inhibition": "alert inhibition rule", "channel": "notification channel", "route": "notification route", "notification": "alert notification", "settings": "alert evaluator account", "grouping": "notification grouping", "template": "notification message template"}[kind]
	verb := map[string]string{"create": "创建", "update": "修改", "delete": "删除", "enable": "启用", "disable": "停用", "list": "查询列表", "get": "查看详情", "query": "查询", "test": "测试", "resend": "补发", "acknowledge": "确认处理", "assign": "指派负责人", "comment": "添加备注"}[operation]
	keywords := []string{"告警", "报警", "监控", "指标", noun, verb + noun, verb + "告警", "alert", "alarm", "monitoring", "metrics", kind, operation}
	keywords = append(keywords, map[string][]string{
		"policy":       {"policy", "rule", "rules", "threshold", "CPU", "memory", "策略", "规则", "阈值", "恢复"},
		"silence":      {"mute", "muting", "silence", "suppress", "suppression", "temporary", "maintenance", "window", "downtime", "屏蔽", "静默", "临时", "维护", "一小时"},
		"inhibition":   {"inhibit", "suppress", "suppression", "dependency", "抑制", "依赖"},
		"channel":      {"webhook", "robot", "smtp", "email", "mail", "dingtalk", "wecom", "feishu", "渠道", "机器人", "邮件"},
		"route":        {"routing", "receiver", "destination", "路由", "接收目标"},
		"event":        {"active", "firing", "incident", "acknowledge", "assignment", "当前", "事件", "异常", "处理"},
		"history":      {"resolved", "recovery", "timeline", "历史", "恢复", "时间线"},
		"notification": {"delivery", "receipt", "retry", "resend", "通知", "投递", "受理", "重试", "补发"},
		"settings":     {"evaluator", "service", "account", "设置", "评估账号"},
		"grouping":     {"merge", "group", "deduplicate", "wait", "repeat", "合并", "分组", "去重"},
		"template":     {"message", "render", "模板", "消息"},
	}[kind]...)
	keywords = append(keywords, map[string][]string{"create": {"add", "新增"}, "update": {"modify", "edit", "修改", "编辑"}, "delete": {"remove", "删除"}, "enable": {"activate", "启用"}, "disable": {"deactivate", "stop", "停用"}, "list": {"find", "exists", "查询", "存在"}, "get": {"inspect", "详情"}, "query": {"find", "查询"}, "comment": {"note", "备注", "评论"}}[operation]...)
	if operation == "enable" || operation == "disable" {
		keywords = append(keywords, "update", "state", "status", "toggle", "启停", "修改状态", "开关")
	}
	if kind == "event" && operation == "query" {
		keywords = append(keywords, "CPU", "memory", "disk", "cluster", "kubernetes", "alerts", "list", "status", "主机", "集群", "内存", "磁盘", "当前告警", "评估健康", "无数据", "查询失败")
	}
	if kind == "silence" && operation == "disable" {
		keywords = append(keywords, "unmute", "unsilence", "解除", "解除屏蔽")
	}
	request, guidance := "查询当前告警，并区分正常、无数据和评估失败", "Use read results' authorized scope, completeness and evaluation health; absence of events alone does not establish health."
	if kind == "silence" {
		request, guidance = "为指定资源和指标临时屏蔽一小时，预览后等我确认 / temporarily mute alerts for a maintenance window", "List existing silences for edits or unsilence; resolve each named resource to its actual UUID. Use current_time_utc from server execution facts for an immediate starts_at and add the requested duration for ends_at; both must be absolute RFC3339 timestamps. Freeze explicit resources, metric, start/end time and reason. Changes require host confirmation; cancellation leaves configuration unchanged."
		guidance += " To revoke or unsilence (解除屏蔽), use silence.disable.preview and keep the existing window/reason for history; silence.delete.preview is for an explicit request to delete the configuration."
		guidance += " For a named host or cluster, put the resolved UUID in spec.resource_ids. Never use a guessed cluster/name matcher instead of this identity; matchers filter observed labels only. An explicit empty resource_ids array means all evaluator-authorized resources and is allowed only when that full scope was requested."
	} else if kind == "policy" {
		request, guidance = "创建 CPU 和内存双指标告警，预览后等我确认 / create CPU and memory alert rules", "Resolve each named resource to its actual UUID with resource tools; use resources.mode=explicit and resources.ids for concrete requested hosts or clusters. Dynamic matchers evaluate labels only, never the resource name field. List channels and evaluator settings, freeze all rules atomically, and inspect returned resource_ids before waiting for confirmation; an empty scope does not satisfy a named-resource request. Preserve explicitly requested display units in each rule.unit, including % for percentage conditions; numeric thresholds use the stated scale. Updates require the current configuration version and an explicit id on every rule. Reuse each existing rule UUID from the current configuration verbatim; never omit or regenerate an unchanged rule id. Supply a new UUID only for a genuinely new rule, and preserve all unrelated fields."
		guidance += " 若只需启停策略，查找对应的启用或停用预览工具，只提交当前id和版本。不要重建完整规则配置、描述、范围或通知。Full policy replacement is for configuration edits; a status-only request uses its dedicated status action."
	} else if kind == "channel" || kind == "notification" {
		request, guidance = "查看通知渠道及投递结果 / inspect notification channels and deliveries", "Use existing encrypted credential references; never request or return raw credentials. Test or resend previews disclose targets and require host confirmation; resend preserves the original delivery."
	}
	description := verb + noun + " / " + operation + " " + english + ". Uses current enterprise and explicit resource authorization. Writes freeze a preview for one host-owned confirmation; reads preserve scope and health."
	result := "Authorized read-only items or a PendingAction with public impact; no private tokens or credential values."
	if kind == "event" && operation == "query" {
		description = "查询主机/集群的当前告警实例、生命周期、屏蔽状态及评估事实 / Query current host or cluster alerts and event instances, lifecycle and mute state. Returns authorized scope, latest evaluation time, health and completeness, including zero events, no data (NoData) and query errors (Error); policy definitions or metric values alone cannot establish current alert state."
		description += " 仅查询normal/pending/firing/recovering，始终排除resolved/closed；恢复或关闭历史必须另用history.query。Only current episodes; time filters never turn this into history."
		result = "Current authorized event page: resource_ids, items with lifecycle/notification_state, evaluations with policy_id/health/last_evaluated_at, and complete. Zero current events does not imply health or absence of resolved history."
		guidance += " 当前与历史分别查询；恢复、关闭、过去已处理事件必须另发现并Describe history.query，不能在event.query传resolved/closed。Current event.query always excludes resolved/closed episodes, even with from/to. For historical recovery, independently discover and Describe history.query. Reuse state.metric or the stored policy rule expression when checking metric values; do not invent a formula or average unrelated CPU state series."
	} else if kind == "history" && operation == "query" {
		description = "查询指定主机/集群已恢复或关闭的告警历史，包括条件告警和已恢复NoData/Error健康事件 / Query resolved or closed alert episodes for authorized hosts or clusters, including recovered query-health events. Returns immutable firing rule snapshots, handling and lifecycle history; current event.query excludes these records."
		result = "Authorized historical resolved/closed instances with resource/source scope, values, handling and rule snapshots. This is the required query for recovery history; current active events require event.query."
		request = "查询这台主机最近恢复的告警，以及当时的值、处理人和备注 / find the host's recently recovered alerts and handling history"
		guidance = "Use history.query for resolved/closed episodes, scoped by actual resource UUID and explicit time if requested. Current event.query cannot retrieve recovery history. Read event.get for an instance's full handling/noise timeline. For CPU/other metric checks, reuse the stored rule expression rather than guessing an aggregation."
	} else if kind == "policy" && (operation == "list" || operation == "get") {
		description = "查询告警规则配置及最近评估，用于规则存在性与编辑 / Read alert policy and metric rule configuration for existence checks or edits, with latest evaluation health. This is not the current event list; use event facts to answer whether alerts are active, resolved or muted."
		result = "Authorized policy definitions, versions and latest evaluation facts; configured rules are not proof of current event state or event absence."
	}
	if operation == "enable" || operation == "disable" {
		description = verb + noun + "，只更新状态并保留所有原配置 / " + operation + " " + english + "; update status only, preserving all existing configuration fields. Input is the existing id and expected_version; do not supply name or spec. The domain loads the current snapshot and changes only its enabled state."
		request, guidance = verb+"已有"+noun+"，其他配置保持原样，先预览", "Find the existing configuration and current version, Describe this status action, then Invoke with only id and expected_version. Never reconstruct rules, description, scope or notifications for a status-only request. Host confirmation applies the frozen state change once."
	}
	titleNoun := noun
	if kind == "event" {
		titleNoun = "告警"
	} else if kind == "policy" {
		titleNoun = "告警策略"
	}
	title := verb + titleNoun
	switch operation {
	case "list":
		title = titleNoun + "列表"
	case "get":
		title = titleNoun + "详情"
	case "query":
		title = titleNoun + "查询"
		if kind == "event" {
			title = "告警列表"
		}
	case "assign":
		title = "分配告警负责人"
	case "comment":
		title = "添加告警备注"
	}
	titleEnglishNoun := english
	switch kind {
	case "event":
		titleEnglishNoun = "alert"
	case "policy":
		titleEnglishNoun = "alert policy"
	case "silence":
		titleEnglishNoun = "temporary alert silence"
	}
	englishVerb := map[string]string{"create": "Create", "update": "Update", "delete": "Delete", "enable": "Enable", "disable": "Disable", "list": "List", "get": "Get", "query": "Query", "test": "Test", "resend": "Resend", "acknowledge": "Acknowledge", "assign": "Assign owner to", "comment": "Add comment to"}[operation]
	englishTitle := englishVerb + " " + titleEnglishNoun
	if operation == "list" {
		plural := map[string]string{"policy": "alert policies", "event": "alerts", "silence": "alert silences", "inhibition": "alert inhibition rules", "channel": "notification channels", "route": "notification routes", "settings": "alert evaluator accounts", "grouping": "notification grouping settings", "template": "notification message templates"}[kind]
		if plural != "" {
			englishTitle = "List " + plural
		}
	} else if kind == "event" && operation == "query" {
		englishTitle = "List alerts"
	}
	if strings.HasSuffix(id, ".preview") {
		title += "预览"
		englishTitle = "Preview: " + englishTitle
	}
	return mcp.Discovery{SchemaVersion: "argus.tool_discovery/v1", Revision: "plan6-v8", Title: title + " / " + englishTitle,
		Description: description,
		Keywords:    keywords, ResultDescription: result,
		Preconditions: []string{"Current enterprise membership and tool permission", "Explicit resource grants; configured evaluator for policy creation"},
		Examples:      []mcp.DiscoveryExample{{Request: request, Guidance: guidance}}}
}
