# 当前全量工具及计划映射

日期：2026-10-29。由当前生产注册生成器的 129 个条目，加上 Server/Worker 注册的 workflow.plan.propose 生成，共 130 个。此表是源码快照，不是某个账号或部署的实际可用清单。

category/name 是模型通过元工具使用的身份；内部 ID 用于实现和审计。interaction 表示独立宿主交互；template、trace、file、action 使用现有交付声明。

## 1. 当前业务工具

| category    | name                                | 内部 ID                                        | 业务用途                       | 当前交付    | Plan11                     |
| ----------- | ----------------------------------- | ---------------------------------------------- | ------------------------------ | ----------- | -------------------------- |
| alert       | `channel.list`                      | `alert.channel.list`                           | 通知渠道列表                   | template    | 补导航元数据，保留业务职责 |
| alert       | `event.acknowledge.preview`         | `alert.event.acknowledge.preview`              | 确认处理告警预览               | template    | 补导航元数据，保留业务职责 |
| alert       | `event.assign.preview`              | `alert.event.assign.preview`                   | 分配告警负责人预览             | template    | 补导航元数据，保留业务职责 |
| alert       | `event.comment.preview`             | `alert.event.comment.preview`                  | 添加告警备注预览               | template    | 补导航元数据，保留业务职责 |
| alert       | `event.get`                         | `alert.event.get`                              | 告警详情                       | template    | 补导航元数据，保留业务职责 |
| alert       | `event.query`                       | `alert.event.query`                            | 告警列表                       | template    | 补导航元数据，保留业务职责 |
| alert       | `grouping.list`                     | `alert.grouping.list`                          | 告警合并设置列表               | template    | 补导航元数据，保留业务职责 |
| alert       | `history.query`                     | `alert.history.query`                          | 告警历史查询                   | template    | 补导航元数据，保留业务职责 |
| alert       | `inhibition.create.preview`         | `alert.inhibition.create.preview`              | 创建告警抑制规则预览           | template    | 补导航元数据，保留业务职责 |
| alert       | `inhibition.delete.preview`         | `alert.inhibition.delete.preview`              | 删除告警抑制规则预览           | template    | 补导航元数据，保留业务职责 |
| alert       | `inhibition.disable.preview`        | `alert.inhibition.disable.preview`             | 停用告警抑制规则预览           | template    | 补导航元数据，保留业务职责 |
| alert       | `inhibition.enable.preview`         | `alert.inhibition.enable.preview`              | 启用告警抑制规则预览           | template    | 补导航元数据，保留业务职责 |
| alert       | `inhibition.list`                   | `alert.inhibition.list`                        | 告警抑制规则列表               | template    | 补导航元数据，保留业务职责 |
| alert       | `inhibition.update.preview`         | `alert.inhibition.update.preview`              | 修改告警抑制规则预览           | template    | 补导航元数据，保留业务职责 |
| alert       | `notification.query`                | `alert.notification.query`                     | 告警通知查询                   | template    | 补导航元数据，保留业务职责 |
| alert       | `notification.resend.preview`       | `alert.notification.resend.preview`            | 补发告警通知预览               | template    | 补导航元数据，保留业务职责 |
| alert       | `notification.test.preview`         | `alert.notification.test.preview`              | 测试告警通知预览               | template    | 补导航元数据，保留业务职责 |
| alert       | `policy.create.preview`             | `alert.policy.create.preview`                  | 创建告警策略预览               | template    | 补导航元数据，保留业务职责 |
| alert       | `policy.delete.preview`             | `alert.policy.delete.preview`                  | 删除告警策略预览               | template    | 补导航元数据，保留业务职责 |
| alert       | `policy.disable.preview`            | `alert.policy.disable.preview`                 | 停用告警策略预览               | template    | 补导航元数据，保留业务职责 |
| alert       | `policy.enable.preview`             | `alert.policy.enable.preview`                  | 启用告警策略预览               | template    | 补导航元数据，保留业务职责 |
| alert       | `policy.get`                        | `alert.policy.get`                             | 告警策略详情                   | template    | 补导航元数据，保留业务职责 |
| alert       | `policy.list`                       | `alert.policy.list`                            | 告警策略列表                   | template    | 补导航元数据，保留业务职责 |
| alert       | `policy.update.preview`             | `alert.policy.update.preview`                  | 修改告警策略预览               | template    | 补导航元数据，保留业务职责 |
| alert       | `route.create.preview`              | `alert.route.create.preview`                   | 创建通知路由预览               | template    | 补导航元数据，保留业务职责 |
| alert       | `route.delete.preview`              | `alert.route.delete.preview`                   | 删除通知路由预览               | template    | 补导航元数据，保留业务职责 |
| alert       | `route.disable.preview`             | `alert.route.disable.preview`                  | 停用通知路由预览               | template    | 补导航元数据，保留业务职责 |
| alert       | `route.enable.preview`              | `alert.route.enable.preview`                   | 启用通知路由预览               | template    | 补导航元数据，保留业务职责 |
| alert       | `route.list`                        | `alert.route.list`                             | 通知路由列表                   | template    | 补导航元数据，保留业务职责 |
| alert       | `route.update.preview`              | `alert.route.update.preview`                   | 修改通知路由预览               | template    | 补导航元数据，保留业务职责 |
| alert       | `settings.list`                     | `alert.settings.list`                          | 告警评估账号列表               | template    | 补导航元数据，保留业务职责 |
| alert       | `silence.create.preview`            | `alert.silence.create.preview`                 | 创建告警屏蔽预览               | template    | 补导航元数据，保留业务职责 |
| alert       | `silence.delete.preview`            | `alert.silence.delete.preview`                 | 删除告警屏蔽预览               | template    | 补导航元数据，保留业务职责 |
| alert       | `silence.disable.preview`           | `alert.silence.disable.preview`                | 停用告警屏蔽预览               | template    | 补导航元数据，保留业务职责 |
| alert       | `silence.enable.preview`            | `alert.silence.enable.preview`                 | 启用告警屏蔽预览               | template    | 补导航元数据，保留业务职责 |
| alert       | `silence.list`                      | `alert.silence.list`                           | 告警屏蔽列表                   | template    | 补导航元数据，保留业务职责 |
| alert       | `silence.update.preview`            | `alert.silence.update.preview`                 | 修改告警屏蔽预览               | template    | 补导航元数据，保留业务职责 |
| alert       | `template.list`                     | `alert.template.list`                          | 告警消息模板列表               | template    | 补导航元数据，保留业务职责 |
| application | `change.preview`                    | `application.change.preview`                   | 应用配置变更预览               | template    | 补导航元数据，保留业务职责 |
| application | `endpoint.connection`               | `application.endpoint.connection`              | SDK 接入配置                   | template    | 补导航元数据，保留业务职责 |
| application | `endpoint.list`                     | `application.endpoint.list`                    | 应用接入点                     | template    | 补导航元数据，保留业务职责 |
| application | `get`                               | `application.get`                              | 应用详情                       | template    | 补导航元数据，保留业务职责 |
| application | `key.list`                          | `application.key.list`                         | 接入 Key 元数据                | template    | 补导航元数据，保留业务职责 |
| application | `list`                              | `application.list`                             | 应用列表                       | template    | 补导航元数据，保留业务职责 |
| application | `service.list`                      | `application.service.list`                     | 应用服务列表                   | template    | 补导航元数据，保留业务职责 |
| connector   | `bastion.create.preview`            | `connector.bastion.create.preview`             | 创建堡垒机范围                 | template    | 补导航元数据，保留业务职责 |
| connector   | `bastion.delete.preview`            | `connector.bastion.delete.preview`             | 删除堡垒机范围                 | template    | 补导航元数据，保留业务职责 |
| connector   | `bastion.enrollment_rotate.preview` | `connector.bastion.enrollment_rotate.preview`  | 轮换堡垒机注册凭据             | template    | 补导航元数据，保留业务职责 |
| connector   | `bastion.replacement.preview`       | `connector.bastion.replacement.preview`        | 替换堡垒机代理                 | template    | 补导航元数据，保留业务职责 |
| connector   | `bastion.update.preview`            | `connector.bastion.update.preview`             | 修改堡垒机范围                 | template    | 补导航元数据，保留业务职责 |
| connector   | `collector.get`                     | `telemetry.collector.get`                      | 查看采集器详情                 | template    | 补导航元数据，保留业务职责 |
| connector   | `collector.list`                    | `telemetry.collector.list`                     | 列出遥测采集器                 | template    | 补导航元数据，保留业务职责 |
| connector   | `get`                               | `connector.get`                                | 查看Connector 连接代理详情     | template    | 补导航元数据，保留业务职责 |
| connector   | `install.retry.preview`             | `connector.install.retry.preview`              | 重试代理安装                   | template    | 补导航元数据，保留业务职责 |
| connector   | `list`                              | `connector.list`                               | 列出Connector 连接代理         | template    | 补导航元数据，保留业务职责 |
| connector   | `uninstall.preview`                 | `connector.uninstall.preview`                  | 卸载连接代理                   | template    | 补导航元数据，保留业务职责 |
| dashboard   | `catalog`                           | `telemetry.dashboard.catalog`                  | 遥测数据目录                   | trace       | 补导航元数据，保留业务职责 |
| dashboard   | `catalog.resources`                 | `telemetry.dashboard.catalog.resources`        | 可用分析资源                   | trace       | 补导航元数据，保留业务职责 |
| dashboard   | `context.candidates`                | `telemetry.dashboard.context.candidates`       | 筛选候选获取                   | trace       | 补导航元数据，保留业务职责 |
| dashboard   | `context.resolve`                   | `telemetry.dashboard.context.resolve`          | 分析条件解析                   | trace       | 补导航元数据，保留业务职责 |
| dashboard   | `draft.create`                      | `telemetry.dashboard.draft.create`             | 仪表盘草稿创建                 | trace       | 补导航元数据，保留业务职责 |
| dashboard   | `draft.get`                         | `telemetry.dashboard.draft.get`                | 仪表盘草稿获取                 | template    | 补导航元数据，保留业务职责 |
| dashboard   | `draft.save`                        | `telemetry.dashboard.draft.save`               | 仪表盘草稿保存                 | trace       | 补导航元数据，保留业务职责 |
| dashboard   | `draft.validate`                    | `telemetry.dashboard.draft.validate`           | 仪表盘草稿校验                 | trace       | 补导航元数据，保留业务职责 |
| dashboard   | `get`                               | `telemetry.dashboard.get`                      | 仪表盘获取                     | template    | 补导航元数据，保留业务职责 |
| dashboard   | `list`                              | `telemetry.dashboard.list`                     | 仪表盘列表                     | template    | 修改业务契约并补元数据     |
| dashboard   | `publish.preview`                   | `telemetry.dashboard.publish.preview`          | 仪表盘发布预览                 | template    | 补导航元数据，保留业务职责 |
| dashboard   | `query`                             | `telemetry.dashboard.query`                    | 仪表盘查询                     | template    | 补导航元数据，保留业务职责 |
| dashboard   | `query.cancel`                      | `telemetry.dashboard.query.cancel`             | 仪表盘查询取消                 | trace       | 补导航元数据，保留业务职责 |
| dashboard   | `query.get`                         | `telemetry.dashboard.query.get`                | 仪表盘查询结果                 | template    | 补导航元数据，保留业务职责 |
| dashboard   | `query.resume`                      | `telemetry.dashboard.query.resume`             | 仪表盘查询恢复                 | trace       | 补导航元数据，保留业务职责 |
| host        | `collector.configure.preview`       | `host.collector.configure.preview`             | 配置主机采集器                 | template    | 补导航元数据，保留业务职责 |
| host        | `collector.install.preview`         | `host.collector.install.preview`               | 安装主机采集器                 | template    | 补导航元数据，保留业务职责 |
| host        | `collector.repair.preview`          | `host.collector.repair.preview`                | 修复主机采集器                 | template    | 补导航元数据，保留业务职责 |
| host        | `collector.uninstall.preview`       | `host.collector.uninstall.preview`             | 卸载主机采集器                 | template    | 补导航元数据，保留业务职责 |
| host        | `collector.upgrade.preview`         | `host.collector.upgrade.preview`               | 升级主机采集器                 | template    | 补导航元数据，保留业务职责 |
| host        | `create.preview`                    | `host.create.preview`                          | 创建主机预览                   | template    | 补导航元数据，保留业务职责 |
| host        | `delete.preview`                    | `host.delete.preview`                          | 删除主机预览                   | template    | 补导航元数据，保留业务职责 |
| host        | `get`                               | `host.get`                                     | 主机获取                       | template    | 补导航元数据，保留业务职责 |
| host        | `install.retry.preview`             | `host.install.retry.preview`                   | 重试主机安装                   | template    | 补导航元数据，保留业务职责 |
| host        | `list`                              | `host.list`                                    | 主机列表                       | template    | 补导航元数据，保留业务职责 |
| host        | `removal.preview`                   | `host.removal.preview`                         | 卸载主机组件                   | template    | 补导航元数据，保留业务职责 |
| host        | `update.preview`                    | `host.update.preview`                          | 更新主机预览                   | template    | 补导航元数据，保留业务职责 |
| k8s         | `cluster.create.preview`            | `kubernetes.cluster.create.preview`            | 创建Kubernetes 集群预览        | template    | 补导航元数据，保留业务职责 |
| k8s         | `cluster.delete.preview`            | `kubernetes.cluster.delete.preview`            | 删除Kubernetes 集群预览        | template    | 补导航元数据，保留业务职责 |
| k8s         | `cluster.get`                       | `kubernetes.cluster.get`                       | 查看Kubernetes 集群详情        | template    | 补导航元数据，保留业务职责 |
| k8s         | `cluster.list`                      | `kubernetes.cluster.list`                      | 列出Kubernetes 集群            | template    | 补导航元数据，保留业务职责 |
| k8s         | `cluster.update.preview`            | `kubernetes.cluster.update.preview`            | 更新Kubernetes 集群预览        | template    | 补导航元数据，保留业务职责 |
| k8s         | `collector.configure.preview`       | `kubernetes.collector.configure.preview`       | 配置集群采集器                 | template    | 补导航元数据，保留业务职责 |
| k8s         | `collector.install.preview`         | `kubernetes.collector.install.preview`         | 安装集群采集器                 | template    | 补导航元数据，保留业务职责 |
| k8s         | `collector.repair.preview`          | `kubernetes.collector.repair.preview`          | 修复集群采集器                 | template    | 补导航元数据，保留业务职责 |
| k8s         | `collector.uninstall.preview`       | `kubernetes.collector.uninstall.preview`       | 卸载集群采集器                 | template    | 补导航元数据，保留业务职责 |
| k8s         | `collector.upgrade.preview`         | `kubernetes.collector.upgrade.preview`         | 升级集群采集器                 | template    | 补导航元数据，保留业务职责 |
| k8s         | `daemonset.list`                    | `kubernetes.daemonset.list`                    | 列出 Kubernetes 守护进程集     | template    | 补导航元数据，保留业务职责 |
| k8s         | `deployment.list`                   | `kubernetes.deployment.list`                   | 列出 Kubernetes 无状态工作负载 | template    | 补导航元数据，保留业务职责 |
| k8s         | `drain.cancel.preview`              | `kubernetes.drain.cancel.preview`              | 取消节点排空预览               | template    | 补导航元数据，保留业务职责 |
| k8s         | `helm.change.preview`               | `kubernetes.helm.change.preview`               | Helm 回滚与卸载预览            | template    | 补导航元数据，保留业务职责 |
| k8s         | `helm.history`                      | `kubernetes.helm.history`                      | Helm 版本历史                  | template    | 补导航元数据，保留业务职责 |
| k8s         | `helm.list`                         | `kubernetes.helm.list`                         | Helm 发布列表                  | template    | 补导航元数据，保留业务职责 |
| k8s         | `namespace.list`                    | `kubernetes.namespace.list`                    | 列出 Kubernetes 命名空间       | template    | 补导航元数据，保留业务职责 |
| k8s         | `node_host_binding.confirm.preview` | `kubernetes.node_host_binding.confirm.preview` | 确认节点主机绑定               | template    | 补导航元数据，保留业务职责 |
| k8s         | `node.list`                         | `kubernetes.node.list`                         | 列出 Kubernetes 节点           | template    | 补导航元数据，保留业务职责 |
| k8s         | `object.change.preview`             | `kubernetes.object.change.preview`             | K8s 对象变更预览               | template    | 补导航元数据，保留业务职责 |
| k8s         | `object.get`                        | `kubernetes.object.get`                        | K8s 对象详情                   | template    | 补导航元数据，保留业务职责 |
| k8s         | `object.list`                       | `kubernetes.object.list`                       | K8s 对象列表                   | template    | 补导航元数据，保留业务职责 |
| k8s         | `operation.get`                     | `kubernetes.operation.get`                     | K8s 操作状态                   | template    | 补导航元数据，保留业务职责 |
| k8s         | `operation.list`                    | `kubernetes.operation.list`                    | K8s 操作记录                   | template    | 补导航元数据，保留业务职责 |
| k8s         | `pod.list`                          | `kubernetes.pod.list`                          | 列出 Kubernetes 容器组         | template    | 补导航元数据，保留业务职责 |
| k8s         | `pod.logs`                          | `kubernetes.pod.logs`                          | 读取 Pod 容器日志              | template    | 补导航元数据，保留业务职责 |
| k8s         | `service.list`                      | `kubernetes.service.list`                      | 列出 Kubernetes 服务           | template    | 补导航元数据，保留业务职责 |
| k8s         | `statefulset.list`                  | `kubernetes.statefulset.list`                  | 列出 Kubernetes 有状态工作负载 | template    | 补导航元数据，保留业务职责 |
| k8s         | `workbench.types`                   | `kubernetes.workbench.types`                   | K8s 类型目录                   | template    | 补导航元数据，保留业务职责 |
| log         | `query`                             | `telemetry.kql.query`                          | 查询 KQL 日志                  | template    | 修改业务契约并补元数据     |
| metric      | `overview`                          | `telemetry.overview`                           | 查看遥测覆盖                   | template    | 补导航元数据，保留业务职责 |
| metric      | `query`                             | `telemetry.promql.query`                       | 指标查询                       | template    | 修改业务契约并补元数据     |
| monitoring  | `builtin.change.preview`            | `monitoring.builtin.change.preview`            | 自身监控启停预览               | template    | 补导航元数据，保留业务职责 |
| monitoring  | `builtin.list`                      | `monitoring.builtin.list`                      | 自身监控能力                   | template    | 补导航元数据，保留业务职责 |
| monitoring  | `collection.change.preview`         | `monitoring.collection.change.preview`         | 采集实例变更预览               | template    | 补导航元数据，保留业务职责 |
| monitoring  | `collection.form`                   | `monitoring.collection.form`                   | 监控安全表单                   | template    | 补导航元数据，保留业务职责 |
| monitoring  | `collection.get`                    | `monitoring.collection.get`                    | 采集详情                       | template    | 补导航元数据，保留业务职责 |
| monitoring  | `collection.list`                   | `monitoring.collection.list`                   | 采集实例列表                   | template    | 补导航元数据，保留业务职责 |
| monitoring  | `collector.choices`                 | `monitoring.collector.choices`                 | 可用采集器                     | template    | 补导航元数据，保留业务职责 |
| monitoring  | `integration.list`                  | `monitoring.integration.list`                  | 监控能力目录                   | template    | 补导航元数据，保留业务职责 |
| monitoring  | `target.list`                       | `monitoring.target.list`                       | 监控目标列表                   | template    | 补导航元数据，保留业务职责 |
| trace       | `query`                             | `telemetry.skywalking.trace`                   | 查询调用链                     | template    | 修改业务契约并补元数据     |
| workflow    | `cancel`                            | `pending_action.cancel`                        | 取消待确认操作                 | trace       | 补导航元数据，保留业务职责 |
| workflow    | `get`                               | `pending_action.get`                           | 查看操作状态                   | action      | 补导航元数据，保留业务职责 |
| workflow    | `import_result`                     | `workflow.import_result`                       | 导入授权查询结果               | file        | 补导航元数据，保留业务职责 |
| workflow    | `plan.propose`                      | `workflow.plan.propose`                        | 提出实施计划                   | interaction | 补导航元数据，保留业务职责 |
| workflow    | `publish_file`                      | `workflow.publish_file`                        | 发布文件下载                   | file        | 补导航元数据，保留业务职责 |

