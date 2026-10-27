# Kubernetes 终端与端口转发会话

2026-10-19，接续 [48 复核](./48-completion-audit.md)。已实现 Kubernetes 终端和 Pod/Service TCP 转发，并完成下述分层验证；T11 和 Plan8 整体仍在实施，未发布。

保留 Host 与 K8s 的独立权限、票据与审计；共享底层帧、背压和 Connector 通道，不创建虚假 Host/账号记录。

## 实现边界

- 交互用户显式创建，分别检查 kubernetes.exec / kubernetes.port_forward、当前 Cluster/Namespace UID 范围及 HTTP Session。冻结配置/接入身份/Connector epoch、Pod UID、容器 ID/重启、安全上下文；Service 转发还冻结 Service UID、端口与选定端点，不自动换 Pod。
- 主机特权 Pod 还要求 Cluster 范围及 workload.privileged；终端可能读取服务账户 Token 时，另查 service_account.use。API 接入身份不会自动授权用户使用这些能力。
- 创建发放 30 秒一次性票据；尚未连接的会话在自身有效期内可替换票据，替换立即废止旧票据。票据不出现在 URL/持久浏览器存储，CLI 使用标准输入。会话固定原 HTTP 登录与授权版本；Gateway 每秒复验授权，输入只使用至多一秒的已验证缓存，检查超时则停止。对象身份每秒检查一次，单次检查限五秒；这些检查不构成原子的零延迟撤权保证。
- 最长 15 分钟，显式凭据接入还受最长五分钟的凭据租约限制，不在当前会话中自动续租。无 I/O 五分钟结束。每用户最多十个未结束会话，关闭中的会话仍占容量。执行端原子认领，不因重放或 Peer 重试启动第二个后端；失联会话由心跳超时收口，不能重开旧 PTY。
- Direct 与 Connector 使用 client-go 原生 exec/portforward；不安装容器内 helper、不开放任意 API 代理。Gateway 副本之间使用现有 mTLS 监听上的独立 K8s Peer RPC。端口转发仅在受控组件/客户端监听回环地址，二进制多通道传输保留 TCP 字节和多连接。
- 原生 Pod exec/portforward API 没有 UID 前置条件。连接前后与期间核验；通过校验前不发送人工输入、不交付输出。无法承诺原生 shell 启动与 Pod 替换之间原子互斥，不能将此限制描述成零副作用保证。
- 人工终端与原始 TCP 不进入模型参数、聊天或 Workspace；审计保留目标、权限、生命周期和字节计数，不记录按键、凭据或业务正文。
- WebSocket 升级前保留完整 Origin 校验，升级时继续启用库的 Origin 校验并传入精确允许来源；配置值不解释为通配符。无 Origin 的 CLI、同源页面和显式允许的门户可以连接，未授权来源、协议或端口变化均拒绝；不使用跳过验证选项。

## 交付路径

| 层 | 当前实现 |
| --- | --- |
| 存储/领域 | migration 41、sqlc；独立 `kubernetes_stream_sessions`，期望连接、执行认领、实际状态、期限、字节统计与元数据审计 |
| 授权/凭据 | 仅交互用户创建；当前 HTTP Session、RBAC、Namespace UID、集群版本和接入路由复验；原加密存储与受接收方约束的凭据租约 |
| Direct | 严格内联 kubeconfig、规范 API Server、TLS、禁代理/重定向、DNS 验证与固定拨号地址；调用 client-go 原生会话 |
| Connector | Hello 中协商 `argus.kubernetes_stream/v1`，以当前连接会话能力为准；仅 Kubernetes/Bastion 角色声明，Host Hello 保持原能力；旧版本明确拒绝 |
| Peer | 现有 Gateway mTLS 监听上的独立 Kubernetes RPC 和独立请求/响应信封；再次核对 owner、epoch、fence、租约与执行认领 |
| Gateway | 一次性票据、帧序列/nonce、独立关闭、背压、有界输入输出和持续复验；状态先持久化再交付最终通知，满输出缓冲也不丢失终止原因 |
| 门户 | Pod 终端、Pod/Service 转发入口；共享 Dialog/表单/终端和受保护文本域；中英/明暗/桌面；终端历史有界，关闭清空 |
| CLI/部署 | `argusctl kubernetes port-forward`，本机回环监听，最多八个 TCP 连接，支持二进制及半关闭；企业同源 Ingress 增加独立 WSS 路由 |

