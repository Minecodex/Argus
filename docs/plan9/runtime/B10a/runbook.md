# B10a 常用对象与工作台运行包

本批运行 `p9-b10a-20261111-01`，前一批 `p9-b09b-20261110-01` 已通过正常生命周期退役。入口和实际结果见 [查看记录](../../batches/B10a-review.md)。

## 范围与前件

- 正式 Argus 位于 `docker-desktop`；所有测试对象位于独立 Kind 集群。显式使用 Context/私有 kubeconfig，每次原生操作检查 Docker 容器 ID 和 Kind 所有权标签。
- 复用 B09a 的 Kind、正常 Collector 安装、两阶段确认与精确资源清理，复用 `common/api.py` 的正常登录、CSRF、幂等键与严格 CA 验证。
- Node 限制 3 vCPU / 3072 MiB，业务容器分别设置 CPU、内存和 emptyDir 上限。StatefulSet 本批使用 emptyDir；PVC/存储留给 B10b。
- 写入权限只授予 `b10a-workloads`；Namespace 创建在本批独立集群，Namespace 更新/删除仅允许 `b10a-transient`。kube-system 写入与集群 RBAC 委派拒绝。
- Pod 日志/终端使用实际门户。按 2026-11-09 [产品边界](../../../plans/frontend-redesign/kubernetes-resource-workspace-20261109.md)，端口转发仅验收既有 API/CLI。
- 不往 Argus 业务库或遥测库构造数据。Kind、前置权限、原生 Ingress 控制器属于环境基础设施；业务对象操作全部走正常 Argus 预览/确认。

## 部署与复验顺序

以下命令在仓库根执行。正常企业会话保存于本轮 `private/enterprise-session.json`，私有目录限制 ACL；过期后走正常登录刷新，不复制数据库会话或绕过鉴权。

```powershell
python docs/plan9/runtime/B10a/transition.py preflight
python docs/plan9/runtime/B10a/transition.py retire
python docs/plan9/runtime/B10a/run.py cluster
python docs/plan9/runtime/B10a/run.py images
python docs/plan9/runtime/B10a/run.py bootstrap
python docs/plan9/runtime/B10a/run.py foundation
python docs/plan9/runtime/B10a/run.py objects
python docs/plan9/runtime/B10a/recover_ingress_image.py
python docs/plan9/runtime/B10a/ingress.py
python docs/plan9/runtime/B10a/run.py exercise
python docs/plan9/runtime/B10a/configuration_exercise.py
python docs/plan9/runtime/B10a/run.py monitoring
python docs/plan9/runtime/B10a/dashboard_metadata.py
python docs/plan9/runtime/B10a/run.py sessions
python docs/plan9/runtime/B10a/browser.py
python docs/plan9/runtime/B10a/browser_cleanup.py
python docs/plan9/runtime/B10a/browser_dashboards.py
python docs/plan9/runtime/B10a/delivery_watch.py
python docs/plan9/runtime/B10a/run.py verify
```

这组命令记录本次执行顺序，前件退役不能对已经销毁的旧节点重新运行 preflight。新一轮使用新的 run ID、私有目录和独立集群，不把旧成功证据当作新运行。

Ingress 使用已验证 SHA256 的官方 amd64 镜像；镜像恢复脚本验证公开 OCI blob 和归档，不重用未完成的多架构导出。本轮已加载成功，无需再次下载。Ingress 的 Host 为 `b10a.test`，在本批集群内用实际 HTTP 请求验证，未增加正式外网入口。

## 场景与证据

`objects` 创建 ConfigMap、受保护 Secret、两个 Service、Deployment、StatefulSet、DaemonSet、Ingress。Namespace、Deployment/StatefulSet 扩缩容、滚动重启、版本升级后回滚、Pod 删除与替换、过期 resourceVersion 拒绝都通过实际平台操作核对原生结果。

`configuration_exercise.py` 更新 Secret、检查挂载内容摘要（不记录原值）、增加实际 Service 8081 端口、更新 Ingress 并访问、重启 DaemonSet。浏览器创建/更新四组中英明暗 ConfigMap，清理脚本仅删除同一自有 Namespace 内重复的浏览器夹具，保留最终四组。

`monitoring` 接好三项原生采集、四个仪表盘及副本可用性策略，并通过 Argus 缩容到 0 实际触发，再扩容到 1 恢复。故障有 120 秒、精确 UID 的原生恢复保护；成功后取消定时器。告警只有平台内事件，未配置任何外部通知收件人。

`sessions` 使用产品 argusctl、受保护标准输入票据、严格 TLS 和回环临时端口，验证固定 Pod 的真实 HTTP 和双向字节数，再正常关闭。浏览器另验证真实 Pod 标准输出日志和 PTY 命令结果。

低速 HTTP 每 15 秒一次，固定八小时截止；到期停止请求，环境、对象和持续原生监控保留，等待人工查看。

## 本轮修复与版本

[共享修复记录](../../15-b10a-workbench-lifecycle-20261111.md) 记录 Secret 引用、ID 游标、已交付凭据租约和短命 Namespace 指标四项修正。仅向冻结 Schema52 基线叠加修复，不发布当前工作区 Schema53 或无关功能。正式六个 PVC 绑定保留。

修复前实际观察到 133 点终止投递失败；保留台账、原日志及当前历史缺口事件。修复后重复真实 Namespace 创建/删除，再观察完整 305 秒重试周期；不能通过清零台账或确认告警冒充修复。

## 之后清理

只有用户要求销毁或进入下一批时才执行：

```powershell
python docs/plan9/runtime/B10a/retirement.py
```

清理按策略 → 停采 → 自定义仪表盘归档 → Collector 卸载 → 平台集群退役 → 凭据/授权撤回 → 专属遥测入口 → 精确所有权 Kind 删除执行。保留默认共享仪表盘、平台历史和正式存储。最后验证集群 404、所属容器为 0、正式服务/PVC 基线一致。当前 B10a 尚未执行退役。
