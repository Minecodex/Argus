# 编辑预览契约

接口与公开类型由 `api/openapi/components/dashboard.yaml` 和 `paths/dashboard.yaml` 生成；实现统一在 `internal/dashboard`。查询语言、后端和对象授权体系不变。

## 当前图样本

`POST /dashboard-drafts/{id}/sample`：

```json
{
  "expected_version": 12,
  "parameters": {
    "panel_ids": ["cpu"],
    "from": "2026-10-07T00:00:00Z",
    "to": "2026-10-07T01:00:00Z",
    "resource_ids": ["00000000-0000-0000-0000-000000000001"],
    "variables": {},
    "local_values": {}
  }
}
```

`parameters` 可省略；未指定 `panel_ids` 保持整板预览。指定图时验证它、详情定义和实际引用变量的依赖闭包，无关的未完成统计图不阻止当前图运行。不存在/重复图 ID、未知参数和被引用的重名变量拒绝执行。发布预览仍检查整板，不能以单图通过代替发布验证。

配置验证与样本状态分别返回；无数据、暂不可用、部分或取消不表示数据已验证正常。执行冻结绝对时间、来源、有效参数、草稿版本与定义摘要，并生成签名详情上下文。

## 草稿下钻

`POST /dashboard-drafts/{id}/drilldown` 接受以下字段：

```json
{
  "expected_version": 12,
  "context_token": "opaque-signed-context",
  "panel_id": "services",
  "drilldown_id": "open_instances",
  "values": {"service": "api"},
  "expand_authorized_resources": false
}
```

只执行当前编辑者拥有的草稿内已有定义。行选择证明、资源授权、来源、跨服务链路范围、预算和取消与已发布下钻复用。未编辑过的上下文不能跨草稿/用户使用；草稿 token 也不能用于已发布下钻端点。

样式、标题、单位、布局修改保持数据上下文；查询、来源、变量依赖或下钻定义变化后拒绝旧上下文。当前版本不符、归档、撤权及生命周期变化仍校验，不能因样式复用绕过所有权。

## 前端运行与取消

父级草稿工作区持有服务端草稿与本地缓冲，切换画布/编辑页不丢未保存修改。运行前先保存，当前图仅提交它的有效参数。数据条件变化标记待运行并保留旧结果；样式直接使用旧数据。

请求使用 AbortSignal 与代次判断；取消、切换条件或离开后，迟到响应不更新当前预览。下钻同样在关闭后取消请求。Ctrl/⌘+Enter 等同运行当前图。人工发布与 AI 创建仍通过完整服务端验证和一次 PendingAction 确认。

## 平台概览与用量

`GET /platform/overview` 仅接受平台管理员会话；企业会话不能通过该端点读取跨企业统计。契约由 `api/openapi/components/m2.yaml`、`paths/m2.yaml` 和 `generation/platform.yaml` 生成，查询在 `internal/platform/overview.go` 使用同一只读数据库快照执行。

企业数量不依赖目录分页；Sandbox 会话计数采用现有配额占用状态（creating/running/terminating/unknown）；待首次登录计数采用有效的直接企业管理员授权。用量汇总当前 UTC 月及之前 11 个月，返回全部企业的月度 `session_count` 与 `session_seconds`，缺失月份不推算或造数。`usage_from_month` 包含起始月，`usage_to_month` 不含终止月。

概览和用量页共用缓存与图表；样本时间及口径随结果返回。未加载/失败/403 由公共查询反馈呈现，只有成功结果中的零可以显示为 0。API 没有 CPU 消耗字段，不绘制 CPU 曲线或把未知 CPU 显示为 0。
