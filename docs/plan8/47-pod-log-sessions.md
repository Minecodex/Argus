# Namespace 授权的 Pod 日志会话

2026-10-19，接续 [46 完成度复核](./46-completion-audit.md)。本轮补充 T11 的日志会话，不将其等同于终端、端口转发或全部 T11 完成。

## 契约

- 会话固定企业、交互用户与 HTTP Session、授权版本、Cluster 配置版本、Namespace ID/UID、Pod UID、容器 ID/重启计数以及 previous 模式；同名重建、重启、撤权、注销和接入配置变更均拒绝继续。
- 创建发放 30 秒有效的一次性随机票据，数据库只存 SHA-256；attach 在当前 HTTP Session 中原子消费。会话最长 15 分钟，读取和关闭分别有 CSRF 与当前授权检查。普通 Cluster 范围不能代替 `kubernetes.operational_logs.read`。
- 暂停跟随及上次日志读取完成后，门户仍每两秒检查会话授权；撤权、注销或过期时清空已显示内容。GET/POST 成功响应均为 no-store。每用户最多十个有效会话，创建使用用户行锁及读已提交事务保证并发容量；票据消费和关闭使用原子条件更新，K8s I/O 不放入数据库事务。
- 当前日志和上次终止容器日志共用 Direct/Connector 原生适配器，读取前后验证真实 Namespace/Pod/容器身份。Kubernetes 日志 API 不支持 UID 前置条件，因而读取后必须再次核验，失败时丢弃全部正文，不能向用户返回同名替换对象的数据。
- 实时体验采用有界增量轮询，单次最多 256 KiB、初始时间窗和 tail 行数可调；不新增独立 exporter、外部采集服务或任意 API 代理。请求无长期服务器内存状态，可在 Server 副本间分发。
- SinceTime 在 Kubernetes 中按秒传输。签名游标保存边界秒、已读行数及原始前缀的 HMAC，私有的每会话 HMAC Key 不进入公开游标，避免根据正文摘要猜测凭据。保留相同内容的重复日志；轮转导致前缀缺失、时钟倒退或格式变化时明确终止，要求用户重新打开。末尾未完成行等待下次读取，预算截断明确提示并停止自动跟随。
- 日志正文走统一遥测脱敏规则。票据、凭据和日志原文不进入审计元数据、模型参数或浏览器持久化；公开会话不包含游标内部绑定或票据 Hash。
- Gateway 在发送每条新日志命令前，从持久会话重新检查当前用户、HTTP Session、权限与资源范围，不能仅相信命令中已有的 UID。
- 物理 API 地址固定到会话创建时的平台 Cluster 配置；Gateway 比对持久绑定，Connector 覆盖 Kubeconfig 内的 server，不能用凭据内另一地址替换所选目标。共享 `kubernetesaccess` 在 client-go 读取材料前拒绝调用者文件引用、exec/auth-provider、代理、跳过 TLS 校验和身份模拟，Direct、Connector 与远程执行复用；系统 in-cluster 的固定文件单独保留。原先引用本地文件的受管凭据需要改为内联材料，本轮未部署或改写现有凭据。
- 已关闭或失效时去掉私有连续性 Key，现有 Worker 协调器清退过期票据/会话并在到期二十四小时后移除记录；审计事件保留。此会话只承载交互运维日志，持久采集规则和模型的有界日志查询仍按各自链路验收。

## 验证状态

已执行下列验证；统一冻结运行及最终工程检查继续记录在 progress.json，不能将分层结果合并成正式部署通过。

| 范围 | 结果与证据 |
| --- | --- |
| 游标与身份 | 保留同时间重复行、秒边界续读、轮转/乱序拒绝、半行与预算、日志读取后替换 UID 不泄露正文；[单元与 Connector 协议](../../artifacts/plan8-pod-log-protocol.log) |
| Connector | 原生 TLS/Token 请求、旧 Schema 拒绝、固定 UID、凭据脱敏及类型化结果工厂；本层 API 使用受控 HTTPS 夹具，未代替正式 Connector 注册/颁证 |
| PostgreSQL/HTTP | 一次性票据、跨 HTTP Session 拒绝、篡改游标、撤权前后复验、重启/配置变化/注销/过期、十二个并发创建限制到十个、能力清退；实际 HTTP 的 Cookie/CSRF/no-store/Namespace 读取及暂停检查：[记录](../../artifacts/plan8-pod-log-http-domain-cas.log) |
| 实际 Pod | 当前/增量/上次容器日志、真实容器重启拒绝续读、统一脱敏、撤销 Namespace 授权前拒绝访问 kubelet；[首次真实执行与 UID 清理](../../artifacts/plan8-pod-log-live-first.log) |
| 门户 | 中英×明暗四场景，校验输入、打开、键盘暂停、上次日志、有界结束、关闭及存储无正文；[浏览器记录](../../artifacts/plan8-pod-log-portal-keyboard.log) |

最终 [五组冻结记录](../../artifacts/plan8-pod-logs-identity-20261019-1923360225/manifest.json)通过 11 个必需具名根测试，包含私有 HMAC、固定 API 地址、内联身份与外部引用拒绝；执行期间源码未变，plan_complete/release_eligible 均为 false。随后视觉检查修复了错误格式回调不稳定导致的重复读取，并给开关补上可见标签；[最终 24 场景门户回归](../../artifacts/plan8-pod-log-portal-final.log)增加暂停后跨轮询周期正文不变、手动读取后才增加的验证。前端证据独立记录，没有将两次源码快照伪装为同一次完整部署验收。

[最终工程检查](../../artifacts/plan8-pod-log-final-checks.json)：Go 全量、前端 1406 项、类型/lint/样式/i18n、生成契约及门户 24 场景通过。统一入口现在注册 28 组，尚未进行当前完整 28 组同次执行；正式 76 项基础与 60 个内置分项均保持 not_run。实际测试 Namespace 均已按 owner/UID 清理，argus-monitoring、docker-desktop 的 API readyz 均为 ok；没有据此声明完整受保护业务数据恢复或正式发布通过。

真实 Pod 测试仅新建带本轮 owner 的临时 Namespace/Pod，按 UID 核验后删除；未暂停原有集群或修改控制面/监听/RBAC。后端注入真实 Kubernetes API 适配器，HTTP 单独使用受控日志后端，门户使用 mock；三者不能合并声称正式注册、部署后的 Connector/Gateway/浏览器全链路已经通过。

首次验证暴露的夹具 Session 前件、SQL 参数推断、并发 Serializable 事务冲突、路由 CORS 配置、共享弹框可访问名称及开关的键盘操作分别修复，失败日志保留。生成检查曾遇 Windows Node/libuv 崩溃，修复生成物并重新完整检查后再执行 Go/浏览器验证；崩溃不计通过。

T11 仍缺 Helm、终端/端口转发、CSI 正向扩容和完整正式入口验收；drain 独立取消模型工具也仍待补。Windows 实测继续按用户决定暂缓，不对未验证的部署链路计通过。
