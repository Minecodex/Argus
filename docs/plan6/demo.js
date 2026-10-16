/* Interactive design prototype. All operations stay in this origin's demo namespace. */
'use strict';
const DATA_KEY = 'argus.plan6.demo.v1';
const PREF_KEY = 'argus.plan6.preferences.v1';
const I = {
  center: ['告警中心', 'Alert center'], prototype: ['交互原型', 'Prototype'], company: ['星河科技', 'Stellar Technologies'],
  observability: ['可观测性', 'Observability'], resources: ['资源管理', 'RESOURCES'], execution: ['执行与治理', 'OPERATIONS'],
  dashboard: ['仪表盘', 'Dashboards'], hosts: ['主机', 'Hosts'], clusters: ['Kubernetes 集群', 'Kubernetes'], chat: ['会话工作台', 'Conversations'],
  tasks: ['执行任务', 'Tasks'], approvals: ['审批', 'Approvals'], settings: ['企业设置', 'Enterprise settings'],
  hero: ['统一发现异常、管理规则与跟进处置，让每一次告警都有迹可循。', 'Detect anomalies, manage rules, and follow every alert through to resolution.'],
  active: ['当前告警', 'Active alerts'], history: ['告警历史', 'Alert history'], deliveries: ['通知记录', 'Deliveries'], policies: ['告警策略', 'Alert policies'],
  noise: ['屏蔽与抑制', 'Silences'], channels: ['通知配置', 'Notifications'], organization: ['组织与权限', 'Organization & Access'],
  admin: ['企业管理员', 'Administrator'], editor: ['运维编辑者', 'Operator'], viewer: ['只读成员', 'Viewer'], demoRole: ['演示角色', 'Demo role'],
  language: ['切换语言', 'Switch language'], theme: ['切换配色', 'Switch theme'], light: ['浅色', 'Light'], dark: ['深色', 'Dark'],
  create: ['创建告警策略', 'Create policy'], ask: ['让 AI 帮我配置', 'Ask AI to configure'], reset: ['重置演示', 'Reset demo'], coverage: ['功能覆盖', 'Feature coverage'],
  local: ['本地演示数据 · 示例快照 2026-10-07 · 点击可体验完整交互', 'Local demo data · Sample snapshot: Oct 7, 2026 · Explore the interactions'],
  activeCount: ['活跃告警', 'Active alerts'], unhandled: ['待确认', 'Unacknowledged'], policyCount: ['告警策略', 'Policies'], failedCount: ['通知失败', 'Failed deliveries'],
  includesQuiet: ['包含被屏蔽与抑制的事件', 'Includes muted and inhibited events'], handlingHint: ['确认与指标恢复独立记录', 'Acknowledgment is tracked separately'],
  multiHint: ['支持多个指标与组合条件', 'Multiple metrics and conditions'], failedHint: ['失败原因与尝试记录可追溯', 'Failure reasons and attempts are retained'],
  search: ['搜索策略、指标或资源…', 'Search policies, metrics, or resources…'], all: ['全部', 'All'], allLevels: ['全部等级', 'All severities'],
  critical: ['严重', 'Critical'], warning: ['警告', 'Warning'], info: ['提示', 'Info'], allHandling: ['全部处理状态', 'All handling states'],
  acknowledged: ['已确认', 'Acknowledged'], unacknowledged: ['未确认', 'Unacknowledged'], group: ['合并视图', 'Grouped view'], flat: ['展开列表', 'Flat view'],
  bulkAck: ['批量确认', 'Acknowledge selected'], acknowledge: ['确认告警', 'Acknowledge'], ackShort: ['确认', 'Ack'], details: ['详情', 'Details'],
  event: ['告警事件', 'Alert event'], resource: ['资源', 'Resource'], currentValue: ['当前值 / 阈值', 'Value / threshold'], duration: ['持续时间', 'Duration'],
  notification: ['通知状态', 'Delivery state'], owner: ['负责人', 'Owner'], action: ['操作', 'Actions'], notAssigned: ['未认领', 'Unassigned'],
  received: ['已受理', 'Accepted'], failed: ['发送失败', 'Failed'], muted: ['已屏蔽', 'Muted'], inhibited: ['已抑制', 'Inhibited'], unknown: ['结果未知', 'Unknown'],
  pending: ['待触发', 'Pending'], firing: ['活跃', 'Firing'], resolved: ['已恢复', 'Resolved'], noData: ['无数据', 'No data'], error: ['评估失败', 'Evaluation error'],
  normal: ['评估正常', 'Healthy'], notEvaluated: ['尚未评估', 'Not evaluated'], enabled: ['已启用', 'Enabled'], disabled: ['已停用', 'Disabled'],
  pageInfo: ['共 {total} 条 · 第 {page}/{pages} 页', '{total} results · Page {page} of {pages}'], previous: ['上一页', 'Previous'], next: ['下一页', 'Next'],
  empty: ['没有匹配的记录', 'No matching records'], emptyHelp: ['调整筛选条件，或创建一条告警策略。', 'Adjust your filters or create an alert policy.'],
  export: ['导出记录', 'Export records'], allLifecycle: ['全部生命周期', 'All lifecycles'], triggered: ['触发时间', 'Triggered at'], recovered: ['恢复时间', 'Resolved at'],
  lifetime: ['生命周期', 'Lifecycle'], never: ['—', '—'], attempts: ['尝试次数', 'Attempts'], channel: ['通知渠道', 'Channel'], result: ['结果 / 原因', 'Result / reason'],
  sentAt: ['最近尝试', 'Last attempt'], resend: ['重新发送', 'Resend'], test: ['测试通知', 'Test notification'], edit: ['编辑', 'Edit'],
  copy: ['复制策略', 'Duplicate policy'], remove: ['删除', 'Delete'], pause: ['停用', 'Disable'], resume: ['启用', 'Enable'], ruleCount: ['指标规则', 'Metric rules'],
  combination: ['触发模式', 'Condition mode'], independent: ['各规则独立触发', 'Independent rules'], any: ['任一满足 · OR', 'Any condition · OR'], every: ['全部满足 · AND', 'All conditions · AND'],
  scope: ['生效范围', 'Resource scope'], allHosts: ['全部已授权资源', 'All authorized resources'], health: ['评估健康', 'Evaluation health'], lastEval: ['最近评估', 'Last evaluation'],
  manageRules: ['查看规则', 'View rules'], silence: ['屏蔽告警', 'Silence alert'], silences: ['临时屏蔽', 'Temporary silences'], inhibitions: ['抑制规则', 'Inhibition rules'],
  createSilence: ['创建屏蔽', 'Create silence'], createInhibition: ['创建抑制规则', 'Create inhibition'], noiseHint: ['屏蔽和抑制暂停通知；指标继续评估，告警事件与命中原因保留。', 'Silences and inhibitions pause notifications. Evaluation and event history continue.'],
  reason: ['原因', 'Reason'], until: ['结束时间', 'Ends at'], match: ['匹配条件', 'Matchers'], matches: ['命中事件', 'Matched events'], status: ['状态', 'Status'],
  revoke: ['解除屏蔽', 'End silence'], source: ['源告警', 'Source alert'], target: ['被抑制告警', 'Target alerts'], equal: ['相同维度', 'Equal labels'],
  contactPoints: ['通知渠道', 'Contact points'], routes: ['路由与合并', 'Routing & grouping'], templates: ['消息模板', 'Message templates'], createChannel: ['添加渠道', 'Add channel'],
  routeHint: ['按照等级和标签选择多个接收目标；每个通知都可回溯到原始事件。', 'Route by severity and labels to multiple destinations; retain the source event for each delivery.'],
  demoChannel: ['示例渠道', 'Demo contact point'], credential: ['凭据引用', 'Credential reference'], endpoint: ['接收地址', 'Destination'], routeCount: ['关联路由', 'Routes'],
  configure: ['配置', 'Configure'], createRoute: ['添加通知路由', 'Add route'], destinations: ['接收目标', 'Destinations'], template: ['消息模板', 'Message template'],
  grouping: ['通知合并设置', 'Notification grouping'], groupBy: ['合并维度', 'Group by'], groupWait: ['首次等待（秒）', 'Group wait (seconds)'],
  groupInterval: ['组变化间隔（秒）', 'Group interval (seconds)'], repeatInterval: ['重复提醒（秒）', 'Repeat interval (seconds)'],
  timing: ['通知时机', 'Notification timing'], notifyFirst: ['首次触发', 'First firing'], notifyRepeat: ['持续异常', 'Ongoing firing'], notifyRecovery: ['恢复', 'Recovery'],
  notifySeverity: ['等级变化', 'Severity change'], save: ['保存', 'Save'], cancel: ['取消', 'Cancel'], close: ['关闭', 'Close'], preview: ['预览变更', 'Preview changes'],
  confirm: ['确认提交', 'Confirm changes'], confirmed: ['已提交', 'Committed'], waiting: ['等待用户确认', 'Awaiting your confirmation'], canceled: ['已取消', 'Canceled'],
  pendingHint: ['请核对影响范围与变更内容。确认后，按当前预览一次提交。', 'Review the scope and changes. One confirmation applies the current preview.'],
  localAction: ['本次操作仅更新本地演示数据。', 'This action updates local demo data only.'], simulatedSend: ['模拟投递已记录', 'Simulated delivery recorded'],
  committed: ['变更已提交，审计记录已更新', 'Changes committed; audit updated'], canceledHint: ['预览已取消，配置未改变', 'Preview canceled; configuration unchanged'],
  forbidden: ['当前演示角色没有此操作权限', 'The current demo role cannot perform this action'], expired: ['预览已过期，请重新生成', 'Preview expired; generate a new preview'],
  conflict: ['配置版本已变化，请重新预览', 'Configuration changed; generate a new preview'], required: ['请填写必填项，并检查数值和范围', 'Complete required fields and check values and scope'],
  name: ['名称', 'Name'], description: ['说明', 'Description'], fromDate: ['开始日期', 'From date'], toDate: ['结束日期', 'To date'], allMetrics: ['全部指标', 'All metrics'], minutes: ['分钟', 'minutes'], evaluationInterval: ['检查频率（秒）', 'Evaluation interval (seconds)'],
  forDuration: ['触发持续时间（分钟）', 'Pending duration (minutes)'], recoveryDuration: ['恢复持续时间（分钟）', 'Recovery duration (minutes)'],
  addRule: ['添加指标规则', 'Add metric rule'], metric: ['指标 / PromQL', 'Metric / PromQL'], operator: ['比较', 'Operator'], threshold: ['阈值', 'Threshold'], severity: ['等级', 'Severity'],
  recoveryThreshold: ['恢复阈值', 'Recovery threshold'], previewQuery: ['试算规则', 'Test conditions'], trial: ['示例试算：CPU 94.7%，内存 89.2%；正式版本通过实际查询评估。', 'Sample test: CPU 94.7%, memory 89.2%. The production version uses real query results.'],
  ruleLabel: ['规则 {n}', 'Rule {n}'], modeHelp: ['组合条件只匹配同一资源、同一来源及相同维度。', 'Combined conditions match the same resource, source, and dimensions.'],
  noDataBehavior: ['无数据处理', 'No-data behavior'], errorBehavior: ['评估失败处理', 'Error behavior'], report: ['记录独立异常状态', 'Record a separate health state'],
  keep: ['保持上一状态并记录原因', 'Keep last state and record reason'], activate: ['确认后启用', 'Enable after confirmation'], durationHours: ['屏蔽时长（小时）', 'Silence duration (hours)'],
  policy: ['告警策略', 'Alert policy'], allPolicies: ['全部策略', 'All policies'], eventScope: ['影响当前及未来匹配事件', 'Applies to current and future matching events'],
  overview: ['概览', 'Overview'], timeline: ['时间线', 'Timeline'], trend: ['指标趋势', 'Metric trend'], sampleTrend: ['示例观测曲线', 'Sample observations'],
  policyRevision: ['规则版本', 'Policy revision'], recoveryCondition: ['恢复条件', 'Recovery condition'], claim: ['认领 / 指派', 'Assign owner'],
  comment: ['处理备注', 'Comment'], commentPlaceholder: ['补充排查进展或处理说明…', 'Add investigation notes or progress…'], addComment: ['添加备注', 'Add comment'],
  acknowledgedToast: ['已确认，指标状态继续独立评估', 'Acknowledged; metric evaluation continues independently'], ownerToast: ['负责人已更新', 'Owner updated'], commentToast: ['备注已加入时间线', 'Comment added to timeline'],
  historyExported: ['已导出当前筛选范围的记录', 'Exported records in the current filter scope'], versionSnapshot: ['事件保留触发时的规则与阈值快照', 'The event retains its original rule and threshold snapshot'],
  queryTimeout: ['查询超时，不能据此判断指标正常', 'Query timed out; metric health is unknown'], signatureError: ['机器人签名校验失败', 'Bot signature validation failed'],
  mutedReason: ['维护静默：数据库磁盘扩容', 'Maintenance silence: database volume expansion'], inhibitedReason: ['同资源严重告警正在触发', 'Critical alert firing on the same resource'],
  acceptedReason: ['渠道已受理', 'Accepted by contact point'], recoveredReason: ['恢复通知已受理', 'Recovery notification accepted'],
  chatTitle: ['生产环境巡检', 'Production environment review'], chatHint: ['输入自然语言，模型按当前权限查询资源、指标和告警，并为变更生成确认预览。', 'Describe your request. The model queries resources, metrics, and alerts, and previews changes for confirmation.'],
  assistantName: ['Argus 助手', 'Argus assistant'], userName: ['陈默', 'Chen Mo'], welcome: ['可以帮你创建多指标告警、查找规则、查询告警，或为指定资源设置临时屏蔽。变更会先展示预览，等待你确认。', 'I can create multi-metric alerts, find rules, query alerts, or silence a resource temporarily. I will preview changes for your confirmation.'],
  aiCreate: ['创建 CPU + 内存告警', 'Create CPU + memory alert'], aiQuery: ['查询当前活跃告警', 'Query active alerts'], aiExists: ['检查规则是否存在', 'Check whether a rule exists'],
  aiSilence: ['屏蔽 web-prod-01 一小时', 'Silence web-prod-01 for one hour'], aiModify: ['修改 CPU 阈值为 90%', 'Update CPU threshold to 90%'], aiPause: ['停用 CPU 告警策略', 'Disable CPU alert policy'], aiResume: ['启用 CPU 告警策略', 'Enable CPU alert policy'], aiUnsilence: ['解除临时屏蔽', 'End temporary silence'], prompt: ['描述你想查询或配置的告警…', 'Describe the alert you want to query or configure…'], send: ['发送', 'Send'],
  readOnly: ['只读查询结果', 'Read-only result'], metricLookup: ['指标与规则关联', 'Metric-to-policy links'], queryResults: ['查询结果', 'Query results'],
  aiPrepared: ['已生成配置预览，请核对指标、范围和通知目标。确认后再生效。', 'The preview is ready. Review metrics, scope, and destinations, then confirm to apply.'],
  aiRead: ['已查询当前演示范围：{n} 个活跃告警，{quiet} 个通知受限事件。请同时查看各规则的评估健康。', 'In the demo scope: {n} active alerts and {quiet} notification-restricted events. Check evaluation health alongside the results.'],
  aiFound: ['找到 {n} 条相关策略。规则存在性、告警状态和最近评估时间分别列出。', 'Found {n} related policies. Rule existence, alert state, and last evaluation are shown separately.'],
  aiNotFound: ['当前范围未找到匹配策略。可调整名称或指标后再次查询。', 'No matching policy was found in this scope. Try a different name or metric.'], aiChoosePolicy: ['找到多条相关策略，请在消息中指定策略名称后再修改。', 'Multiple policies match. Specify a policy name before changing it.'],
  aiCommitted: ['已按确认内容提交，配置和审计已更新。可以继续只读查询告警状态。', 'Committed the confirmed configuration and updated audit. You can now query alert state.'],
  aiGuide: ['可使用上方示例创建、查询或屏蔽；输入“规则 + 名称”查询存在性。此原型展示固定工具交互。', 'Use a sample to create, query, or silence. Enter “rule + name” to check existence. This prototype demonstrates deterministic tool interactions.'],
  noSideEffects: ['只读查询无需确认；变更由用户确认后提交。', 'Read-only queries run directly; changes require your confirmation.'],
  permissions: ['功能权限', 'Permissions'], audit: ['操作审计', 'Audit log'], permissionHint: ['复用企业角色与显式资源授权；标签只筛选，不授予访问权限。', 'Reuse enterprise roles and explicit resource grants. Labels filter resources; they do not grant access.'],
  permission: ['功能能力', 'Capability'], readPermission: ['查看告警、历史与只读查询', 'Read alerts, history, and queries'], policyPermission: ['管理告警策略与规则', 'Manage policies and rules'],
  silencePermission: ['管理屏蔽与抑制', 'Manage silences and inhibitions'], channelPermission: ['管理渠道、路由与投递', 'Manage channels, routes, and deliveries'],
  handlePermission: ['确认、认领与添加备注', 'Acknowledge, assign, and comment'], auditPermission: ['查看授权范围内的审计', 'Read audit in the authorized scope'], iamPermission: ['维护角色权限', 'Manage role permissions'],
  resourceGrants: ['显式资源授权', 'Explicit resource grants'], grantsHint: ['演示范围：3 台主机、1 个集群；后台评估账号使用相同明确范围。', 'Demo scope: 3 hosts and 1 cluster. The evaluation account has the same explicit scope.'],
  auditTime: ['时间', 'Time'], actor: ['操作者', 'Actor'], origin: ['来源', 'Origin'], object: ['对象', 'Object'], outcome: ['结果', 'Outcome'],
  manual: ['页面操作', 'Portal'], model: ['会话工具', 'Conversation tool'], system: ['后台评估', 'Evaluator'], success: ['成功', 'Success'],
  operation: ['操作', 'Operation'], editPermissions: ['调整权限', 'Edit permissions'], changeRole: ['权限已调整，请切换演示角色验证', 'Permissions updated; switch demo roles to verify'],
  notifyTestHint: ['使用示例载荷验证交互；此原型只记录模拟结果，不向机器人或邮箱发信。', 'Use a sample payload to preview the interaction. This prototype records simulated results without sending messages.'],
  simulateResult: ['模拟投递结果', 'Simulated result'], simulateSuccess: ['受理成功', 'Accepted'], simulateFailure: ['限流失败', 'Rate limited'], testTitle: ['测试消息', 'Test message'],
  testContent: ['[测试] web-prod-01 CPU 使用率 94.7%，超过阈值 85%，持续 12 分钟。', '[TEST] web-prod-01 CPU usage 94.7%, threshold 85%, firing for 12 minutes.'],
  provider: ['渠道类型', 'Provider'], smtpHost: ['SMTP 主机', 'SMTP host'], smtpPort: ['SMTP 端口', 'SMTP port'], smtpSecurity: ['传输加密', 'Transport security'], fromAddress: ['发件邮箱', 'Sender email'], recipient: ['接收邮箱', 'Recipient email'], botEndpoint: ['机器人 Webhook', 'Bot webhook'],
  invalidEndpoint: ['请输入合法的 HTTPS 地址或邮件地址', 'Enter a valid HTTPS URL or email address'], ruleSource: ['源规则 / 等级', 'Source rule / severity'],
  ruleTarget: ['目标规则 / 等级', 'Target rule / severity'], previewContent: ['消息预览', 'Message preview'], acknowledgedBy: ['已由 {name} 确认', 'Acknowledged by {name}'],
  covered: ['D01–D08 均已纳入原型', 'D01–D08 are covered by the prototype'], resetHint: ['重置后恢复初始示例数据，主题和语言偏好保留。', 'Restore initial sample data while keeping theme and language preferences.'],
  deleteHint: ['删除配置后，已有告警与审计历史继续保留。', 'Existing event and audit history remains after configuration deletion.'],
  enabledAfter: ['提交后启用', 'Enable on commit'], minuteValue: ['{n} 分钟', '{n} min'], itemsValue: ['{n} 条', '{n} items'], rolesHint: ['角色切换只用于本地演示，正式授权由服务端校验。', 'Role switching is local to this demo. Production authorization is enforced by the server.'],
  healthNote: ['无活跃事件仍须核对评估健康与最近成功时间。', 'Check evaluation health and freshness even when no event is firing.'],
  templateHint: ['支持规则名称、资源、现场值、阈值、等级、持续时间和详情链接。', 'Include policy, resource, value, threshold, severity, duration, and detail link.'],
};
Object.assign(I, {
  workspaceGroup: ['工作台', 'Workspace'],
  workspaceDescription: ['在同一个会话中理解问题、查询数据与提出变更。', 'Understand issues, query data, and propose changes in one conversation.'],
  organizationDescription: ['统一维护企业角色、明确资源范围与操作审计。', 'Manage enterprise roles, explicit resource scope, and audit in one place.'],
  toolRecord: ['工具调用记录', 'Tool activity'],
  toolQuery: ['工具查询', 'Find tools'],
  toolDetails: ['工具详情', 'Read tool details'],
  toolCall: ['工具调用', 'Invoke tool'],
});
const icons = {
  bell: '<path d="M18 8a6 6 0 0 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9"/><path d="M10 21h4"/>',
  grid: '<rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/>',
  server: '<rect x="3" y="3" width="18" height="7" rx="2"/><rect x="3" y="14" width="18" height="7" rx="2"/><path d="M7 6h.01M7 17h.01"/>',
  cluster: '<rect x="3" y="3" width="6" height="6" rx="1"/><rect x="15" y="3" width="6" height="6" rx="1"/><rect x="9" y="15" width="6" height="6" rx="1"/><path d="M6 9v3h12V9M12 12v3"/>',
  bot: '<rect x="3" y="6" width="18" height="15" rx="4"/><path d="M12 2v4M7 12h.01M17 12h.01M8 17h8"/>',
  clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
  shield: '<path d="m12 3 8 3v6c0 5-8 9-8 9s-8-4-8-9V6l8-3Z"/><path d="m8 12 3 3 5-6"/>',
  settings: '<path d="M4 7h16M4 17h16"/><circle cx="9" cy="7" r="3"/><circle cx="15" cy="17" r="3"/>',
  search: '<circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 5 5"/>',
  plus: '<path d="M12 5v14M5 12h14"/>', x: '<path d="m6 6 12 12M18 6 6 18"/>',
  check: '<path d="m5 12 4 4L19 6"/>', chevron: '<path d="m9 5 7 7-7 7"/>',
  sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M2 12h2M20 12h2M5 5l1 1M18 18l1 1M5 19l1-1M18 6l1-1"/>',
  moon: '<path d="M20 15A9 9 0 0 1 9 4a9 9 0 1 0 11 11Z"/>',
  globe: '<circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3c5 5 5 13 0 18-5-5-5-13 0-18Z"/>',
  filter: '<path d="M3 4h18l-7 8v8l-4-2v-6Z"/>', download: '<path d="M12 3v12m-4-4 4 4 4-4M4 17v4h16v-4"/>',
  info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v6M12 7h.01"/>',
  mute: '<path d="M9 5a6 6 0 0 1 9 5c0 4 2 6 3 7H7M10 21h4M3 3l18 18"/>',
  send: '<path d="m22 2-7 20-4-9-9-4 20-7ZM22 2 11 13"/>',
  list: '<path d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01"/>',
  mail: '<rect x="3" y="5" width="18" height="14" rx="2"/><path d="m3 7 9 6 9-6"/>',
};
function read(key, fallback) { try { return JSON.parse(localStorage.getItem(key)) || fallback; } catch { return fallback; } }
let pref = read(PREF_KEY, { locale: 'zh', theme: 'light' });
if (!['zh', 'en'].includes(pref.locale)) pref.locale = 'zh';
if (!['light', 'dark'].includes(pref.theme)) pref.theme = 'light';
const L = value => Array.isArray(value) ? value[pref.locale === 'zh' ? 0 : 1] : String(value ?? '');
function t(key, vars = {}) { let value = L(I[key] || key); for (const [name, replacement] of Object.entries(vars)) value = value.replaceAll(`{${name}}`, String(replacement)); return value; }
const esc = value => String(value ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
const icon = name => `<svg class='argus-alert-icon' viewBox='0 0 24 24' aria-hidden='true'>${icons[name] || icons.bell}</svg>`;
const people = { none: ['未认领', 'Unassigned'], chen: ['陈默', 'Chen Mo'], lin: ['林晓', 'Lin Xiao'], han: ['韩澈', 'Han Che'] };
const metrics = { cpu: 'host_cpu_usage_percent', memory: 'host_memory_usage_percent', disk: 'host_disk_usage_percent', errors: 'http_error_ratio_percent', up: 'collector_up' };
const providerLabels = { dingtalk: ['钉钉机器人', 'DingTalk bot'], wecom: ['企业微信机器人', 'WeCom bot'], feishu: ['飞书机器人', 'Feishu bot'], email: ['邮件 · SMTP', 'Email · SMTP'] };
const resourceOptions = [['all', 'allHosts'], ['web-prod-01', 'web-prod-01'], ['web-prod-02', 'web-prod-02'], ['db-prod-01', 'db-prod-01'], ['k8s-production', 'k8s-production']];
function initialData() {
  const policies = [
    { id: 'p1', name: ['主机高负载', 'Host resource pressure'], resource: 'all', mode: 'independent', interval: 30, duration: 5, recovery: 3, enabled: true, revision: 3, health: 'normal', noData: 'report', error: 'keep', channels: ['c3', 'c4'], lastEval: '10:32:00', rules: [{ metric: metrics.cpu, op: '>', threshold: 85, recovery: 75, severity: 'critical' }, { metric: metrics.memory, op: '>', threshold: 80, recovery: 70, severity: 'warning' }] },
    { id: 'p2', name: ['磁盘容量不足', 'Disk capacity low'], resource: 'db-prod-01', mode: 'independent', interval: 60, duration: 3, recovery: 3, enabled: true, revision: 2, health: 'normal', noData: 'report', error: 'keep', channels: ['c2'], lastEval: '10:32:00', rules: [{ metric: metrics.disk, op: '>', threshold: 90, recovery: 85, severity: 'critical' }] },
    { id: 'p3', name: ['API 错误率过高', 'High API error rate'], resource: 'k8s-production', mode: 'any', interval: 30, duration: 2, recovery: 2, enabled: true, revision: 1, health: 'normal', noData: 'report', error: 'keep', channels: ['c1', 'c3'], lastEval: '10:32:00', rules: [{ metric: metrics.errors, op: '>', threshold: 3, recovery: 1, severity: 'critical' }] },
    { id: 'p4', name: ['采集器心跳异常', 'Collector heartbeat'], resource: 'web-prod-02', mode: 'independent', interval: 30, duration: 2, recovery: 2, enabled: true, revision: 1, health: 'error', noData: 'report', error: 'keep', channels: ['c1'], lastEval: '10:29:00', rules: [{ metric: metrics.up, op: '<', threshold: 1, recovery: 1, severity: 'warning' }] },
  ];
  const makeEvent = (id, name, policy, resource, service, metric, value, severity, notification, owner, ack, duration, status = 'firing') => {
    const anchor = status === 'resolved' ? Date.UTC(2026, 9, 7, 1, 12) : Date.UTC(2026, 9, 7, 2, 32);
    const startTime = new Intl.DateTimeFormat('sv-SE', { timeZone: 'Asia/Shanghai', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false }).format(new Date(anchor - duration * 60000));
    return { id, name, policy, resource, service, metric, value, threshold: metric === metrics.memory ? 80 : metric === metrics.disk ? 90 : metric === metrics.errors ? 3 : metric === metrics.up ? 1 : 85, op: metric === metrics.up ? '<' : '>', severity, notification, baseNotification: notification === 'muted' || notification === 'inhibited' ? 'received' : notification, owner, ack, duration, status, started: `2026-10-07 ${startTime}`, resolvedAt: status === 'resolved' ? '2026-10-07 09:12:00' : null, revision: policy === 'p1' ? 3 : 1, timeline: [{ time: startTime, text: ['规则持续命中，生成告警事件', 'Condition sustained; alert event created'], actor: 'system' }] };
  };
  return {
    version: 1, seq: 20, policies,
    events: [
      makeEvent('e1', ['CPU 使用率持续过高', 'Sustained high CPU usage'], 'p1', 'web-prod-01', 'checkout', metrics.cpu, 94.7, 'critical', 'received', 'none', false, 12),
      makeEvent('e2', ['内存使用率超过阈值', 'Memory usage above threshold'], 'p1', 'web-prod-01', 'checkout', metrics.memory, 89.2, 'warning', 'inhibited', 'han', true, 10),
      makeEvent('e3', ['数据盘剩余空间不足', 'Database volume running low'], 'p2', 'db-prod-01', 'database', metrics.disk, 96.1, 'critical', 'muted', 'lin', true, 42),
      makeEvent('e4', ['支付接口错误率升高', 'Payment API error rate high'], 'p3', 'k8s-production', 'payment', metrics.errors, 4.8, 'critical', 'failed', 'none', false, 7),
      makeEvent('e5', ['CPU 使用率超过阈值', 'CPU usage above threshold'], 'p1', 'web-prod-02', 'checkout', metrics.cpu, 88.3, 'critical', 'received', 'none', false, 5),
      makeEvent('e6', ['采集器查询暂不可用', 'Collector query unavailable'], 'p4', 'web-prod-02', 'collector', metrics.up, null, 'warning', 'unknown', 'none', false, 3, 'pending'),
      makeEvent('e7', ['数据盘容量告警已恢复', 'Volume capacity recovered'], 'p2', 'db-prod-01', 'database', metrics.disk, 76.4, 'warning', 'received', 'lin', true, 52, 'resolved'),
      makeEvent('e8', ['主机 CPU 使用率已恢复', 'Host CPU usage recovered'], 'p1', 'web-prod-02', 'checkout', metrics.cpu, 64.2, 'warning', 'received', 'han', true, 21, 'resolved'),
    ],
    silences: [{ id: 's1', name: ['数据库磁盘扩容', 'Database volume expansion'], resource: 'db-prod-01', policy: 'p2', reason: ['维护窗口内扩容数据盘', 'Volume expansion during maintenance'], until: '2026-10-08T02:00:00+08:00', enabled: true, revision: 1 }],
    inhibitions: [{ id: 'i1', name: ['严重告警抑制警告', 'Critical suppresses warning'], source: 'critical', target: 'warning', equal: 'resource_id, policy_id', reason: ['同资源同策略只通知最高等级', 'Notify the highest severity for the same resource and policy'], enabled: true, revision: 1 }],
    channels: [
      { id: 'c1', name: ['运维告警群', 'Operations alerts'], provider: 'dingtalk', endpoint: 'https://example.invalid/dingtalk/operations', credential: 'demo-secret-01', enabled: true, revision: 1 },
      { id: 'c2', name: ['数据库值守群', 'Database operations'], provider: 'wecom', endpoint: 'https://example.invalid/wecom/database', credential: 'demo-secret-02', enabled: true, revision: 1 },
      { id: 'c3', name: ['生产事件群', 'Production incidents'], provider: 'feishu', endpoint: 'https://example.invalid/feishu/production', credential: 'demo-secret-03', enabled: true, revision: 1 },
      { id: 'c4', name: ['基础设施邮件', 'Infrastructure email'], provider: 'email', endpoint: 'operations@example.invalid', smtp: 'smtp.example.invalid', credential: 'demo-secret-04', enabled: true, revision: 1 },
    ],
    routes: [{ id: 'r1', name: ['严重告警通知', 'Critical notifications'], severity: 'critical', channels: ['c3', 'c4'], revision: 1 }, { id: 'r2', name: ['常规运维通知', 'Operations notifications'], severity: 'warning', channels: ['c1', 'c2'], revision: 1 }],
    grouping: { by: 'service', wait: 30, interval: 300, repeat: 3600, timing: ['first', 'repeat', 'recovery', 'severity'], revision: 1 },
    template: '[{{severity}}] {{policy}}\n{{resource}} · {{metric}} = {{value}}\n{{threshold}} · {{duration}}\n{{detail_url}}',
    deliveries: [
      { id: 'd1', event: 'e1', channel: 'c3', status: 'received', attempts: 1, time: '10:20:30', reason: 'acceptedReason' },
      { id: 'd2', event: 'e1', channel: 'c4', status: 'received', attempts: 1, time: '10:20:31', reason: 'acceptedReason' },
      { id: 'd3', event: 'e4', channel: 'c1', status: 'failed', attempts: 3, time: '10:27:30', reason: 'signatureError' },
      { id: 'd4', event: 'e3', channel: 'c2', status: 'muted', attempts: 0, time: '10:22:00', reason: 'mutedReason' },
      { id: 'd5', event: 'e2', channel: 'c2', status: 'inhibited', attempts: 0, time: '10:22:00', reason: 'inhibitedReason' },
      { id: 'd6', event: 'e5', channel: 'c1', status: 'received', attempts: 1, time: '10:27:30', reason: 'acceptedReason' },
      { id: 'd7', event: 'e7', channel: 'c4', status: 'received', attempts: 1, time: '09:12:30', reason: 'recoveredReason' },
    ],
    permissions: { admin: ['read', 'policy', 'silence', 'channel', 'handle', 'audit', 'iam'], editor: ['read', 'policy', 'silence', 'handle', 'audit'], viewer: ['read'] },
    audit: [{ id: 'a1', time: '10:24:12', action: ['确认磁盘告警并认领', 'Acknowledged and assigned disk alert'], object: 'db-prod-01', actor: 'lin', origin: 'manual' }, { id: 'a2', time: '10:18:02', action: ['创建数据库临时屏蔽', 'Created temporary database silence'], object: 's1', actor: 'chen', origin: 'manual' }, { id: 'a3', time: '09:40:22', action: ['通过会话创建双指标策略', 'Created two-metric policy from conversation'], object: 'p1 · revision 3', actor: 'chen', origin: 'model' }],
    completed: [],
  };
}
let data = read(DATA_KEY, null);
if (!data || data.version !== 1 || !Array.isArray(data.events) || !Array.isArray(data.policies)) data = initialData();
const tabNames = ['active', 'history', 'deliveries', 'policies', 'noise', 'channels'];
function routeForHash(hash) {
  if (['workspace', 'ai'].includes(hash)) return { page: 'workspace' };
  if (['organization', 'security'].includes(hash)) return { page: 'organization' };
  return { page: 'alerts', tab: tabNames.includes(hash) ? hash : 'active' };
}
const ui = { tab: 'active', ...routeForHash(location.hash.slice(1)), role: 'admin', filters: {}, selected: new Set(), group: false, noise: 'silences', channels: 'contactPoints', security: 'permissions', drawer: null, detailTab: 'overview', modal: null, pending: null, messages: [], result: null };
const app = document.getElementById('app');
const modal = document.getElementById('modal');
const drawer = document.getElementById('drawer');
const persist = () => { try { localStorage.setItem(DATA_KEY, JSON.stringify(data)); localStorage.setItem(PREF_KEY, JSON.stringify(pref)); } catch { /* In-memory interaction also works when storage is disabled. */ } };
const can = permission => !permission || data.permissions[ui.role]?.includes(permission);
const uid = prefix => `${prefix}${++data.seq}`;
const current = () => data.events.filter(e => e.status !== 'resolved');
const countActive = () => current().filter(e => e.status === 'firing').length;
const eventById = id => data.events.find(e => e.id === id);
const policyById = id => data.policies.find(p => p.id === id);
const channelById = id => data.channels.find(c => c.id === id);
const now = () => new Intl.DateTimeFormat(pref.locale === 'zh' ? 'zh-CN' : 'en-GB', { timeZone: 'Asia/Shanghai', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false }).format(new Date());
const dateText = value => value ? new Intl.DateTimeFormat(pref.locale === 'zh' ? 'zh-CN' : 'en-GB', { timeZone: 'Asia/Shanghai', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }).format(new Date(value)) : '—';
function btn(label, action, options = {}) { return `<button type='button' class='argus-alert-btn ${options.primary ? 'argus-alert-btn--primary' : ''} ${options.quiet ? 'argus-alert-btn--quiet' : ''} ${options.small ? 'argus-alert-btn--small' : ''} ${options.danger ? 'argus-alert-btn--danger' : ''}' data-action='${action}' ${options.id ? `data-id='${esc(options.id)}'` : ''} ${options.extra ? `data-extra='${esc(options.extra)}'` : ''} ${options.disabled || !can(options.perm) ? 'disabled' : ''} ${options.title ? `title='${esc(t(options.title))}'` : ''}>${options.icon ? icon(options.icon) : ''}${esc(t(label))}</button>`; }
function badge(label, tone) { return `<span class='argus-alert-badge ${tone ? `argus-alert-badge--${tone}` : ''}'><span class='argus-alert-dot'></span>${esc(t(label))}</span>`; }
const severityBadge = value => badge(value, value === 'critical' ? 'danger' : value === 'warning' ? 'warning' : 'info');
const deliveryBadge = value => badge(value, ({ received: 'success', failed: 'danger', muted: 'info', inhibited: 'info', unknown: 'warning' })[value]);
const healthBadge = value => badge(value, value === 'normal' ? 'success' : value === 'error' ? 'danger' : 'warning');
function select(name, options, value) { return `<select id='${name}' name='${name}'>${options.map(([key, label]) => `<option value='${esc(key)}' ${key === value ? 'selected' : ''}>${esc(Array.isArray(label) ? L(label) : t(label))}</option>`).join('')}</select>`; }
function input(name, value = '', type = 'text', extra = '') { return `<input id='${name}' name='${name}' type='${type}' value='${esc(value)}' ${extra}>`; }
function field(label, control, help = '', wide = false) { return `<label class='argus-alert-field ${wide ? 'argus-alert-field--wide' : ''}'><span class='argus-alert-field-label'>${esc(t(label))}</span>${control}${help ? `<span class='argus-alert-field-help'>${esc(t(help))}</span>` : ''}</label>`; }
const callout = (key, tone = '') => `<div class='argus-alert-callout ${tone ? `argus-alert-callout--${tone}` : ''}'>${icon('info')}<span>${esc(t(key))}</span></div>`;
function filters() { return ui.filters[ui.page === 'alerts' ? ui.tab : ui.page] ||= { query: '', severity: 'all', status: 'all', handling: 'all', from: '', to: '', page: 1 }; }
function filtered(rows) { const f = filters(); return rows.filter(row => (!f.query || JSON.stringify(row).toLowerCase().includes(f.query.toLowerCase())) && (f.severity === 'all' || row.severity === f.severity) && (f.status === 'all' || row.status === f.status || row.notification === f.status) && (f.handling === 'all' || row.ack === (f.handling === 'acknowledged')) && (!f.from || !row.started || row.started.slice(0, 10) >= f.from) && (!f.to || !row.started || row.started.slice(0, 10) <= f.to)); }
function searchFilter() { return `<div class='argus-alert-search'>${icon('search')}<input id='filter-query' data-filter='query' value='${esc(filters().query)}' placeholder='${esc(t('search'))}' aria-label='${esc(t('search'))}'></div>`; }
function filterSelect(key, labels, label) { return `<label><span class='argus-alert-hidden-label'>${esc(t(label))}</span>${select(`filter-${key}`, [['all', label], ...labels.map(l => [l, l])], filters()[key])}</label>`; }
function toolbar(actions = '', options = {}) { return `<div class='argus-alert-toolbar'><div class='argus-alert-filters'>${searchFilter()}${options.severity ? filterSelect('severity', ['critical', 'warning', 'info'], 'allLevels') : ''}${options.status ? filterSelect('status', options.status, options.statusLabel || 'status') : ''}${options.handling ? filterSelect('handling', ['unacknowledged', 'acknowledged'], 'allHandling') : ''}</div><div class='argus-alert-actions'>${actions}</div></div>`; }
function table(headers, rows, total, page) { const pages = Math.max(1, Math.ceil(total / 5)); return `<div class='argus-alert-surface'><div class='argus-alert-table-wrap'><table class='argus-alert-table'><thead><tr>${headers.map(h => `<th scope='col'>${h.startsWith('<') ? h : esc(t(h))}</th>`).join('')}</tr></thead><tbody>${rows || `<tr><td colspan='${headers.length}'><div class='argus-alert-empty'>${esc(t('empty'))}<p class='argus-alert-secondary'>${esc(t('emptyHelp'))}</p></div></td></tr>`}</tbody></table></div><div class='argus-alert-table-footer'><span>${esc(t('pageInfo', { total, page, pages }))}</span><div class='argus-alert-actions'>${btn('previous', 'page', { extra: '-1', quiet: true, small: true, disabled: page <= 1 })}${btn('next', 'page', { extra: '1', quiet: true, small: true, disabled: page >= pages })}</div></div></div>`; }
function paged(rows, rowRenderer, headers) { const all = filtered(rows); const f = filters(); f.page = Math.max(1, Math.min(f.page, Math.ceil(all.length / 5) || 1)); return table(headers, all.slice((f.page - 1) * 5, f.page * 5).map(rowRenderer).join(''), all.length, f.page); }
function eventRow(e, history = false) {
  const value = e.value === null ? t('noData') : `${e.value}${e.metric === metrics.up ? '' : '%'}`;
  return `<tr>${history ? '' : `<td class='argus-alert-check-cell'><input type='checkbox' data-action='select-event' data-id='${e.id}' aria-label='${esc(L(e.name))}' ${ui.selected.has(e.id) ? 'checked' : ''}></td>`}<td><div class='argus-alert-event-cell'><span class='argus-alert-severity-bar argus-alert-severity-bar--${e.severity}'></span><div><button class='argus-alert-link' data-action='event-detail' data-id='${e.id}'>${esc(L(e.name))}</button><div class='argus-alert-tags'><span class='argus-alert-tag'>${esc(e.metric)}</span><span class='argus-alert-tag'>service=${esc(e.service)}</span></div></div></div></td><td><div>${esc(e.resource)}</div><div class='argus-alert-secondary'>${severityBadge(e.severity)}</div></td><td><div class='argus-alert-mono ${e.status === 'resolved' ? 'argus-alert-success-text' : e.value === null ? '' : 'argus-alert-danger-text'}'>${esc(value)}</div><div class='argus-alert-secondary'>${history ? esc(t(e.status)) : `${esc(e.op || '>')} ${e.threshold}${e.metric === metrics.up ? '' : '%'}`}</div></td><td>${deliveryBadge(e.notification)}<div class='argus-alert-secondary'>${esc(t(e.ack ? 'acknowledged' : 'unacknowledged'))}</div></td><td class='argus-alert-no-wrap'>${esc(t('minuteValue', { n: e.duration }))}<div class='argus-alert-secondary'>${history ? esc(e.resolvedAt?.slice(11, 16) || '—') : esc(e.started.slice(11, 16))}</div></td><td>${esc(L(people[e.owner] || people.none))}</td><td><div class='argus-alert-actions'>${btn('details', 'event-detail', { id: e.id, quiet: true, small: true })}${history ? '' : btn('ackShort', 'ack', { id: e.id, quiet: true, small: true, perm: 'handle', disabled: e.ack })}</div></td></tr>`;
}
function renderActive() {
  const headers = [`<input type='checkbox' data-action='select-all' aria-label='${esc(t('bulkAck'))}'>`, 'event', 'resource', 'currentValue', 'notification', 'duration', 'owner', 'action'];
  let list;
  if (ui.group) { const all = filtered(current()); const keyOf = e => data.grouping.by === 'resource_id' ? e.resource : data.grouping.by === 'policy_id' ? L(policyById(e.policy)?.name || e.policy) : e.service; const groups = [...new Set(all.map(keyOf))]; list = `<div class='argus-alert-surface'>${groups.map(service => `<div class='argus-alert-group-heading'>${icon('cluster')}<b>${esc(service)}</b><span class='argus-alert-count'>${all.filter(e => keyOf(e) === service).length}</span><span class='argus-alert-secondary'>${esc(t('groupBy'))}: ${esc(data.grouping.by)}</span></div><div class='argus-alert-table-wrap'><table class='argus-alert-table'><tbody>${all.filter(e => keyOf(e) === service).map(e => eventRow(e)).join('')}</tbody></table></div>`).join('') || `<div class='argus-alert-empty'>${esc(t('empty'))}</div>`}</div>`; }
  else list = paged(current(), e => eventRow(e), headers);
  return toolbar(`${btn(ui.group ? 'flat' : 'group', 'group', { icon: 'list' })}${btn('bulkAck', 'bulk-ack', { perm: 'handle', disabled: !ui.selected.size, icon: 'check' })}`, { severity: true, handling: true }) + list;
}
function renderHistory() { return toolbar(btn('export', 'export-history', { icon: 'download', perm: 'read' }), { severity: true, status: ['firing', 'pending', 'resolved'], statusLabel: 'allLifecycle' }) + `<div class='argus-alert-history-dates'>${field('fromDate', input('filter-from', filters().from, 'date'))}${field('toDate', input('filter-to', filters().to, 'date'))}</div>` + paged(data.events, e => eventRow(e, true), ['event', 'resource', 'currentValue', 'notification', 'duration', 'owner', 'action']); }
function deliveryRow(d) { const event = eventById(d.event); const channel = channelById(d.channel); return `<tr><td><button class='argus-alert-link' data-action='delivery-detail' data-id='${d.id}'>${esc(event ? L(event.name) : t('testTitle'))}</button><div class='argus-alert-secondary'>${esc(event?.resource || 'demo')}</div></td><td>${esc(channel ? L(channel.name) : d.channel)}<div class='argus-alert-secondary'>${esc(channel ? L(providerLabels[channel.provider]) : '')}</div></td><td>${deliveryBadge(d.status)}<div class='argus-alert-secondary'>${esc(t(d.reason))}</div></td><td class='argus-alert-mono'>${d.attempts}</td><td class='argus-alert-mono'>${esc(d.time)}</td><td><div class='argus-alert-actions'>${btn('details', 'delivery-detail', { id: d.id, quiet: true, small: true })}${btn('resend', 'resend', { id: d.id, quiet: true, small: true, perm: 'channel', disabled: !['failed', 'unknown'].includes(d.status) })}</div></td></tr>`; }
function renderDeliveries() { return toolbar('', { status: ['received', 'failed', 'muted', 'inhibited', 'unknown'] }) + paged(data.deliveries, deliveryRow, ['event', 'channel', 'result', 'attempts', 'sentAt', 'action']); }
function renderPolicies() { return toolbar(btn('create', 'policy-create', { primary: true, icon: 'plus', perm: 'policy' })) + paged(data.policies, p => `<tr><td><button class='argus-alert-link' data-action='policy-view' data-id='${p.id}'>${esc(L(p.name))}</button><div class='argus-alert-secondary'>${esc(t(p.enabled ? 'enabled' : 'disabled'))} · v${p.revision}</div></td><td><span class='argus-alert-count'>${p.rules.length}</span> ${esc(t('ruleCount'))}<div class='argus-alert-secondary'>${esc(t(p.mode))}</div></td><td>${esc(p.resource === 'all' ? t('allHosts') : p.resource)}</td><td>${healthBadge(p.health)}<div class='argus-alert-secondary'>${esc(p.lastEval || t('notEvaluated'))}</div></td><td><div class='argus-alert-actions'>${btn('edit', 'policy-edit', { id: p.id, small: true, quiet: true, perm: 'policy' })}${btn(p.enabled ? 'pause' : 'resume', 'policy-state', { id: p.id, small: true, quiet: true, perm: 'policy' })}</div></td></tr>`, ['policy', 'ruleCount', 'scope', 'health', 'action']) + `<p class='argus-alert-footer-note'>${icon('info')}${esc(t('healthNote'))}</p>`; }
function subTabs(group, names, active) { return `<div class='argus-alert-subtabs' role='tablist' aria-label='${esc(t(group === 'security' ? 'organization' : ui.tab))}'>${names.map(name => `<button class='argus-alert-subtab' role='tab' aria-selected='${name === active}' tabindex='${name === active ? 0 : -1}' data-action='subtab' data-id='${group}' data-extra='${name}'>${esc(t(name))}</button>`).join('')}</div>`; }
function silenceMatches(s) { return current().filter(e => (s.resource === 'all' || e.resource === s.resource) && (s.policy === 'all' || e.policy === s.policy) && (!s.metric || s.metric === 'all' || e.metric === s.metric)); }
function renderNoise() {
  const heading = subTabs('noise', ['silences', 'inhibitions'], ui.noise) + callout('noiseHint');
  if (ui.noise === 'silences') return heading + toolbar(btn('createSilence', 'silence-create', { primary: true, icon: 'plus', perm: 'silence' })) + paged(data.silences, s => `<tr><td><button class='argus-alert-link' data-action='silence-edit' data-id='${s.id}'>${esc(L(s.name))}</button><div class='argus-alert-secondary'>${esc(L(s.reason))}</div></td><td>${esc(s.resource === 'all' ? t('allHosts') : s.resource)}<div class='argus-alert-secondary'>${esc(s.policy === 'all' ? t('allPolicies') : L(policyById(s.policy)?.name || s.policy))}</div></td><td>${badge(s.enabled ? 'enabled' : 'disabled', s.enabled ? 'info' : '')}</td><td>${esc(dateText(s.until))}</td><td><span class='argus-alert-count'>${s.enabled ? silenceMatches(s).length : 0}</span></td><td>${btn('edit', 'silence-edit', { id: s.id, quiet: true, small: true, perm: 'silence' })}${btn('revoke', 'silence-revoke', { id: s.id, quiet: true, small: true, perm: 'silence', disabled: !s.enabled })}</td></tr>`, ['name', 'scope', 'status', 'until', 'matches', 'action']);
  return heading + toolbar(btn('createInhibition', 'inhibition-create', { primary: true, icon: 'plus', perm: 'silence' })) + paged(data.inhibitions, i => `<tr><td><button class='argus-alert-link' data-action='inhibition-edit' data-id='${i.id}'>${esc(L(i.name))}</button><div class='argus-alert-secondary'>${esc(L(i.reason))}</div></td><td>${severityBadge(i.source)}</td><td>${severityBadge(i.target)}</td><td><code>${esc(i.equal)}</code></td><td>${badge(i.enabled ? 'enabled' : 'disabled', i.enabled ? 'success' : '')}</td><td>${btn('edit', 'inhibition-edit', { id: i.id, quiet: true, small: true, perm: 'silence' })}${btn(i.enabled ? 'pause' : 'resume', 'inhibition-state', { id: i.id, quiet: true, small: true, perm: 'silence' })}</td></tr>`, ['name', 'source', 'target', 'equal', 'status', 'action']);
}
function renderChannels() {
  const heading = subTabs('channels', ['contactPoints', 'routes', 'templates'], ui.channels);
  if (ui.channels === 'contactPoints') return heading + toolbar(btn('createChannel', 'channel-create', { primary: true, icon: 'plus', perm: 'channel' })) + `<div class='argus-alert-grid'>${filtered(data.channels).map(c => `<article class='argus-alert-panel'><div class='argus-alert-panel-head'><div class='argus-alert-channel-head'><span class='argus-alert-channel-icon'>${c.provider === 'email' ? icon('mail') : esc(L(providerLabels[c.provider]).slice(0, 1))}</span><div><h3>${esc(L(c.name))}</h3><p class='argus-alert-secondary'>${esc(L(providerLabels[c.provider]))}</p></div></div>${badge(c.enabled ? 'demoChannel' : 'disabled', c.enabled ? 'info' : '')}</div><div class='argus-alert-code-block'>${esc(c.endpoint)}</div><div class='argus-alert-channel-meta'><span>${esc(t('credential'))} · ${esc(c.credential)}</span><span>${data.routes.filter(r => r.channels.includes(c.id)).length} ${esc(t('routeCount'))}</span></div><div class='argus-alert-actions argus-alert-actions--end'>${btn('configure', 'channel-edit', { id: c.id, perm: 'channel' })}${btn('test', 'channel-test', { id: c.id, perm: 'channel', icon: 'send', disabled: !c.enabled })}</div></article>`).join('') || `<div class='argus-alert-empty'>${esc(t('empty'))}</div>`}</div>`;
  if (ui.channels === 'templates') return heading + `<div class='argus-alert-panel'><div class='argus-alert-panel-head'><div><h2>${esc(t('templates'))}</h2><p class='argus-alert-secondary'>${esc(t('templateHint'))}</p></div>${btn('edit', 'template-edit', { perm: 'channel' })}</div><div class='argus-alert-grid'><div><h3>${esc(t('template'))}</h3><pre class='argus-alert-code-block'>${esc(data.template)}</pre></div><div><h3>${esc(t('previewContent'))}</h3><pre class='argus-alert-code-block'>${esc(renderMessageTemplate())}</pre></div></div></div>`;
  return heading + callout('routeHint') + toolbar(btn('createRoute', 'route-create', { primary: true, icon: 'plus', perm: 'channel' })) + paged(data.routes, r => `<tr><td><button class='argus-alert-link' data-action='route-edit' data-id='${r.id}'>${esc(L(r.name))}</button></td><td>${r.severity === 'all' ? badge('all') : severityBadge(r.severity)}</td><td><div class='argus-alert-tags'>${r.channels.map(id => `<span class='argus-alert-tag'>${esc(L(channelById(id)?.name || id))}</span>`).join('')}</div></td><td>${btn('edit', 'route-edit', { id: r.id, quiet: true, small: true, perm: 'channel' })}</td></tr>`, ['name', 'match', 'destinations', 'action']) + `<div class='argus-alert-panel argus-alert-drawer-section'><div class='argus-alert-panel-head'><h3>${esc(t('grouping'))}</h3>${btn('configure', 'grouping-edit', { perm: 'channel', small: true })}</div><div class='argus-alert-grid'><div>${esc(t('groupBy'))}<div class='argus-alert-secondary'><code>${esc(data.grouping.by)}</code></div></div><div>${esc(t('groupWait'))}: ${data.grouping.wait}<div class='argus-alert-secondary'>${esc(t('groupInterval'))}: ${data.grouping.interval} · ${esc(t('repeatInterval'))}: ${data.grouping.repeat}</div></div></div></div>`;
}
function renderMessageTemplate() { const values = { severity: t('critical'), policy: L(data.policies[0]?.name || 'Host CPU'), resource: 'web-prod-01', metric: metrics.cpu, value: '94.7%', threshold: '> 85%', duration: t('minuteValue', { n: 12 }), detail_url: 'https://argus.example.invalid/alerts/events/e1' }; return data.template.replace(/\{\{(\w+)\}\}/g, (raw, key) => values[key] || raw); }
function renderToolRecord(tools) {
  return `<details class='argus-alert-tool-record'><summary>${icon('settings')}<span>${esc(t('toolRecord'))}</span><span class='argus-alert-count'>${tools.length}</span>${badge('prototype','info')}</summary><ol>${tools.map(tool => `<li><div><b>${esc(t(tool.title))}</b><code>${esc(tool.meta)}</code></div><p class='argus-alert-secondary'>${esc(tool.detail)}</p></li>`).join('')}</ol></details>`;
}
function hubReply(text, toolId) {
  const tools = toolId ? [
    { title: 'toolQuery', meta: 'tool.search', detail: 'Argus Registry · alerting' },
    { title: 'toolDetails', meta: 'tool.describe', detail: toolId },
    { title: 'toolCall', meta: 'tool.invoke', detail: toolId },
  ] : [];
  ui.messages.push({ role: 'assistant', text, tools });
}
function renderWorkbench() {
  const messages = [{ role: 'assistant', text: I.welcome }, ...ui.messages];
  return `<section class='argus-alert-panel argus-alert-workbench'><header class='argus-alert-panel-head'><div><h2>${esc(t('chatTitle'))}</h2><p class='argus-alert-secondary'>${esc(t('chatHint'))}</p></div>${badge('prototype','brand')}</header><div class='argus-alert-chat-log'>${messages.map(m => `<div class='argus-alert-message ${m.role === 'user' ? 'argus-alert-message--user' : ''}'><span class='argus-alert-avatar'>${m.role === 'user' ? 'C' : icon('bot')}</span><div class='argus-alert-message-body'><div class='argus-alert-message-title'>${esc(t(m.role === 'user' ? 'userName' : 'assistantName'))}</div>${m.tools?.length ? renderToolRecord(m.tools) : ''}<div class='argus-alert-message-text'>${esc(L(m.text))}</div></div></div>`).join('')}${ui.result ? renderWorkbenchResults() : ''}${ui.pending?.origin === 'model' ? renderPending(ui.pending) : ''}</div><footer class='argus-alert-composer'><div class='argus-alert-ai-prompts'>${['aiCreate','aiQuery','aiExists','aiSilence'].map(p=>btn(p,'ai-prompt',{id:p,small:true})).join('')}</div><form id='ai-form' class='argus-alert-ai-input'><textarea id='ai-input' name='prompt' aria-label='${esc(t('prompt'))}' placeholder='${esc(t('prompt'))}' required></textarea><button class='argus-alert-btn argus-alert-btn--primary' type='submit'>${icon('send')}${esc(t('send'))}</button></form><p class='argus-alert-secondary'>${esc(t('noSideEffects'))}</p></footer></section>`;
}
function renderWorkbenchResults() {
  const rows = ui.result.rows;
  return `<section class='argus-alert-panel argus-alert-workbench-result'><div class='argus-alert-panel-head'><h3>${esc(t('readOnly'))}</h3>${badge('scope','info')}</div>${callout('healthNote')}${rows.map(r=>ui.result.type==='policies' ? `<div class='argus-alert-result-row'><div><button class='argus-alert-link' data-action='policy-view' data-id='${r.id}'>${esc(L(r.name))}</button><p class='argus-alert-secondary'>${r.rules.length} ${esc(t('ruleCount'))} · ${esc(r.lastEval||t('notEvaluated'))}</p></div>${healthBadge(r.health)}</div>` : `<div class='argus-alert-result-row'><div><button class='argus-alert-link' data-action='event-detail' data-id='${r.id}'>${esc(L(r.name))}</button><p class='argus-alert-secondary'>${esc(r.resource)} · ${esc(t(r.status))} · ${r.value===null ? esc(t('noData')) : esc(r.value+'%')}</p></div>${deliveryBadge(r.notification)}</div>`).join('') || `<p class='argus-alert-empty'>${esc(t('empty'))}</p>`}</section>`;
}
const permissionKeys = ['read', 'policy', 'silence', 'channel', 'handle', 'audit', 'iam'];
const permissionIds = { read: 'alert.read', policy: 'alert.policy.manage', silence: 'alert.silence.manage', channel: 'alert.notification.manage', handle: 'alert.handle', audit: 'audit.read', iam: 'role.manage' };
function renderSecurity() {
  const header = subTabs('security', ['permissions', 'audit'], ui.security);
  if (ui.security === 'audit') return header + (can('audit') ? toolbar() + paged(data.audit, a => `<tr><td class='argus-alert-mono'>${esc(a.time)}</td><td>${esc(L(people[a.actor] || I.userName))}</td><td>${esc(L(a.action))}</td><td><code>${esc(a.object)}</code></td><td>${badge(a.origin, a.origin === 'model' ? 'brand' : '')}</td><td>${badge('success', 'success')}</td></tr>`, ['auditTime', 'actor', 'operation', 'object', 'origin', 'outcome']) : callout('forbidden', 'warm'));
  return header + callout('permissionHint') + `<div class='argus-alert-grid'><div class='argus-alert-panel argus-alert-field--wide'><div class='argus-alert-panel-head'><h2>${esc(t('permissions'))}</h2>${btn('editPermissions', 'permissions-edit', { perm: 'iam' })}</div><div class='argus-alert-table-wrap'><table class='argus-alert-table argus-alert-permission-table'><thead><tr><th>${esc(t('permission'))}</th>${['admin', 'editor', 'viewer'].map(r => `<th>${esc(t(r))}</th>`).join('')}</tr></thead><tbody>${permissionKeys.map(p => `<tr><td><div class='argus-alert-permission-name'>${esc(t(`${p}Permission`))}</div><code>${permissionIds[p]}</code></td>${['admin', 'editor', 'viewer'].map(r => `<td><input type='checkbox' tabindex='-1' aria-label='${esc(`${t(r)} ${t(`${p}Permission`)}`)}' ${data.permissions[r].includes(p) ? 'checked' : ''} disabled></td>`).join('')}</tr>`).join('')}</tbody></table></div></div><div class='argus-alert-panel argus-alert-field--wide'><h3>${esc(t('resourceGrants'))}</h3><p class='argus-alert-secondary'>${esc(t('grantsHint'))}</p><div class='argus-alert-tags'>${resourceOptions.filter(([id]) => id !== 'all').map(([id]) => `<span class='argus-alert-tag'>${esc(id)}</span>`).join('')}</div></div></div>`;
}
function spark(values, tone = 'info') { return `<svg class='argus-alert-spark' viewBox='0 0 80 38' style='color:var(--${tone})' aria-hidden='true'><polyline points='${values.map((v, i) => `${i * 8},${36 - v}`).join(' ')}' fill='none' stroke='currentColor' stroke-width='1.7'/></svg>`; }
function render() {
  const focus = document.activeElement?.id, start = document.activeElement?.selectionStart, end = document.activeElement?.selectionEnd;
  document.documentElement.dataset.theme = pref.theme;
  document.documentElement.lang = pref.locale === 'zh' ? 'zh-CN' : 'en-US';
  const title = ui.page === 'workspace' ? 'chat' : ui.page === 'organization' ? 'organization' : 'center';
  const group = ui.page === 'workspace' ? 'workspaceGroup' : ui.page === 'organization' ? 'settings' : 'observability';
  document.title = `Argus · ${t(title)}`;
  const stats = [
    ['activeCount',countActive(),'includesQuiet',[10,15,12,26,20,30,18,24,26,20],'danger'],
    ['unhandled',current().filter(e=>!e.ack).length,'handlingHint',[24,27,20,18,20,10,12,10,16,14],'warning'],
    ['policyCount',data.policies.length,'multiHint',[8,8,8,13,13,13,23,23,23,23],'info'],
    ['failedCount',data.deliveries.filter(d=>d.status==='failed').length,'failedHint',[3,3,3,16,3,3,3,3,14,8],'warning'],
  ];
  const nav = (key,name,page) => {
    const active = page===ui.page;
    return `<${page?'a':'div'} ${page?`href='#${page==='alerts'?ui.tab:page}' data-action='page-entry' data-id='${page}' ${active?`aria-current='page'`:''}`:''} class='argus-alert-nav ${active?'argus-alert-nav--active':''}'>${icon(name)}<span>${esc(t(key))}</span>${page==='alerts'?`<span class='argus-alert-count'>${can('read')?countActive():'—'}</span>`:''}</${page?'a':'div'}>`;
  };
  const alertsHeader = `<section class='argus-alert-stats' aria-label='${esc(t('overview'))}'>${stats.map(([label,value,caption,values,tone])=>`<div class='argus-alert-stat'><div><div class='argus-alert-stat-label'>${icon(label==='policyCount'?'settings':'bell')}${esc(t(label))}</div><div class='argus-alert-stat-value'>${can('read')?value:'—'}</div><div class='argus-alert-stat-caption'>${esc(t(caption))}</div></div>${spark(values,tone)}</div>`).join('')}</section><nav class='argus-alert-tabs' role='tablist' aria-label='${esc(t('center'))}'>${tabNames.map(tab=>`<button id='tab-${tab}' role='tab' aria-controls='panel-${tab}' aria-selected='${ui.tab===tab}' tabindex='${ui.tab===tab?0:-1}' class='argus-alert-tab' data-action='tab' data-id='${tab}'>${esc(t(tab))}${tab==='active'?`<span class='argus-alert-count'>${can('read')?current().length:'—'}</span>`:tab==='policies'?`<span class='argus-alert-count'>${can('read')?data.policies.length:'—'}</span>`:''}</button>`).join('')}</nav>`;
  const alertsPanel = () => !can('read')?callout('forbidden','warm'):({active:renderActive,history:renderHistory,deliveries:renderDeliveries,policies:renderPolicies,noise:renderNoise,channels:renderChannels})[ui.tab]();
  const content = ui.page==='workspace'?renderWorkbench():ui.page==='organization'?renderSecurity():`<section id='panel-${ui.tab}' role='tabpanel' aria-labelledby='tab-${ui.tab}'>${alertsPanel()}</section>`;
  app.innerHTML = `<aside class='argus-alert-sidebar'><div class='argus-alert-brand'><span class='argus-alert-brand-mark'>◉</span><div class='argus-alert-brand-name'>Argus<small>ENTERPRISE</small></div></div><div class='argus-alert-nav-label'>${esc(t('resources'))}</div>${nav('dashboard','grid')}${nav('hosts','server')}${nav('clusters','cluster')}${nav('center','bell','alerts')}<div class='argus-alert-nav-label'>${esc(t('execution'))}</div>${nav('chat','bot','workspace')}${nav('tasks','clock')}${nav('approvals','shield')}<div class='argus-alert-nav-label'>${esc(t('settings'))}</div>${nav('organization','shield','organization')}<div class='argus-alert-sidebar-bottom'><div class='argus-alert-user'><span class='argus-alert-avatar'>C</span><div><b>${esc(t('userName'))}</b><p class='argus-alert-secondary'>${esc(t(ui.role))}</p></div></div><label class='argus-alert-field argus-alert-form-divider'><span class='argus-alert-field-label'>${esc(t('demoRole'))}</span>${select('demo-role',[['admin','admin'],['editor','editor'],['viewer','viewer']],ui.role)}</label>${btn('reset','reset',{quiet:true,small:true,icon:'clock'})}</div></aside><main class='argus-alert-main'><header class='argus-alert-topbar'><div class='argus-alert-breadcrumb'><span>${esc(t('company'))}</span>${icon('chevron')}<span>${esc(t(group))}</span>${icon('chevron')}<b>${esc(t(title))}</b></div><div class='argus-alert-actions'>${badge('prototype','brand')}${btn(pref.locale==='zh'?'EN':'中文','locale',{quiet:true,small:true,icon:'globe',title:'language'})}${btn(pref.theme==='light'?'dark':'light','theme',{quiet:true,small:true,icon:pref.theme==='light'?'moon':'sun',title:'theme'})}</div></header><div class='argus-alert-page'><div class='argus-alert-hero'><div><div class='argus-alert-eyebrow'>${ui.page==='workspace'?'CONVERSATION / WORKSPACE':ui.page==='organization'?'IDENTITY / GOVERNANCE':'ALERTING / OPERATIONS'}</div><h1>${esc(t(title))}</h1><p class='argus-alert-description'>${esc(t(ui.page==='workspace'?'workspaceDescription':ui.page==='organization'?'organizationDescription':'hero'))}</p></div>${ui.page==='alerts'?`<div class='argus-alert-actions'>${btn('create','policy-create',{primary:true,icon:'plus',perm:'policy'})}</div>`:''}</div>${ui.page==='alerts'?alertsHeader:''}${content}<div class='argus-alert-footer-note'>${icon('info')}<span>${esc(t('local'))}</span><button class='argus-alert-link' data-action='coverage'>${esc(t('coverage'))}</button></div></div></main>`;
  if (focus) { const target=document.getElementById(focus); if(target){target.focus({preventScroll:true});if(typeof target.setSelectionRange==='function'&&start!=null)target.setSelectionRange(start,end);} }
  if (drawer.open && ui.drawer) renderDrawer();
  const chatLog=document.querySelector('.argus-alert-chat-log');
  if(chatLog&&ui.messages.length)chatLog.scrollTop=chatLog.scrollHeight;
  persist();
}

function toast(key) { const target = document.getElementById('toast'); target.textContent = t(key); target.dataset.visible = 'true'; clearTimeout(toast.timer); toast.timer = setTimeout(() => { target.dataset.visible = 'false'; }, 3500); }
function audit(action, object, origin = 'manual') { data.audit.unshift({ id: uid('a'), time: now(), action, object, actor: 'chen', origin }); }
function changeTab(tab) { ui.page = 'alerts'; ui.tab = tab; location.hash = tab; render(); }
function changePage(page) { ui.page = page; location.hash = page === 'alerts' ? ui.tab : page; render(); }
function recordHandling(id, kind, value) {
  if (!can('handle')) return toast('forbidden');
  const e = eventById(id); if (!e) return;
  if (kind === 'ack') { if (e.ack) return; e.ack = true; }
  if (kind === 'owner') e.owner = value;
  if (kind === 'comment' && !String(value).trim()) return;
  const text = kind === 'ack' ? I.acknowledgedToast : kind === 'owner' ? [I.owner[0] + ': ' + people[value][0], I.owner[1] + ': ' + people[value][1]] : [String(value), String(value)];
  e.timeline.unshift({ time: now(), text, actor: 'chen' });
  audit(kind === 'ack' ? ['确认告警', 'Acknowledged alert'] : kind === 'owner' ? ['指派告警负责人', 'Assigned alert owner'] : ['添加处理备注', 'Added handling comment'], e.id);
  render(); toast(kind === 'ack' ? 'acknowledgedToast' : kind === 'owner' ? 'ownerToast' : 'commentToast');
}
function chart(e) {
  const values = [43, 54, 48, 61, 53, 69, 64, 75, 86, 91, 88, 95];
  return `<svg class='argus-alert-chart' viewBox='0 0 420 160' role='img' aria-label='${esc(t('sampleTrend'))}'><path d='M35 20H405M35 60H405M35 100H405M35 140H405' stroke='var(--border-subtle)'/><path d='M35 42H405' stroke='var(--warning)' stroke-dasharray='4 5'/><text x='42' y='35' style='font-size:var(--font-size-11)' fill='var(--warning)'>${esc(t('threshold'))} ${e.threshold}</text><polyline points='${values.map((v, i) => `${35 + i * 33},${140 - v}`).join(' ')}' fill='none' stroke='var(--accent-text)' stroke-width='2'/><text x='35' y='154' style='font-size:var(--font-size-10)' fill='var(--text-tertiary)'>10:00</text><text x='365' y='154' style='font-size:var(--font-size-10)' fill='var(--text-tertiary)'>10:32</text></svg>`;
}
function renderDrawer() {
  const e = eventById(ui.drawer); if (!e) return;
  const p = policyById(e.policy);
  const title = `<header class='argus-alert-drawer-head'><div class='argus-alert-actions'><div class='argus-alert-actions'>${severityBadge(e.severity)}${badge(e.status, e.status === 'resolved' ? 'success' : 'warning')}</div>${btn('close', 'drawer-close', { quiet: true, small: true, icon: 'x' })}</div><h2 id='drawer-title'>${esc(L(e.name))}</h2><p class='argus-alert-secondary'>${esc(e.resource)} · ${esc(e.id)} · ${esc(e.service)}</p></header>`;
  const tabs = subTabs('detailTab', ['overview', 'timeline', 'deliveries'], ui.detailTab);
  const timeline = `<ol class='argus-alert-timeline'>${e.timeline.map(entry => `<li><div>${esc(L(entry.text))}</div><p class='argus-alert-secondary'>${esc(entry.time)} · ${esc(entry.actor === 'system' ? t('system') : L(people[entry.actor] || I.userName))}</p></li>`).join('')}<li><div>${esc(e.status === 'resolved' ? t('resolved') : t('lastEval'))}</div><p class='argus-alert-secondary'>${esc(e.resolvedAt || p?.lastEval || t('notEvaluated'))}</p></li></ol>`;
  let content = '';
  if (ui.detailTab === 'timeline') content = timeline;
  else if (ui.detailTab === 'deliveries') content = data.deliveries.filter(d => d.event === e.id).map(d => `<div class='argus-alert-result-row'><div>${esc(L(channelById(d.channel)?.name || d.channel))}<p class='argus-alert-secondary'>${esc(d.time)} · ${d.attempts} ${esc(t('attempts'))}</p><p class='argus-alert-secondary'>${esc(t(d.reason))}</p></div>${deliveryBadge(d.status)}</div>`).join('') || `<div class='argus-alert-empty'>${esc(t('empty'))}</div>`;
  else content = `${e.value === null ? callout('queryTimeout', 'warm') : ''}<div class='argus-alert-detail-grid'>${[['currentValue', e.value === null ? t('noData') : `${e.value}% / ${e.threshold}%`], ['policy', L(p?.name || e.policy)], ['duration', t('minuteValue', { n: e.duration })], ['triggered', e.started], ['recovered', e.resolvedAt || '—'], ['notification', t(e.notification)], ['policyRevision', `v${e.revision}`], ['health', t(p?.health || 'notEvaluated')]].map(([label, value]) => `<div><span class='argus-alert-secondary'>${esc(t(label))}</span><div class='argus-alert-detail-value'>${esc(value)}</div></div>`).join('')}</div><div class='argus-alert-code-block'>${esc(e.metric)} ${esc(e.op || '>')} ${e.threshold}</div><section class='argus-alert-drawer-section'><h3>${esc(t('trend'))}</h3>${chart(e)}<p class='argus-alert-secondary'>${esc(t('sampleTrend'))} · ${esc(t('versionSnapshot'))}</p></section><section class='argus-alert-drawer-section'><h3>${esc(t('timeline'))}</h3>${timeline}</section>`;
  const handling = `<div class='argus-alert-form-divider'><div class='argus-alert-actions'>${btn('acknowledge', 'ack', { id: e.id, primary: true, icon: 'check', perm: 'handle', disabled: e.ack })}${btn('silence', 'event-silence', { id: e.id, icon: 'mute', perm: 'silence', disabled: e.status === 'resolved' })}</div><div class='argus-alert-grid argus-alert-drawer-section'>${field('owner', select('detail-owner', Object.entries(people), e.owner))}<div class='argus-alert-field'><span class='argus-alert-field-label'>${esc(t('claim'))}</span>${btn('save', 'assign', { id: e.id, perm: 'handle' })}</div></div>${field('comment', `<textarea id='detail-comment' placeholder='${esc(t('commentPlaceholder'))}' ${can('handle') ? '' : 'disabled'}></textarea>`)}<div class='argus-alert-actions argus-alert-actions--end'>${btn('addComment', 'comment', { id: e.id, perm: 'handle' })}</div></div>`;
  drawer.innerHTML = `${title}<div class='argus-alert-drawer-body'>${tabs}${content}${handling}</div>`;
}
function openDrawer(id) { ui.drawer = id; ui.detailTab = 'overview'; renderDrawer(); if (!drawer.open) drawer.showModal(); }
function closeDrawer() { drawer.close(); ui.drawer = null; }
function ruleExpression(p) { return p.rules.map((r, index) => `${String.fromCharCode(65 + index)}: ${r.metric} ${r.op} ${r.threshold}`).join('\n') + `\n${t('combination')}: ${t(p.mode)}`; }
function checkboxes(name, rows, selected) { return `<div class='argus-alert-checks'>${rows.map(([id, label]) => `<label class='argus-alert-checkbox-label'><input name='${name}' type='checkbox' value='${esc(id)}' ${selected.includes(id) ? 'checked' : ''}>${esc(Array.isArray(label) ? L(label) : t(label))}</label>`).join('')}</div>`; }
function policyForm(p) {
  return `<div class='argus-alert-grid'>${field('name', input('policy-name', L(p.name), 'text', 'required maxlength=100'))}${field('scope', select('policy-resource', resourceOptions, p.resource))}${field('combination', select('policy-mode', [['independent', 'independent'], ['any', 'any'], ['every', 'every']], p.mode), 'modeHelp', true)}${field('evaluationInterval', input('policy-interval', p.interval, 'number', 'required min=10 max=3600'))}${field('forDuration', input('policy-duration', p.duration, 'number', 'required min=0 max=1440'))}</div><div class='argus-alert-panel-head argus-alert-form-divider'><h3>${esc(t('ruleCount'))}</h3>${btn('addRule', 'rule-add', { icon: 'plus', small: true })}</div><datalist id='metric-candidates'>${Object.values(metrics).map(m => `<option value='${m}'></option>`).join('')}</datalist>${p.rules.map((r, index) => `<div class='argus-alert-rule'><div class='argus-alert-rule-head'><b>${esc(t('ruleLabel', { n: index + 1 }))}</b>${btn('remove', 'rule-remove', { id: String(index), quiet: true, small: true, disabled: p.rules.length <= 1 })}</div><div class='argus-alert-rule-fields'>${field('metric', input(`rule-metric-${index}`, r.metric, 'text', 'required list=metric-candidates'))}${field('operator', select(`rule-op-${index}`, [['>', '>'], ['<', '<']], r.op))}${field('threshold', input(`rule-threshold-${index}`, r.threshold, 'number', 'required step=any'))}${field('severity', select(`rule-severity-${index}`, [['critical', 'critical'], ['warning', 'warning'], ['info', 'info']], r.severity))}</div><div class='argus-alert-grid argus-alert-drawer-section'>${field('recoveryThreshold', input(`rule-recovery-${index}`, r.recovery, 'number', 'required step=any'))}<div class='argus-alert-field'><span class='argus-alert-field-label'>${esc(t('policyRevision'))}</span><span class='argus-alert-secondary'>${esc(p.id || 'new')} · v${p.revision || 1}</span></div></div></div>`).join('')}<div class='argus-alert-actions'>${btn('previewQuery', 'rule-test', { icon: 'search' })}</div><div id='trial-output'></div><div class='argus-alert-grid argus-alert-form-divider'>${field('recoveryDuration', input('policy-recovery-duration', p.recovery, 'number', 'required min=0 max=1440'))}${field('noDataBehavior', select('policy-nodata', [['report', 'report'], ['keep', 'keep']], p.noData))}${field('errorBehavior', select('policy-error', [['report', 'report'], ['keep', 'keep']], p.error))}<label class='argus-alert-checkbox-label'><input id='policy-enabled' type='checkbox' name='policy-enabled' ${p.enabled ? 'checked' : ''}>${esc(t('activate'))}</label></div><div class='argus-alert-form-divider'><h3>${esc(t('destinations'))}</h3><div class='argus-alert-drawer-section'>${checkboxes('policy-channels', data.channels.map(c => [c.id, c.name]), p.channels)}</div></div>`;
}
function readPolicyForm() {
  const p = structuredClone(ui.modal.item); const f = new FormData(document.getElementById('modal-form'));
  p.name = String(f.get('policy-name') || '').trim(); p.resource = String(f.get('policy-resource')); p.mode = String(f.get('policy-mode'));
  p.interval = Number(f.get('policy-interval')); p.duration = Number(f.get('policy-duration')); p.recovery = Number(f.get('policy-recovery-duration'));
  p.noData = String(f.get('policy-nodata')); p.error = String(f.get('policy-error')); p.enabled = f.has('policy-enabled'); p.channels = f.getAll('policy-channels').map(String);
  p.rules = p.rules.map((_, index) => ({ metric: String(f.get(`rule-metric-${index}`) || '').trim(), op: String(f.get(`rule-op-${index}`)), threshold: Number(f.get(`rule-threshold-${index}`)), recovery: Number(f.get(`rule-recovery-${index}`)), severity: String(f.get(`rule-severity-${index}`)) }));
  return p;
}
function blankPolicy() { return { name: '', resource: 'all', mode: 'independent', interval: 30, duration: 5, recovery: 3, noData: 'report', error: 'keep', enabled: true, channels: ['c3'], rules: [{ metric: metrics.cpu, op: '>', threshold: 85, recovery: 75, severity: 'critical' }] }; }
function datetimeLocal(value) { return new Intl.DateTimeFormat('sv-SE', { timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }).format(new Date(value)).replace(' ', 'T'); }
function previewRows(action) {
  const d = action.data;
  const rows = [['operation', L(action.title)], ['origin', t(action.origin || 'manual')]];
  if (action.kind === 'policy.save') rows.push(['name', L(d.name)], ['scope', d.resource === 'all' ? t('allHosts') : d.resource], ['combination', t(d.mode)], ['ruleCount', ruleExpression(d)], ['destinations', d.channels.map(id => L(channelById(id)?.name || id)).join(' · ')], ['enabledAfter', t(d.enabled ? 'enabled' : 'disabled')]);
  else if (action.kind === 'silence.save') rows.push(['scope', d.resource === 'all' ? t('allHosts') : d.resource], ['policy', d.policy === 'all' ? t('allPolicies') : L(policyById(d.policy)?.name || d.policy)], ['metric', !d.metric || d.metric === 'all' ? t('allMetrics') : d.metric], ['reason', L(d.reason)], ['until', dateText(d.until)]);
  else if (action.kind === 'channel.save') rows.push(['name', L(d.name)], ['provider', L(providerLabels[d.provider])], ['endpoint', d.endpoint], ['credential', d.credential], ['status', t(d.enabled ? 'enabled' : 'disabled')]);
  else if (action.kind === 'route.save') rows.push(['name', L(d.name)], ['severity', t(d.severity)], ['destinations', d.channels.map(id => L(channelById(id)?.name || id)).join(' · ')]);
  else if (action.kind === 'inhibition.save') rows.push(['name', L(d.name)], ['source', t(d.source)], ['target', t(d.target)], ['equal', d.equal], ['reason', L(d.reason)]);
  else if (action.kind === 'grouping.save') rows.push(['groupBy', d.by], ['groupWait', String(d.wait)], ['groupInterval', String(d.interval)], ['repeatInterval', String(d.repeat)]);
  else if (action.kind === 'permissions.save') rows.push(['demoRole', t(d.role)], ['permissions', d.permissions.join(', ')]);
  else if (action.kind === 'template.save') rows.push(['template', d.text]);
  else if (action.kind === 'notification.test') rows.push(['channel', L(channelById(d.channel)?.name || d.channel)], ['previewContent', t('testContent')], ['simulateResult', t(d.status)]);
  else if (action.kind === 'notification.resend') { const delivery = data.deliveries.find(v => v.id === d.id); rows.push(['channel', L(channelById(delivery?.channel)?.name || delivery?.channel)], ['object', d.id]); }
  else rows.push(['object', d.name || d.id], ['status', d.enabled === undefined ? t('remove') : t(d.enabled ? 'enabled' : 'disabled')]);
  return rows.map(([label, value]) => `<div class='argus-alert-preview-row'><span>${esc(t(label))}</span><span style='white-space:pre-wrap;overflow-wrap:anywhere'>${esc(value)}</span></div>`).join('');
}
function renderPending(action) {
  const waiting = action.status === 'waiting';
  return `<section class='argus-alert-panel argus-alert-pending'><div class='argus-alert-panel-head'><h2>${esc(t('preview'))}</h2>${badge(action.status, waiting ? 'brand' : action.status === 'confirmed' ? 'success' : '')}</div>${callout('pendingHint')}<div class='argus-alert-preview-scroll'>${previewRows(action)}</div><p class='argus-alert-secondary argus-alert-drawer-section'>${esc(t('localAction'))}</p><div class='argus-alert-actions argus-alert-actions--end'>${btn('cancel', 'pending-cancel', { disabled: !waiting })}${btn('confirm', 'pending-confirm', { primary: true, disabled: !waiting || !can(action.permission) })}</div></section>`;
}
function stage(action) {
  if (!can(action.permission)) return toast('forbidden');
  ui.pending = Object.freeze({ ...structuredClone(action), id: uid('demo-action-'), expires: Date.now() + 300000, status: 'waiting' });
  if (modal.open) modal.close(); ui.modal = null;
  if (action.origin === 'model') { ui.result = null; changePage('workspace'); }
  else openModal('preview', ui.pending);
}
function openModal(type, item = {}) { ui.modal = { type, item: structuredClone(item) }; renderModal(); if (!modal.open) modal.showModal(); }
function closeModal() { if (ui.modal?.type === 'preview' && ui.pending?.status === 'waiting') cancelPending(); else { modal.close(); ui.modal = null; } }
function renderModal() {
  const { type, item } = ui.modal; let title = '', body = '', saveLabel = 'preview', extraButtons = '';
  if (type === 'policy') { title = item.id ? 'edit' : 'create'; body = policyForm(item); }
  else if (type === 'policy-view') { title = 'manageRules'; body = `<h2>${esc(L(item.name))}</h2><div class='argus-alert-drawer-section'>${healthBadge(item.health)} ${badge(item.enabled ? 'enabled' : 'disabled', 'info')}</div><div class='argus-alert-code-block'>${esc(ruleExpression(item))}</div><div class='argus-alert-preview-row'><span>${esc(t('scope'))}</span><span>${esc(item.resource === 'all' ? t('allHosts') : item.resource)}</span></div><div class='argus-alert-preview-row'><span>${esc(t('destinations'))}</span><span>${esc(item.channels.map(id => L(channelById(id)?.name || id)).join(' · '))}</span></div>`; extraButtons = btn('copy', 'policy-copy', { id: item.id, perm: 'policy' }) + btn('remove', 'policy-delete', { id: item.id, danger: true, perm: 'policy' }) + btn('edit', 'policy-edit', { id: item.id, primary: true, perm: 'policy' }); saveLabel = ''; }
  else if (type === 'silence') { title = item.id ? 'edit' : 'createSilence'; body = callout('noiseHint') + `<div class='argus-alert-grid'>${field('name', input('silence-name', L(item.name || ''), 'text', 'required'))}${field('scope', select('silence-resource', resourceOptions, item.resource || 'all'))}${field('policy', select('silence-policy', [['all', 'allPolicies'], ...data.policies.map(p => [p.id, p.name])], item.policy || 'all'))}${field('metric', select('silence-metric', [['all', 'allMetrics'], ...Object.values(metrics).map(m => [m, m])], item.metric || 'all'))}${field('durationHours', input('silence-hours', 1, 'number', 'min=1 max=720'))}${field('until', input('silence-until', datetimeLocal(item.until || Date.now() + 3600000), 'datetime-local', 'required'))}${field('reason', input('silence-reason', L(item.reason || ''), 'text', 'required'))}</div>`; }
  else if (type === 'inhibition') { title = item.id ? 'edit' : 'createInhibition'; body = callout('noiseHint') + `<div class='argus-alert-grid'>${field('name', input('inhibition-name', L(item.name || ''), 'text', 'required'), '', true)}${field('ruleSource', select('inhibition-source', [['critical', 'critical'], ['warning', 'warning']], item.source || 'critical'))}${field('ruleTarget', select('inhibition-target', [['warning', 'warning'], ['info', 'info']], item.target || 'warning'))}${field('equal', input('inhibition-equal', item.equal || 'resource_id, policy_id', 'text', 'required'))}${field('reason', input('inhibition-reason', L(item.reason || ''), 'text', 'required'))}</div>`; }
  else if (type === 'channel') { title = item.id ? 'configure' : 'createChannel'; body = `<div class='argus-alert-grid'>${field('name', input('channel-name', L(item.name || ''), 'text', 'required'))}${field('provider', select('channel-provider', Object.entries(providerLabels), item.provider || 'dingtalk'))}${field(item.provider === 'email' ? 'recipient' : 'botEndpoint', input('channel-endpoint', item.endpoint || (item.provider === 'email' ? 'operations@example.invalid' : 'https://example.invalid/webhook/team'), item.provider === 'email' ? 'email' : 'url', 'required'), '', true)}${item.provider === 'email' ? field('smtpHost', input('channel-smtp', item.smtp || 'smtp.example.invalid', 'text', 'required')) + field('smtpPort', input('channel-port', item.port || 465, 'number', 'required min=1 max=65535')) + field('smtpSecurity', select('channel-security', [['tls','TLS'],['starttls','STARTTLS']], item.security || 'tls')) + field('fromAddress', input('channel-from', item.from || 'alerts@example.invalid', 'email', 'required')) : ''}${field('credential', select('channel-credential', ['demo-secret-01', 'demo-secret-02', 'demo-secret-03', 'demo-secret-04'].map(id => [id, id]), item.credential || 'demo-secret-01'))}<label class='argus-alert-checkbox-label'><input name='channel-enabled' type='checkbox' ${item.enabled !== false ? 'checked' : ''}>${esc(t('enabled'))}</label></div><p class='argus-alert-secondary'>${esc(t('notifyTestHint'))}</p>`; }
  else if (type === 'test') { title = 'test'; body = callout('notifyTestHint') + `<div class='argus-alert-preview-row'><span>${esc(t('channel'))}</span><b>${esc(L(item.name))}</b></div><div class='argus-alert-code-block'>${esc(t('testContent'))}</div><div class='argus-alert-drawer-section'>${field('simulateResult', select('test-status', [['received', 'simulateSuccess'], ['failed', 'simulateFailure']], 'received'))}</div>`; }
  else if (type === 'route') { title = item.id ? 'edit' : 'createRoute'; body = `<div class='argus-alert-grid'>${field('name', input('route-name', L(item.name || ''), 'text', 'required'))}${field('severity', select('route-severity', [['all', 'all'], ['critical', 'critical'], ['warning', 'warning'], ['info', 'info']], item.severity || 'critical'))}</div><h3>${esc(t('destinations'))}</h3><div class='argus-alert-drawer-section'>${checkboxes('route-channels', data.channels.map(c => [c.id, c.name]), item.channels || ['c3'])}</div>`; }
  else if (type === 'grouping') { title = 'grouping'; body = `<div class='argus-alert-grid'>${field('groupBy', select('grouping-by', [['service', 'service'], ['resource_id', 'resource_id'], ['policy_id', 'policy_id']], data.grouping.by))}${field('groupWait', input('grouping-wait', data.grouping.wait, 'number', 'required min=0 max=3600'))}${field('groupInterval', input('grouping-interval', data.grouping.interval, 'number', 'required min=1 max=86400'))}${field('repeatInterval', input('grouping-repeat', data.grouping.repeat, 'number', 'required min=1 max=604800'))}</div><h3>${esc(t('timing'))}</h3><div class='argus-alert-drawer-section'>${checkboxes('grouping-timing', [['first', 'notifyFirst'], ['repeat', 'notifyRepeat'], ['recovery', 'notifyRecovery'], ['severity', 'notifySeverity']], data.grouping.timing)}</div>`; }
  else if (type === 'template') { title = 'templates'; body = callout('templateHint') + field('template', `<textarea id='template-text' name='template-text' required rows='7'>${esc(data.template)}</textarea>`); }
  else if (type === 'permissions') { title = 'editPermissions'; body = callout('rolesHint') + field('demoRole', select('permission-role', [['editor', 'editor'], ['viewer', 'viewer']], item.role || 'editor')) + checkboxes('permission-values', permissionKeys.map(p => [p, `${p}Permission`]), data.permissions[item.role || 'editor']); }
  else if (type === 'delivery') { title = 'deliveries'; const e = eventById(item.event); body = `${deliveryBadge(item.status)}<div class='argus-alert-drawer-section'>${previewRows({ title: I.deliveries, origin: 'manual', kind: 'notification.resend', data: item })}</div><div class='argus-alert-code-block'>${esc(e ? L(e.name) : t('testContent'))}\n${esc(t(item.reason))}\n${esc(t('attempts'))}: ${item.attempts}\n${esc(item.time)}</div><ol class='argus-alert-timeline'>${(item.history || Array.from({length:item.attempts}, (_, i) => ({number:i+1,status:item.status,reason:item.reason,time:item.time}))).slice().reverse().map(attempt => `<li><div>${esc(t('attempts'))} #${attempt.number} · ${esc(t(attempt.status))}</div><p class='argus-alert-secondary'>${esc(attempt.time)} · ${esc(t(attempt.reason))}</p></li>`).join('')}</ol>${callout('notifyTestHint')}`; saveLabel = ''; extraButtons = btn('resend', 'resend', { id: item.id, primary: true, perm: 'channel', disabled: !['failed', 'unknown'].includes(item.status) }); }
  else if (type === 'preview') { title = 'preview'; body = callout('pendingHint') + previewRows(item); saveLabel = ''; extraButtons = btn('confirm', 'pending-confirm', { primary: true, perm: item.permission }); }
  else if (type === 'reset') { title = 'reset'; body = callout('resetHint'); saveLabel = 'reset'; }
  else if (type === 'coverage') { title = 'coverage'; body = callout('covered') + ['D01 · 当前告警 / Active alerts', 'D02 · 告警历史 / Alert history', 'D03 · 通知记录 / Deliveries', 'D04 · 会话工作台 → Tool Hub → 多指标创建 / Workspace tools: multi-metric creation', 'D05 · 会话工作台 → Tool Hub → 指标与告警查询 / Workspace tools: metric and alert queries', 'D06 · 会话工具与人工配置 / Conversation tools and manual configuration', 'D07 · 确认、认领与评论 / Acknowledge, assign & comment', 'D08 · 组织与权限 → 角色、资源授权与告警审计 / Organization & Access'].map(label => `<div class='argus-alert-result-row'><span>${esc(label)}</span>${icon('check')}</div>`).join(''); saveLabel = ''; }
  modal.innerHTML = `<form id='modal-form'><header class='argus-alert-modal-head'><h2 id='modal-title'>${esc(t(title))}</h2>${btn('close', 'modal-close', { quiet: true, small: true, icon: 'x' })}</header><div class='argus-alert-modal-body'>${body}<div id='form-error' class='argus-alert-field-error' role='alert'></div></div><footer class='argus-alert-modal-foot'><span class='argus-alert-secondary'>${esc(t('localAction'))}</span><div class='argus-alert-actions'>${btn(type === 'coverage' || type === 'delivery' || type === 'policy-view' ? 'close' : 'cancel', 'modal-close')}${extraButtons}${saveLabel ? `<button type='submit' class='argus-alert-btn argus-alert-btn--primary'>${esc(t(saveLabel))}</button>` : ''}</div></footer></form>`;
}
function formError(key) { document.getElementById('form-error').textContent = t(key); }
function saveModal() {
  if (!ui.modal) return;
  const { type, item } = ui.modal; const form = document.getElementById('modal-form'); if (!form.reportValidity()) return;
  const f = new FormData(form); let action;
  if (type === 'policy') { const p = readPolicyForm(); if (!p.channels.length || p.rules.some(r => !r.metric || !Number.isFinite(r.threshold) || !Number.isFinite(r.recovery) || (r.op === '>' ? r.recovery >= r.threshold : r.recovery < r.threshold))) return formError('required'); action = { kind: 'policy.save', title: p.id ? I.edit : I.create, permission: 'policy', data: p }; }
  else if (type === 'silence') { const until = new Date(`${f.get('silence-until')}+08:00`); if (Number.isNaN(until.valueOf()) || until.valueOf() <= Date.now()) return formError('required'); action = { kind: 'silence.save', title: I.createSilence, permission: 'silence', data: { ...item, name: String(f.get('silence-name')).trim(), resource: String(f.get('silence-resource')), policy: String(f.get('silence-policy')), metric: String(f.get('silence-metric')), reason: String(f.get('silence-reason')).trim(), until: until.toISOString(), enabled: true } }; }
  else if (type === 'inhibition') { const equal = String(f.get('inhibition-equal')); const source = String(f.get('inhibition-source')), target = String(f.get('inhibition-target')); if (!equal.split(',').map(v => v.trim()).includes('resource_id') || source === target) return formError('required'); action = { kind: 'inhibition.save', title: I.createInhibition, permission: 'silence', data: { ...item, name: String(f.get('inhibition-name')).trim(), source, target, equal, reason: String(f.get('inhibition-reason')).trim(), enabled: true } }; }
  else if (type === 'channel') { const endpoint = String(f.get('channel-endpoint')); const provider = String(f.get('channel-provider')); if (provider !== 'email' && !endpoint.startsWith('https://')) return formError('invalidEndpoint'); action = { kind: 'channel.save', title: I.configure, permission: 'channel', data: { ...item, name: String(f.get('channel-name')).trim(), endpoint, provider, credential: String(f.get('channel-credential')), smtp: String(f.get('channel-smtp') || ''), port: Number(f.get('channel-port') || 465), security: String(f.get('channel-security') || 'tls'), from: String(f.get('channel-from') || ''), enabled: f.has('channel-enabled') } }; }
  else if (type === 'route') { const channels = f.getAll('route-channels').map(String); if (!channels.length) return formError('required'); action = { kind: 'route.save', title: I.createRoute, permission: 'channel', data: { ...item, name: String(f.get('route-name')).trim(), severity: String(f.get('route-severity')), channels } }; }
  else if (type === 'grouping') action = { kind: 'grouping.save', title: I.grouping, permission: 'channel', data: { by: String(f.get('grouping-by')), wait: Number(f.get('grouping-wait')), interval: Number(f.get('grouping-interval')), repeat: Number(f.get('grouping-repeat')), timing: f.getAll('grouping-timing').map(String), revision: data.grouping.revision } };
  else if (type === 'test') action = { kind: 'notification.test', title: I.test, permission: 'channel', data: { channel: item.id, status: String(f.get('test-status')) } };
  else if (type === 'template') action = { kind: 'template.save', title: I.templates, permission: 'channel', data: { text: String(f.get('template-text')) } };
  else if (type === 'permissions') action = { kind: 'permissions.save', title: I.editPermissions, permission: 'iam', data: { role: String(f.get('permission-role')), permissions: f.getAll('permission-values').map(String) } };
  else if (type === 'reset') { data = initialData(); ui.pending = null; ui.selected.clear(); ui.messages = []; ui.result = null; ui.role = 'admin'; ui.filters = {}; ui.group = false; ui.noise = 'silences'; ui.channels = 'contactPoints'; ui.security = 'permissions'; closeModal(); changeTab('active'); return toast('resetHint'); }
  if (action) stage({ ...action, origin: 'manual' });
}
function recomputeNoise() {
  for (const e of current()) {
    e.notification = e.baseNotification;
    if (e.notification === 'unknown') continue;
    const silence = data.silences.find(s => s.enabled && new Date(s.until).valueOf() > Date.now() && silenceMatches(s).some(v => v.id === e.id));
    if (silence) { e.notification = 'muted'; continue; }
    const inhibition = data.inhibitions.find(i => i.enabled && e.severity === i.target && current().some(v => v.id !== e.id && v.resource === e.resource && v.severity === i.source && (!i.equal.includes('policy_id') || v.policy === e.policy) && v.status === 'firing'));
    if (inhibition) e.notification = 'inhibited';
  }
}
function commitPending() {
  const action = ui.pending; if (!action || action.status !== 'waiting') return;
  if (!can(action.permission)) return toast('forbidden');
  if (Date.now() > action.expires) return toast('expired');
  if (data.completed.includes(action.id)) return;
  const d = structuredClone(action.data);
  const collection = ({ 'policy.save': data.policies, 'policy.state': data.policies, 'policy.delete': data.policies, 'silence.save': data.silences, 'silence.state': data.silences, 'inhibition.save': data.inhibitions, 'inhibition.state': data.inhibitions, 'channel.save': data.channels, 'route.save': data.routes })[action.kind];
  const old = collection?.find(v => v.id === d.id);
  if (d.id && collection && (!old || d.revision !== old.revision)) return toast('conflict');
  if (action.kind === 'grouping.save' && d.revision !== data.grouping.revision) return toast('conflict');
  if (action.kind.endsWith('.save') && collection) { const entry = { ...d, id: d.id || uid(action.kind.split('.')[0] + '-'), revision: (d.revision || 0) + 1 }; if (action.kind === 'policy.save') { entry.health = 'notEvaluated'; entry.lastEval = null; } if (old) collection.splice(collection.indexOf(old), 1, entry); else collection.unshift(entry); }
  else if (action.kind.endsWith('.state') && old) { old.enabled = d.enabled; old.revision++; }
  else if (action.kind === 'policy.delete') data.policies = data.policies.filter(p => p.id !== d.id);
  else if (action.kind === 'grouping.save') data.grouping = { ...d, revision: d.revision + 1 };
  else if (action.kind === 'template.save') data.template = d.text;
  else if (action.kind === 'permissions.save') data.permissions[d.role] = d.permissions;
  else if (action.kind === 'notification.test') data.deliveries.unshift({ id: uid('d'), event: null, channel: d.channel, status: d.status, attempts: 1, time: now(), reason: d.status === 'received' ? 'acceptedReason' : 'simulateFailure', history:[{number:1,status:d.status,time:now(),reason:d.status === 'received' ? 'acceptedReason' : 'simulateFailure'}] });
  else if (action.kind === 'notification.resend') { const delivery = data.deliveries.find(v => v.id === d.id); if (delivery) { delivery.history ||= Array.from({length:delivery.attempts}, (_, i) => ({number:i+1,status:delivery.status,reason:delivery.reason,time:delivery.time})); delivery.attempts++; delivery.history.push({number:delivery.attempts,status:'received',reason:'acceptedReason',time:now()}); delivery.status = 'received'; delivery.time = now(); delivery.reason = 'acceptedReason'; const e = eventById(delivery.event); if (e) e.baseNotification = 'received'; } }
  recomputeNoise(); data.completed.push(action.id); audit(action.title, d.id || L(d.name || action.title), action.origin); ui.pending = Object.freeze({ ...action, status: 'confirmed' });
  if (action.origin === 'model') ui.messages.push({ role: 'assistant', text: I.aiCommitted });
  if (modal.open) modal.close(); ui.modal = null; render(); toast(action.kind.startsWith('notification.') ? 'simulatedSend' : action.kind === 'permissions.save' ? 'changeRole' : 'committed');
}
function cancelPending() { if (!ui.pending || ui.pending.status !== 'waiting') return; ui.pending = Object.freeze({ ...ui.pending, status: 'canceled' }); if (modal.open) modal.close(); ui.modal = null; render(); toast('canceledHint'); }
function aiSend(prompt, preset = '') {
  if (!String(prompt).trim()) return;
  ui.messages.push({ role: 'user', text: [String(prompt), String(prompt)] });
  if (/解除.*屏蔽|end.*silence|unsilence/i.test(prompt) || preset === 'aiUnsilence') {
    const resource = prompt.match(/web-prod-0[12]|db-prod-01/i)?.[0];
    const s = data.silences.find(v => v.enabled && (!resource || v.resource === resource));
    if (!can('silence')) { ui.messages.push({ role: 'assistant', text: I.forbidden }); return render(); }
    if (!s) { ui.messages.push({ role: 'assistant', text: I.aiNotFound }); return render(); }
    hubReply(I.aiPrepared, 'alert.silence.update.preview');
    return stage({ kind: 'silence.state', title: I.revoke, permission: 'silence', origin: 'model', data: { id: s.id, revision: s.revision, name: L(s.name), enabled: false } });
  }
  if (/修改|更新|停用|启用|update|modify|disable|enable/i.test(prompt) || ['aiModify', 'aiPause', 'aiResume'].includes(preset)) {
    if (!can('policy')) { ui.messages.push({ role: 'assistant', text: I.forbidden }); return render(); }
    const candidates = data.policies.filter(v => v.rules.some(r => /cpu/i.test(r.metric)));
    const named = candidates.find(v => (Array.isArray(v.name) ? v.name : [v.name]).some(name => prompt.toLowerCase().includes(name.toLowerCase())));
    const p = ['aiModify', 'aiPause', 'aiResume'].includes(preset) ? policyById('p1') : named || (candidates.length === 1 ? candidates[0] : null);
    if (!p && candidates.length > 1) { ui.pending = null; ui.result = { type: 'policies', rows: candidates }; ui.messages.push({ role: 'assistant', text: I.aiChoosePolicy }); return render(); }
    if (!p) { ui.messages.push({ role: 'assistant', text: I.aiNotFound }); return render(); }
    if (/停用|启用|disable|enable/i.test(prompt) || ['aiPause', 'aiResume'].includes(preset)) {
      const enabled = /启用|enable/i.test(prompt) || preset === 'aiResume';
      hubReply(I.aiPrepared, 'alert.policy.state.preview');
      return stage({ kind: 'policy.state', title: I[enabled ? 'resume' : 'pause'], permission: 'policy', origin: 'model', data: { id: p.id, revision: p.revision, name: L(p.name), enabled } });
    }
    const changed = structuredClone(p); const cpu = changed.rules.find(r => /cpu/i.test(r.metric));
    cpu.threshold = Number(prompt.match(/(?:为|to|threshold)[^\d]{0,6}(\d+(?:\.\d+)?)/i)?.[1] || 90);
    cpu.recovery = Math.min(cpu.recovery, cpu.threshold - 10);
    hubReply(I.aiPrepared, 'alert.policy.update.preview');
    return stage({ kind: 'policy.save', title: I.edit, permission: 'policy', origin: 'model', data: changed });
  }
  if (/创建|create/i.test(prompt) || preset === 'aiCreate') {
    if (!can('policy')) { ui.messages.push({ role: 'assistant', text: I.forbidden }); return render(); }
    const p = blankPolicy(); p.name = ['生产主机双指标告警', 'Production host multi-metric alert'];
    p.resource = prompt.match(/web-prod-0[12]|db-prod-01|k8s-production/i)?.[0] || 'all';
    p.mode = /同时|并且|\band\b/i.test(prompt) ? 'every' : /任一|\bor\b/i.test(prompt) ? 'any' : 'independent';
    p.rules[0].threshold = Number(prompt.match(/cpu[^\d]{0,12}(\d+(?:\.\d+)?)/i)?.[1] || 85);
    p.rules[0].recovery = Math.max(0, p.rules[0].threshold - 10);
    p.rules.push({ metric: metrics.memory, op: '>', threshold: Number(prompt.match(/(?:内存|memory)[^\d]{0,12}(\d+(?:\.\d+)?)/i)?.[1] || 80), recovery: 70, severity: 'warning' });
    if (p.rules[1].recovery >= p.rules[1].threshold) p.rules[1].recovery = Math.max(0, p.rules[1].threshold - 10);
    hubReply(I.aiPrepared, 'alert.policy.create.preview'); ui.result = null;
    return stage({ kind: 'policy.save', title: I.create, permission: 'policy', origin: 'model', data: p });
  }
  if (/屏蔽|silence|mute/i.test(prompt) || preset === 'aiSilence') {
    if (!can('silence')) { ui.messages.push({ role: 'assistant', text: I.forbidden }); return render(); }
    const resource = prompt.match(/web-prod-0[12]|db-prod-01/i)?.[0] || 'web-prod-01';
    hubReply(I.aiPrepared, 'alert.silence.create.preview');
    return stage({ kind: 'silence.save', title: I.createSilence, permission: 'silence', origin: 'model', data: { name: ['会话临时屏蔽', 'Conversation silence'], resource, policy: 'all', reason: ['排查期间临时屏蔽一小时', 'Silenced for one hour while investigating'], until: new Date(Date.now() + 3600000).toISOString(), enabled: true } });
  }
  if (!can('read')) { ui.messages.push({ role: 'assistant', text: I.forbidden }); return render(); }
  ui.pending = null;
  if (/规则|策略|存在|\brule\b|policy|exist/i.test(prompt) || preset === 'aiExists') {
    const key = preset === 'aiExists' ? 'cpu' : prompt.replace(/规则|策略|检查|查找|查询|是否|存在|告警|请|帮我|一下|的|\brule\b|policy|check|does|exist|[?？:：'"\s]/gi, '').toLowerCase();
    const rows = key ? data.policies.filter(p => JSON.stringify(p).toLowerCase().includes(key) || (/cpu|主机/.test(key) && p.id === 'p1')) : data.policies;
    ui.result = { type: 'policies', rows }; const text = rows.length ? t('aiFound', { n: rows.length }) : t('aiNotFound'); hubReply([text,text], ui.result.type==='policies'?'alert.policy.list':'alert.event.query');
  } else if (/查询|告警|指标|query|alert|metric/i.test(prompt) || preset === 'aiQuery') {
    const resource = prompt.match(/web-prod-0[12]|db-prod-01|k8s-production/i)?.[0];
    const metric = /cpu/i.test(prompt) ? metrics.cpu : /内存|memory/i.test(prompt) ? metrics.memory : /磁盘|disk/i.test(prompt) ? metrics.disk : null;
    const rows = current().filter(e => (!resource || e.resource === resource) && (!metric || e.metric === metric));
    ui.result = { type: 'events', rows }; const text = t('aiRead', { n: rows.filter(e => e.status === 'firing').length, quiet: rows.filter(e => ['muted', 'inhibited'].includes(e.notification)).length }); hubReply([text,text], ui.result.type==='policies'?'alert.policy.list':'alert.event.query');
  } else ui.messages.push({ role: 'assistant', text: I.aiGuide });
  render();
}
function exportHistory() { if (!can('read')) return toast('forbidden'); const blob = new Blob([JSON.stringify({ prototype: true, scope: 'demo', events: filtered(data.events) }, null, 2)], { type: 'application/json' }); const url = URL.createObjectURL(blob); const link = document.createElement('a'); link.href = url; link.download = 'argus-plan6-demo-alert-history.json'; link.click(); setTimeout(() => URL.revokeObjectURL(url), 1000); toast('historyExported'); }
function perform(action, id, extra, element) {
  if (action === 'tab') return changeTab(id);
  if (action === 'subtab') { ui[id] = extra; return id === 'detailTab' ? renderDrawer() : render(); }
  if (action === 'locale') { pref.locale = pref.locale === 'zh' ? 'en' : 'zh'; render(); if (ui.modal) renderModal(); return; }
  if (action === 'theme') { pref.theme = pref.theme === 'light' ? 'dark' : 'light'; return render(); }
  if (action === 'page') { filters().page += Number(extra); return render(); }
  if (action === 'group') { ui.group = !ui.group; return render(); }
  if (action === 'select-event') { element.checked ? ui.selected.add(id) : ui.selected.delete(id); return render(); }
  if (action === 'select-all') { const rows = filtered(current()).slice((filters().page - 1) * 5, filters().page * 5); rows.forEach(e => element.checked ? ui.selected.add(e.id) : ui.selected.delete(e.id)); return render(); }
  if (action === 'ack') return recordHandling(id, 'ack');
  if (action === 'bulk-ack') { [...ui.selected].forEach(key => recordHandling(key, 'ack')); ui.selected.clear(); return render(); }
  if (action === 'assign') return recordHandling(id, 'owner', document.getElementById('detail-owner').value);
  if (action === 'comment') return recordHandling(id, 'comment', document.getElementById('detail-comment').value);
  if (action === 'event-detail') return openDrawer(id);
  if (action === 'drawer-close') return closeDrawer();
  if (action === 'modal-close') return closeModal();
  if (action === 'pending-confirm') return commitPending();
  if (action === 'pending-cancel') return cancelPending();
  if (action === 'policy-create') return can('policy') ? openModal('policy', blankPolicy()) : toast('forbidden');
  if (action === 'policy-edit') return can('policy') ? openModal('policy', policyById(id)) : toast('forbidden');
  if (action === 'policy-copy') { const p = structuredClone(policyById(id)); delete p.id; delete p.revision; p.name = [L(p.name) + ' · 副本', L(p.name) + ' · Copy']; return openModal('policy', p); }
  if (action === 'policy-view') return openModal('policy-view', policyById(id));
  if (action === 'policy-state' || action === 'policy-delete') { const p = policyById(id); return stage({ kind: action === 'policy-state' ? 'policy.state' : 'policy.delete', permission: 'policy', title: action === 'policy-delete' ? I.remove : I[p.enabled ? 'pause' : 'resume'], origin: 'manual', data: { id, name: L(p.name), revision: p.revision, ...(action === 'policy-state' ? { enabled: !p.enabled } : {}) } }); }
  if (action === 'rule-add' || action === 'rule-remove') { ui.modal.item = readPolicyForm(); if (action === 'rule-add') ui.modal.item.rules.push({ metric: metrics.memory, op: '>', threshold: 80, recovery: 70, severity: 'warning' }); else ui.modal.item.rules.splice(Number(id), 1); return renderModal(); }
  if (action === 'rule-test') { if (!document.getElementById('modal-form').reportValidity()) return; const p = readPolicyForm(); const samples = { [metrics.cpu]: 94.7, [metrics.memory]: 89.2, [metrics.disk]: 96.1, [metrics.errors]: 4.8 }; document.getElementById('trial-output').innerHTML = `<div class='argus-alert-callout argus-alert-drawer-section'>${icon('info')}<span>${esc(t('trial'))}</span></div><div class='argus-alert-code-block'>${esc(p.rules.map(r => `${r.metric} ${r.op} ${r.threshold} → ${samples[r.metric] === undefined ? t('noData') : String(r.op === '>' ? samples[r.metric] > r.threshold : samples[r.metric] < r.threshold)}`).join('\n'))}</div>`; return; }
  if (action === 'event-silence') { const e = eventById(id); return openModal('silence', { resource: e.resource, policy: e.policy, metric: e.metric, name: ['排查期间临时屏蔽', 'Investigation silence'], reason: ['维护或人工排查', 'Maintenance or investigation'] }); }
  if (action === 'silence-create') return openModal('silence');
  if (action === 'silence-edit') return openModal('silence', data.silences.find(s => s.id === id));
  if (action === 'silence-revoke') { const s = data.silences.find(v => v.id === id); return stage({ kind: 'silence.state', title: I.revoke, permission: 'silence', origin: 'manual', data: { id, revision: s.revision, name: L(s.name), enabled: false } }); }
  if (action === 'inhibition-create') return openModal('inhibition');
  if (action === 'inhibition-edit') return openModal('inhibition', data.inhibitions.find(v => v.id === id));
  if (action === 'inhibition-state') { const item = data.inhibitions.find(v => v.id === id); return stage({ kind: 'inhibition.state', title: I[item.enabled ? 'pause' : 'resume'], permission: 'silence', origin: 'manual', data: { id, revision: item.revision, name: L(item.name), enabled: !item.enabled } }); }
  if (action === 'channel-create') return openModal('channel', { provider: 'dingtalk' });
  if (action === 'channel-edit') return openModal('channel', channelById(id));
  if (action === 'channel-test') return openModal('test', channelById(id));
  if (action === 'route-create') return openModal('route');
  if (action === 'route-edit') return openModal('route', data.routes.find(v => v.id === id));
  if (action === 'grouping-edit') return openModal('grouping');
  if (action === 'template-edit') return openModal('template');
  if (action === 'permissions-edit') return openModal('permissions', { role: 'editor' });
  if (action === 'delivery-detail') return openModal('delivery', data.deliveries.find(v => v.id === id));
  if (action === 'resend') return stage({ kind: 'notification.resend', title: I.resend, permission: 'channel', origin: 'manual', data: { id } });
  if (action === 'page-entry') return changePage(id);
  if (action === 'ai-prompt') { const input = document.getElementById('ai-input'); input.value = t(id); input.focus(); return; }
  if (action === 'export-history') return exportHistory();
  if (action === 'coverage') return openModal('coverage');
  if (action === 'reset') return openModal('reset');
}
document.addEventListener('click', event => { const target = event.target.closest('[data-action]'); if (target && !target.disabled) perform(target.dataset.action, target.dataset.id, target.dataset.extra, target); });
document.addEventListener('submit', event => { event.preventDefault(); if (event.target.id === 'modal-form') saveModal(); if (event.target.id === 'ai-form') aiSend(new FormData(event.target).get('prompt')); });
document.addEventListener('input', event => { if (event.target.id === 'filter-query') { filters().query = event.target.value; filters().page = 1; render(); } });
document.addEventListener('change', event => {
  const target = event.target;
  if (target.id.startsWith('filter-') && target.id !== 'filter-query') { filters()[target.id.slice(7)] = target.value; filters().page = 1; render(); }
  if (target.id === 'demo-role') { ui.role = target.value; render(); toast('rolesHint'); }
  if (target.id === 'permission-role') { ui.modal.item.role = target.value; renderModal(); }
  if (target.id === 'silence-hours') document.getElementById('silence-until').value = datetimeLocal(Date.now() + Number(target.value) * 3600000);
  if (target.id === 'channel-provider') { const f = new FormData(document.getElementById('modal-form')); ui.modal.item = { ...ui.modal.item, name: String(f.get('channel-name')), provider: target.value, credential: String(f.get('channel-credential')), endpoint: '', smtp: '' }; renderModal(); }
});
document.addEventListener('keydown', event => {
  const target = event.target; if (target.getAttribute('role') !== 'tab') return;
  if (event.ctrlKey || event.metaKey || event.altKey) return;
  if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
  event.preventDefault(); const tabs = [...target.closest('[role=tablist]').querySelectorAll('[role=tab]')]; const index = tabs.indexOf(target);
  const next = event.key === 'Home' ? 0 : event.key === 'End' ? tabs.length - 1 : (index + (event.key === 'ArrowRight' ? 1 : -1) + tabs.length) % tabs.length;
  const tab = tabs[next]; const action = tab.dataset.action, id = tab.dataset.id, extra = tab.dataset.extra; perform(action, id, extra, tab);
  const updated = action === 'tab' ? document.getElementById(`tab-${id}`) : document.querySelector(`[data-action='subtab'][data-id='${id}'][data-extra='${extra}']`); updated?.focus();
});
modal.addEventListener('cancel', event => { event.preventDefault(); closeModal(); });
drawer.addEventListener('cancel', () => { ui.drawer = null; });
window.addEventListener('hashchange', () => { const route = routeForHash(location.hash.slice(1)); if (route.page !== ui.page || (route.tab && route.tab !== ui.tab)) { Object.assign(ui, route); render(); } });
render();