四个 HTTP 接口均使用企业登录与领域授权；敏感响应 `Cache-Control: no-store`，写入要求 CSRF：

| 方法 | `/api/v1/enterprise/kubernetes-clusters/{id}` 下的路径 | 用途 |
| --- | --- | --- |
| POST | `/stream-sessions` | 创建并签发一次性票据 |
| GET | `/stream-sessions/{sessionId}` | 当前状态、固定目标和字节统计 |
| POST | `/stream-sessions/{sessionId}/ticket` | 替换尚未连接会话的票据 |
| POST | `/stream-sessions/{sessionId}/close` | 请求关闭；实际后端结束后提交最终状态与计数 |

门户不提供任意地址 TCP 代理。Service 仅接受归属于该 Service UID 的 EndpointSlice、明确的 Pod 引用和 TCP 端口，不支持 ExternalName/IP-only 端点；端点或 Service 变化不会自动转到别的 Pod。

## API 身份的权限前件

平台用户权限与 Kubernetes API 接入身份各自生效。安装器默认仍是只读身份，新增的 EndpointSlice 读取也保持只读；终端和转发不会自动把 API 身份提升为管理员。

需要启用某 Namespace 的交互能力时，由该集群的授权管理者为实际接入 ServiceAccount 授予对应 Namespace 的 `create pods/exec` 和/或 `create pods/portforward`，参考 [可选 Role/RoleBinding 示例](../../deploy/examples/kubernetes-interactive-rbac.yaml)。示例不自动应用；可删去不需要的那项权限。Direct/凭据接入同样需要其实际身份具备这些权限。

原生 API 拒绝访问会与对象身份变化、网络故障区分；不能把 API 的拒绝解释成平台已经授权或对象已重建。

## 验证与证据范围

- [冻结源码分组记录](../../artifacts/plan8-streams-delivery-20261019-3681301923/manifest.json)：`streams` 注册七组、十五个必执行根测试，要求命名测试实际通过，skip 不能顶替通过。完整 harness 从 28 组增加到 35 组，完整模式仍受其余正式门禁约束。
- 原生真实集群：独立 Namespace、Pod、Service、ServiceAccount 和受限 RBAC。只读身份的终端请求明确被拒绝；按 Namespace 授权后验证 TTY、123×41 resize、两条二进制 TCP 通道、半关闭及 Service 删除失效。按 UID/所有权清理 Namespace 与临时集群级 RBAC。
- PostgreSQL：独立权限、HTTP 登录绑定、原子一次性票据与执行认领、替换/重放、closing 容量、到期、Namespace 撤权/重建、接入 epoch/能力/版本变化、加密租约接收方和 Secret 轮换。
- HTTP/WebSocket/Peer/CLI：真实 Cookie、CSRF、四个新接口、WebSocket、TCP 连接及 mTLS Peer；校验错误 epoch、无客户端证书、重放与运行中撤权。这一层使用受控目标适配器，不能充当已部署 Connector 的端到端证据。
- 门户：四个新增中英明暗场景；另回归日志、drain、CronJob 和回滚，共 24 个场景。浏览器传输由明确的测试适配器驱动，不冒充业务数据；通过无障碍检查，验证输入/票据不进入浏览器持久存储。
- 工程验证：Go 全包、前端 1409 项、类型、lint、样式、双语以及契约校验；最终路径与返回值见[验证汇总](../../artifacts/plan8-stream-final-checks.json)。失败迭代日志保留，不能据此把整套正式验收标成已通过。

## 尚未关闭的范围

当前分层证据尚不覆盖正式安装的 Connector/Gateway/Ingress/门户完整物理链路、所有容器运行时和网络环境、持续运行中的全部 PKI/接入迁移组合。不能将前述受控适配器或原生 API 成功替代这些门禁。

Helm 生命周期、正向 CSI 扩容、Namespace 写入竞态、drain 取消模型工具及其他 Plan8 已记录缺口继续实施；专用集成、正式模型/告警/Workspace、迁移恢复和统一发布门禁未完成。76 个基础正式场景和 60 个内置能力子门禁仍保留 `not_run`，没有以本轮组件测试置为通过。Windows 实机继续按用户决定暂缓。