## 2. 计划新增

| category/name            | 用途                                             | 状态    |
| ------------------------ | ------------------------------------------------ | ------- |
| observe/resources.list   | 按类型、家族、集成与名称定位授权对象             | planned |
| observe/capabilities.get | 获取明确对象的来源、状态、适用仪表盘与下一步引用 | planned |
| observe/catalog.query    | 发现指标、字段、值、来源和版本化推荐查询         | planned |

## 3. 顶层入口与不计入项

| 入口                                      | 用途                                  | Plan11                                           |
| ----------------------------------------- | ------------------------------------- | ------------------------------------------------ |
| tool.search / tool.describe / tool.invoke | 自有业务发现、说明与执行              | 输入字段不变，分类增加 observe；更新元数据与版本 |
| read / write / edit / bash                | 离线 Workspace 读取、生成、编辑与分析 | 保留                                             |
| connect                                   | 受控远程批次及已批准计划步骤          | 保留执行契约；整体任务范围需回归                 |
| request_user_input                        | 真实用户澄清                          | 保留                                             |
| 客户 MCP                                  | 会话所选外部能力                      | 独立接入，数量动态                               |
| 隐藏 .commit / 旧隐藏远程计划入口         | 服务端内部执行/历史边界               | 不进入模型公开目录                               |

130 + 3 = 133 是本计划初始预期，不把硬编码数量当成唯一覆盖门禁。全部条目的注册、元数据、权限、业务场景与交付方式都应生成并核对。原定四项查询/列表修改及实测追加的两项契约修正见 [契约](./03-tool-contracts.md)，业务组合见 [映射](./04-business-tool-map.md)。

来源：[目录生成器](../../scripts/tool-labels/main.go)、[生产 Worker](../../internal/app/worker/app.go)、[交互注册](../../internal/interaction/tools.go)、[身份映射](../../internal/toolgateway/gateway.go)、[展示绑定](../../internal/presentation/bindings.go)。
