# Plan8 具名验收入口

2026-10-19，新增 `argus-dev e2e-plan8`。当前覆盖已实现的组件/后端/局部门户组合；正式部署、模型、全部集成与发布阶段尚未实现，不能由此声明 T16 完成。

## 使用

```text
go run ./cmd/argus-dev e2e-plan8 --suite components --run-id plan8-components-20261019
go run ./cmd/argus-dev e2e-plan8 --suite full --list
```

| Suite | 执行范围 |
| --- | --- |
| engineering | 生成契约一致性，内部顺序执行生成与校验 |
| domain | 实际 PostgreSQL 的应用、服务、Namespace、凭据、原生工具及带类型 Workspace 来源撤权 |
| host | 实际 Linux Host/Collector、进程/systemd、同批真实数据默认查询 |
| sdk | 实际三信号 SDK、后端、原生目录工具与 real 门户；注册/颁证仍为明确夹具 |
| k8s | 实际 API/Kubelet/API Server/CoreDNS、控制面可用性/撤权/启停及同批 24 个实际默认查询；不包含完整 K8s 操作验收 |
| drain | 原生 UID/存储/只读不变量、Connector 独立写阶段和实际 PostgreSQL＋自有双节点维护工作流；另需专用 context 与 kubeconfig |
| control-positive | 接入清单真实授权、三个受保护控制面实际采样/拒绝/恢复/启停、同批十八个真实默认查询 |
| pod-logs | 游标/身份、Connector 协议、实际 PostgreSQL/HTTP、真实 Pod 当前/上次日志与重启/撤权，共五组；使用隔离 Namespace |
| resources | 原生 PostgreSQL/HTTP/TCP/证书期限、实际协议/认证/生命周期及同批 14 个查询 |
| cache-web | 原生 Memcached/Apache/HAProxy、认证/生命周期及同批 18 个查询 |
| brokers | 原生 RabbitMQ/ZooKeeper、只读/认证/生命周期及同批十二个查询 |
| components | 上述 domain、host、sdk，当前含七组具名检查 |
| full | 契约后顺序运行全部已注册组；仍因缺少正式部署/模型/支持矩阵/发布阶段返回 incomplete，退出非零 |

`--list` 仅列出范围。运行前应显式提供 `ARGUS_MONITORING_TEST_DATABASE_URL`、`ARGUS_CLICKHOUSE_TEST_ADDRESS`、`ARGUS_CLICKHOUSE_TEST_PASSWORD`、`ARGUS_MONITORING_COLLECTOR_BINARY` 和适用的 `ARGUS_JOURNALD_SYSTEMD_TEST_IMAGE`。配置值不写入 manifest；缺前件不会把 Skip 当 Pass。

k8s/full 另需 `ARGUS_KUBERNETES_METRICS_CONTEXT` 和 `ARGUS_KUBERNETES_METRICS_NODE`，当前只在明确选择的 Docker-driver 测试集群运行。控制面不可达路径通过不能代替其真实成功样本。

drain/full 另需自有双节点集群的 `ARGUS_KUBERNETES_DRAIN_CONTEXT` 与 `KUBECONFIG`。control-positive/full 使用独立的 `ARGUS_KUBERNETES_COMPONENTS_CONTEXT/NODE/API_PORT/NETWORK`；可通过自有 Kubernetes 驱动设置，避免和原不可达环境混淆。pod-logs/full 使用 `ARGUS_KUBERNETES_INVENTORY_CONTEXT` 和隔离 PostgreSQL，仅创建/清理本轮 Namespace/Pod。当前 full 注册 28 组；历史十组/十七组/二十三组注册记录只证明当时的范围，没有重写为当前二十八组同次通过。

后端限制为本机显式端口和隔离 `argus_plan8` 数据库，另核验 Docker 的 `argus-plan8-*` 名称、Plan8 owner 和仅回环端口绑定。业务库、远程地址或无法证明归属的端口在执行前拒绝。仅复用本轮自有环境，不创建或修改 Windows 管理员前件。

## 结果与失败边界

- 每次在 artifacts 中创建全新的唯一目录，保存 manifest 和先脱敏后写盘的 JSONL。没有覆盖旧证据或自动修改 progress.json。
- 记录 HEAD、已跟踪/未跟踪文件内容 Hash，以及三平台 Collector 二进制 Hash；运行前后比较，源码或制品改变即失败。证据目录本身不进入源码指纹。
- 逐项核验实际 Go JSON 事件中的具名根测试。包成功、同名子用例、无测试、跳过或另一项通过不能替代必需测试。
- 组件检查顺序执行；Host/服务采样文件在本轮目录创建，默认查询只能读取本轮新样本。SDK 组执行原有实际浏览器驱动。
- 报告明确 `plan_complete=false`、`release_eligible=false`，76 项正式基础门禁保持 not_run，并列出尚未交付的正式阶段。`passed_selected_components` 只表示所选组件组通过。
- 清理由具体自有测试驱动执行；完整受保护环境恢复/清理审计仍是发布任务，报告不自动宣称它已通过。镜像和完整生产部署的冻结尚未完成。

当前缺少的正式阶段包括：主机注册与 PKI 轮换、部署后的 Connector/Gateway/Agent/Query、全部支持组合、全部内置能力、全部 K8s 操作与会话、真实模型/Workspace/告警、迁移备份恢复及统一发布回退。Windows 实测继续按[用户决定](./28-windows-testing-deferral.md)暂缓，报告不把暂缓变成通过。

## 实际执行证据

Pod 日志专项：[最终五组](../../artifacts/plan8-pod-logs-identity-20261019-1923360225/manifest.json)与 11 个必需具名根测试通过，源码执行期间未变；真实 Pod 与注入 API 适配器、受控 HTTP/协议、mock 门户分别记录，不能合并抵扣正式部署。当前前端行为及工程检查见[47](./47-pod-log-sessions.md)。

`artifacts/plan8-components-20261019-2974773820/manifest.json`：六组组件检查、14 个必需具名根测试均通过，运行前后源码/Collector 指纹相同；实际 SDK 组包含中英明暗四个 real 门户场景。该结果不是正式 76 项通过记录。

后续十组快照 `artifacts/plan8-full-component-audit-20261019-2649792552/manifest.json`：契约和九组组件/后端/K8s 检查全部通过，17 个必需根测试，源码/Collector 指纹相同；full 正确返回 incomplete 和非零退出，正式 76 项保持 not_run。此后 Workspace/resources 已加入注册表，新增范围按各自最新证据验收，不改写旧快照。

驱动本身的跳过/缺项拒绝、分块日志脱敏、未提交源码识别、证据目录排除和后端范围约束已有单元验证，见 `artifacts/plan8-harness-unit-initial.log`。原生工具版本签名回归也在该日志中通过。
