/** Shared bilingual labels for execution, resource and transport audit events. */
type Label = readonly [string, string];
const resources: Record<string, Label> = {
  monitoring_target: ["监控目标", "Monitoring target"],
  service: ["应用服务", "Application service"],
  endpoint: ["遥测接入端点", "Telemetry endpoint"],
  binding: ["资源关联", "Resource binding"],
  runtime: ["运行环境", "Runtime"],
  distribution: ["采集配置分发", "Collector distribution"],
  kubernetes_namespace: ["Kubernetes 命名空间", "Kubernetes namespace"],
  telemetry_tunnel: ["遥测隧道", "Telemetry tunnel"],
  connector_control_tunnel: ["控制隧道", "Control tunnel"],
  monitoring_collection: ["监控采集", "Monitoring collection"],
  collection: ["监控采集", "Monitoring collection"],
  application: ["应用", "Application"],
  ingest_key: ["接入密钥", "Ingest key"],
  connector_command: ["接入代理指令", "Connector command"],
  conversation: ["会话", "Conversation"],
  mcp_connection: ["MCP 连接", "MCP connection"],
  remote_execution: ["远程执行", "Remote execution"],
  kubernetes_operation: ["Kubernetes 操作", "Kubernetes operation"],
  kubernetes_log_session: ["Kubernetes 日志会话", "Kubernetes log session"],
  kubernetes_stream_session: [
    "Kubernetes 交互会话",
    "Kubernetes stream session",
  ],
  alert: ["告警", "Alert"],
  alert_rule: ["告警规则", "Alert rule"],
  alert_policy: ["告警策略", "Alert policy"],
  alert_channel: ["通知渠道", "Notification channel"],
  alert_silence: ["告警静默", "Alert silence"],
  alert_event: ["告警事件", "Alert event"],
  alert_instance: ["告警实例", "Alert instance"],
  alert_operation: ["告警操作", "Alert operation"],
  alert_delivery: ["告警通知", "Alert delivery"],
  alert_notification: ["告警通知", "Alert notification"],
  collector: ["采集器", "Collector"],
  telemetry_collector: ["采集器", "Collector"],
  telemetry_group: ["采集网关组", "Telemetry gateway group"],
  telemetry_route: ["遥测路由", "Telemetry route"],
  host_onboarding_operation: ["主机接入", "Host onboarding"],
  host_removal_operation: ["主机移除", "Host removal"],
};
const verbs: Record<string, Label> = {
  set_enabled: ["调整启用状态：", "Set enabled state:"],
  terminate: ["终止", "Terminate"],
  patch: ["修改", "Patch"],
  apply: ["应用", "Apply"],
  scale: ["调整副本：", "Scale"],
  restart: ["重启", "Restart"],
  rollout: ["发布", "Roll out"],
  rollback: ["回滚", "Roll back"],
  cordon: ["停止调度：", "Cordon"],
  uncordon: ["恢复调度：", "Uncordon"],
  drain: ["排空", "Drain"],
  resend: ["重新发送", "Resend"],
  create: ["创建", "Create"],
  update: ["更新", "Update"],
  delete: ["删除", "Delete"],
  enable: ["启用", "Enable"],
  disable: ["停用", "Disable"],
  archive: ["归档", "Archive"],
  restore: ["恢复", "Restore"],
  claim: ["认领", "Claim"],
  connect: ["建立连接：", "Connect"],
  disconnect: ["断开", "Disconnect"],
  reconnect: ["重新连接", "Reconnect"],
  renew: ["续期", "Renew"],
  expire: ["到期：", "Expire"],
  establish: ["建立", "Establish"],
  release: ["释放", "Release"],
  reject: ["拒绝", "Reject"],
  install: ["安装", "Install"],
  uninstall: ["卸载", "Uninstall"],
  upgrade: ["升级", "Upgrade"],
  configure: ["配置", "Configure"],
  repair: ["修复", "Repair"],
  rotate: ["轮换", "Rotate"],
  replace: ["替换", "Replace"],
  retry: ["重试", "Retry"],
  cancel: ["取消", "Cancel"],
  start: ["开始", "Start"],
  stop: ["停止", "Stop"],
  finish: ["完成", "Finish"],
  execute: ["执行", "Execute"],
  submit: ["提交", "Submit"],
  confirm: ["确认", "Confirm"],
  dispatch: ["派发", "Dispatch"],
  resume: ["继续", "Resume"],
  revoke: ["撤销", "Revoke"],
  reveal: ["查看", "Reveal"],
  consume: ["使用", "Consume"],
  download: ["下载", "Download"],
  test: ["测试", "Test"],
  acknowledge: ["确认收到", "Acknowledge"],
  resolve: ["解决", "Resolve"],
  assign: ["分派", "Assign"],
  comment: ["评论", "Comment on"],
  close: ["关闭", "Close"],
  lease: ["租用", "Lease"],
  stage: ["暂存", "Stage"],
  respond: ["响应", "Respond to"],
};
const families: Array<[string, Label, string]> = [
  [
    "telemetry.tunnel",
    resources.telemetry_tunnel!,
    "claim establish disconnect",
  ],
  [
    "connector.control_tunnel",
    resources.connector_control_tunnel!,
    "claim establish disconnect",
  ],
  [
    "monitoring.collection",
    resources.collection!,
    "create update set_enabled archive",
  ],
  [
    "monitoring.application",
    resources.application!,
    "create update delete archive restore",
  ],
  [
    "application",
    resources.application!,
    "create update delete archive restore",
  ],
  [
    "application.ingest_key",
    resources.ingest_key!,
    "create rotate revoke reveal",
  ],
  ["host", ["主机", "host"], "create update delete"],
  ["monitoring.service", resources.service!, "create"],
  ["monitoring.endpoint", resources.endpoint!, "create update archive"],
  ["monitoring.ingest_key", resources.ingest_key!, "create rotate revoke"],
  ["monitoring.binding", resources.binding!, "create delete"],
  [
    "kubernetes.object",
    ["Kubernetes 资源", "Kubernetes resource"],
    "apply patch scale restart rollout rollback delete cordon uncordon drain",
  ],
  ["alert.command.event", resources.alert_event!, "acknowledge assign comment"],
  ["alert.handle", resources.alert_event!, "acknowledge assign comment"],
  ["alert.command.notification", resources.alert_notification!, "test resend"],
  ["host.enrollment", ["主机注册令牌", "host enrollment"], "rotate consume"],
  [
    "host.removal",
    ["主机移除", "host removal"],
    "uninstall confirm retry cancel",
  ],
  ["host.control_tunnel", resources.connector_control_tunnel!, "reconnect"],
  ["bastion_scope", ["堡垒机", "bastion"], "create update delete"],
  [
    "bastion.enrollment",
    ["堡垒机注册令牌", "bastion enrollment"],
    "rotate consume",
  ],
  ["bastion.connector", ["堡垒机接入代理", "bastion connector"], "replace"],
  [
    "bastion.connector.install",
    ["接入代理安装", "connector installation"],
    "retry",
  ],
  ["connector", ["接入代理", "connector"], "uninstall"],
  [
    "connector.bootstrap_script",
    ["接入安装脚本", "connector bootstrap script"],
    "download",
  ],
  [
    "kubernetes",
    ["Kubernetes 集群", "Kubernetes cluster"],
    "create update delete",
  ],
  ["kubernetes.drain", ["节点排空", "node drain"], "cancel"],
  [
    "kubernetes.helm",
    ["Helm 操作", "Helm operation"],
    "finish install upgrade uninstall",
  ],
  ["kubernetes.helm_input", ["Helm 私有输入", "private Helm input"], "stage"],
  [
    "kubernetes.private_input",
    ["Kubernetes 私有输入", "private Kubernetes input"],
    "stage lease",
  ],
  [
    "kubernetes.stream",
    resources.kubernetes_stream_session!,
    "create start stop close terminate connect disconnect",
  ],
  [
    "kubernetes.log_session",
    resources.kubernetes_log_session!,
    "create start stop close expire",
  ],
  [
    "remote_execution",
    resources.remote_execution!,
    "create execute start stop cancel resume",
  ],
  [
    "conversation.interaction",
    ["会话交互", "conversation interaction"],
    "respond",
  ],
  [
    "mcp.connection",
    resources.mcp_connection!,
    "create update delete enable disable test",
  ],
  [
    "telemetry.collector",
    resources.collector!,
    "install configure upgrade repair uninstall",
  ],
  [
    "telemetry.host",
    ["主机采集器", "host collector"],
    "install configure upgrade repair uninstall",
  ],
  [
    "telemetry.kubernetes",
    ["集群采集器", "cluster collector"],
    "install configure upgrade repair uninstall",
  ],
  ["alert.rule", resources.alert_rule!, "create update delete enable disable"],
  [
    "alert.policy",
    resources.alert_policy!,
    "create update delete enable disable",
  ],
  [
    "alert.channel",
    resources.alert_channel!,
    "create update delete enable disable test",
  ],
  ["alert.silence", resources.alert_silence!, "create update delete expire"],
  [
    "alert.event",
    resources.alert_event!,
    "acknowledge resolve assign comment close",
  ],
];
// Exact overrides cover events whose wording is not a simple verb + object.
const actions: Record<string, Label> = {
  "pending_action.auto_confirm": [
    "按授权策略确认操作",
    "Confirm operation by access policy",
  ],
  "execution.succeeded": ["操作执行成功", "Operation succeeded"],
  "execution.failed": ["操作执行失败", "Operation failed"],
  "execution.cancelled": ["操作执行已取消", "Operation cancelled"],
  "connector.command.rejected_before_dispatch": [
    "派发前拒绝接入代理指令",
    "Reject connector command before dispatch",
  ],
  "data_authorization.update": [
    "更新资源数据授权",
    "Update resource data authorization",
  ],
  "role_assignment.replace": ["调整用户角色", "Replace role assignments"],
};
for (const [prefix, resource, names] of families) {
  for (const name of names.split(" ")) {
    const verb = verbs[name];
    if (!verb) continue;
    const label: Label = [
      `${verb[0]}${resource[0]}`,
      `${verb[1]} ${resource[1]}`,
    ];
    actions[`${prefix}.${name}`] = label;
  }
}
// Preview/commit are stages of an explicitly registered domain operation.
for (const [code, label] of Object.entries(actions)) {
  if (
    /^(execution\.|pending_action\.|telemetry\.tunnel\.|connector\.control_tunnel\.|conversation\.|data_authorization\.|role_assignment\.|application\.ingest_key\.reveal)/.test(
      code,
    )
  )
    continue;
  actions[`${code}.preview`] = [`预览：${label[0]}`, `Preview: ${label[1]}`];
  actions[`${code}.commit`] = [`提交：${label[0]}`, `Commit: ${label[1]}`];
}
const key = (code: string) => code.replaceAll(".", "_").replaceAll("-", "_");
const labels = (values: Record<string, Label>, index: number) =>
  Object.fromEntries(
    Object.entries(values).map(([code, label]) => [key(code), label[index]!]),
  ) as Record<string, string>;
export const auditAdditionalActions = Object.keys(actions);
export const auditAdditionalResources = Object.keys(resources);
export const auditAdditionalLabels = {
  zh: { actions: labels(actions, 0), resourceTypes: labels(resources, 0) },
  en: { actions: labels(actions, 1), resourceTypes: labels(resources, 1) },
};
