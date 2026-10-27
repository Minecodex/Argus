# 主机与 Kubernetes 容器日志采集责任

## 已复现的问题与边界调整

同一物理节点同时作为 Host 和 Kubernetes Node 接入时，原来的日志冲突检查仅比较同一资源、同一集成。真实 PostgreSQL 测试已经复现：主机文件规则可以读取 `/var/log/pods`，Kubernetes 工作负载规则也可以读取同一文件；两种创建顺序均未被拒绝。这会重复采集，并绕过专用入口的 Pod、Owner 和应用归属处理。

Kubernetes 管理的标准容器日志目录 `/var/log/pods`、`/var/log/containers` 统一由 Kubernetes 日志入口负责。主机文件日志仍可配置普通目录和宽泛 glob；此边界不依赖 IP 猜测或尚未确认的 Host/Node 绑定。

## 实现规则

- 主机文件规则显式指向上述目录时，服务端拒绝创建；不留下活动采集实例。
- 主机文件 Receiver 和旧文件 Profile 的编译结果自动排除上述目录。用户配置的排除项仍然保留。
- 解析 JSON 前，使用已有 Stanza Filter 校验 `log.file.path_resolved`。普通目录中的符号链接也不能将这些容器文件引入主机采集；日志内容无法通过伪造路径属性绕过检查。
- Kubernetes 专用 Receiver 保留原有目录、Pod/Owner UID 和业务日志归属规则，不使用主机文件日志的目录防护。
- 通过共享中英文表单说明告知用户。Linux 目录限制不套用到 Windows 文件路径；Windows 实机仍按用户决定暂缓。

此修改复用现有 Collector 的配置能力，无需新增 Receiver、独立进程或重新构建 Collector 组件。

## 验证与证据边界

候选实现已通过规则与配置编译测试、真实 PostgreSQL 检查和锁定 Collector 的符号链接实测。同节点候选还运行了真实 Host 进程与 DaemonSet，验证实际 Machine ID/Boot ID、主机指标、Kubelet 指标、普通主机日志、实际 Pod UID，以及符号链接不导致容器日志重复采集。接收端是明确标注的受控组件采集端，不作为正式注册或完整发布的证明。

候选结果位于 `artifacts/plan8-kubernetes-log-path-candidate/` 和 `artifacts/plan8-node-coexist-candidate-r6.*`。正式代码合入后，仍须以最终源码重新执行对应组件、领域绑定、完整平台和发布门禁；候选结果不转换为最终版本验收。

配置启动失败回滚已经独立实测：基线通过、非法组件验证失败不更换原进程、实际监听端口冲突使候选启动失败、原配置与新鲜 CPU 样本恢复、并发申请受锁保护、新 revision 生效后旧 revision 被拒绝。测试使用制品中现有 OTLP 出口；不依赖该制品未包含的 debug 出口，不用旧日志伪装新样本。

真实 PostgreSQL 领域测试读取上述实际节点事实，经生产 Preview/Confirm 确认 Host/Node 绑定。身份变化分支明确使用受控报告故障注入：已有 Kubernetes Node 的 providerID 不能任意修改，真实 API 已拒绝该尝试；测试不修改或放松此校验，也不将受控故障声明为 Node 实际重建。该分支验证旧预览失效、Host Claim 别名撤回，以及恢复相同事实不会自动恢复旧确认。

新增用例已接入全量驱动，组件清单从 74 组扩展到 79 组。规则、配置、原生回滚、同节点物理观察、领域确认分别记录，正式门户及完整平台仍须对最终源码统一复验。

## 门户验收前件

长时间的数据库、模型与组件测试可能超过原登录 Cookie 的有效期。最后的门户阶段必须通过原有实际登录和 MFA 刷新会话后导出 Cookie，不插入会话或放宽认证。正式门户复核覆盖中英、明暗及 1280/1440/1920 三种桌面宽度，六个业务入口沿用既有 serious/critical Axe 门槛。
