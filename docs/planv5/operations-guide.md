# PlanV5 运维手册

## 安装边界

`argusctl` 安装配置的 `workspace` 段开启专用文件存储，`openSandbox.enabled` 独立控制计算运行时。没有计算 Profile 时，文件上传下载仍由 workspace-io 完成；模型不获得不可用的基础执行工具。

P5 的 agent-lite 安装明确设置 `openSandbox.enabled=false`，检查计算服务不存在后验证上传、读取和工具调用；随后在同一 release 启用 OpenSandbox，复用原数据库、文件存储和签名身份继续 agent-sandbox。evaluation/local-hardening 仍表示部署资源与加固等级，不与这两个能力模式混为新的部署 profile 枚举。

对象存储镜像同时包含 MinIO 服务和从固定官方源码提交构建的 `mc`，桶初始化与对象读写验收复用该镜像及配置的拉取策略，不再运行时拉取 `minio/mc`。Client 源码版本和提交锁定在 `deploy/versions.lock.yaml`；不会因为上游镜像失效而自动升级或改用第三方重打包版本。

默认 StorageClass `argus-workspace` 不是集群默认类。固定 RawFile LocalPV 0.15.1，使用 ext4、thick、`nodiscard`、RWO 和 `WaitForFirstConsumer`，关闭扩容、快照与非必要 API Server。安装器校验 chart SHA-256。不得把 local-path 的声明容量当作硬额度。

生产将 `/var/local/argus-workspaces/data` 与 `/var/local/argus-workspaces/meta` 放在专用持久磁盘路径。此存储为节点本地，不能承诺跨节点复制或节点永久损坏后的无损恢复，也不能宣称 Workspace HA。

OpenSandbox 使用固定 Controller；Argus 的 sandbox chart 管理 server 配置、template 文件、Deployment 和命名空间权限。不得改为 Pool 模式。Workspace Pod 创建必须经过 server 的 admission：非 root 用户代码、只读根文件系统、禁止提权、capability 限制和进程上限。

受信任的 egress sidecar 采用 `dns+nft` 和镜像中的全拒绝 overlay。管理令牌不注入用户容器。NetworkPolicy 是补充措施，不能代替 DNS、IPv4、IPv6 和集群地址的实测。

## 平台默认配置

| 项目 | 默认值 |
| --- | --- |
| 单 Workspace 容量 | 2 GiB |
| 企业预留池 | 20 GiB |
| 上传/交付上限 | 100 MiB |
| 计算闲置 TTL | 15 分钟 |
| 命令默认/最大时长 | 60 / 300 秒 |
| 每 Run 模型调用 | 24 次 |
| 用户进程上限 | 256 |
| 运行时 | Python 3.12、Node 24、bash |

镜像 Digest 和依赖锁见 `deploy/versions.lock.yaml`、`deploy/workspace/requirements.lock`。平台管理员在 Sandbox 管理页配置受信任 backend、锁定 Digest 的分析镜像及包含 `agent_workspace` 的 Profile；企业管理员仅管理客户 MCP。

## 生命周期与排障

Workspace 与计算实例是两个状态机。实例回收不会删除 PVC。新的空闲复用实现已通过 P5 g 验收：每次操作结束先撤销用户进程和旧文件 RPC，只有确认清理完成才保留实例；下一次租约提升 fence 后复用。空闲 TTL 默认 15 分钟，平台 Profile 的有效期和企业计算额度可使实例更早回收。回收失败、版本变化或监督进程不可用时撤销整个工作负载，不使用旧实例继续写入。满额时报告同步容量错误，保留现有文件，不做 LRU、自动扩容或旧文件清理。

计算镜像内的受信任 Supervisor 使用独立非 root UID 10001，仅持有 SETUID/SETGID/KILL，以便把用户命令切换到该 Workspace 的非 root UID 并撤销全部残留进程。命令进程清空 capabilities、采用 no-new-privileges 和固定环境，不继承管理凭据。Supervisor 私钥位于只允许管理 UID 遍历的只读目录；用户进程与管理进程 UID 不同，文件 IO 仍在独立 PID/挂载命名空间的固定 RPC 容器运行。复用和接管同时要求进程清理与文件 RPC drain 成功，不能仅凭数据库 lease 或 RWO 放行。此实现的新增隔离边界须通过 Linux 进程/密钥访问与 Kubernetes 复用验收后才能标记完成。

显式删除先写入删除意图、取消该会话未完成的 Agent Run，等待/撤销旧写入者，再删除 PVC、PV 对应数据和不可变交付对象，最后归还预留容量。失败由 Reconciler 重试。不能因 lease 到期就强行创建第二个写入者。

优先检查：Workspace 状态和 fence、对应 Pod/BatchSandbox 的所有权标签、PVC/StorageClass、admission 拒绝原因、IO 证书、RawFile 节点日志及额度记录。故障诊断不得打印客户 MCP 凭据、提交令牌或私有结果。

`result_unknown` 是外部请求已可能发送但结果无法确认的终态。停止该 Run 自动推进，不能通过重启 Worker 或清 Redis 自动重放写请求。再次执行需要新的用户意图。

异常接管边界：只有正常释放租约的 Pod 可以热复用。领取使用观察到的 fence 做条件更新，过期 owner 的实例必须物理撤销后重建。旧 owner 收尾前再次核验并延长当前租约；已经失去租约时不再执行 Kubernetes 回收，避免晚到清理误删新 owner 的实例。

## 验收与清理

P5 使用全局 E2E Lease、独立临时 Namespace 和独立测试域名。若暂停正式 Argus 服务，先记录原副本数，结束时恢复；不固定恢复为 1，不操作 Agentx 或未知项目的数据。

测试可使用独立 IngressClass 和 Controller，通过端口转发保持真实 TLS/Origin；不占用已有项目的入口：

```powershell
$env:ARGUS_E2E_ISOLATED_INGRESS='1'
go run ./cmd/argus-dev e2e run --suite p5 --kube-context your-context
```

清理只处理本次持有的 Namespace、PVC、Pod、Artifact、RBAC 和 Lease。卸载本次创建的 RawFile 组件前确认其卷已回收；若服务其他 Namespace，保留共享驱动。验收报告必须记录实际通过/未通过项、脱敏证据、清理和正式服务恢复结果。

安装前要求工作目录所在磁盘至少有 25 GiB 可用空间；不足时停止并报告，不自动执行全局 Docker/BuildKit prune。共享缓存不能推断为本次测试所有。

## 中文真实模型评测

完整 P5 的确定性 Replay 用于协议和故障验收，不能代替模型任务成功率。额外使用 `--real-model-config <本地 JSON 路径>` 启用六个自然语言任务：三个业务域的空目录查询、附件 CSV 汇总和不可变交付、同会话下一 Run 的文件复用、客户 MCP 只读查询。

配置示例（端点、模型、窗口和价格必须替换成实际配置）：

```json
{
  "base_url": "https://model.example/v1",
  "model_id": "your-model",
  "api_protocol": "chat_completions",
  "api_key_env": "ARGUS_BENCHMARK_API_KEY",
  "context_window_tokens": 32768,
  "max_output_tokens": 4096,
  "input_price_per_million": 0.1,
  "output_price_per_million": 0.2,
  "monthly_amount": 5
}
```

运行进程通过指定环境变量读取凭据；配置不接受明文 `api_key`，不得把密钥写进命令行或验收证据。支持 `chat_completions` / `responses`，只接受 HTTPS，明确拒绝 Argus Replay。该开关仅用于 P5 集群套件，运行前检查配置，不能在 unit-only 下假装完成真实评测。

```powershell
go run ./cmd/argus-dev e2e run --suite p5 --kube-context your-context --real-model-config D:/private/argus-benchmark.json
```

`p5-real-model-statistics.json` 与 Replay 统计分开，记录逐任务成功、Run 状态、实际输入/输出及总 Token、模型调用次数、推理轮次、总时长和首个成功非发现类 Tool 结果延迟；没有结果时延迟为 null。统计只含任务 Run，模型兼容性探测另计。Provider 未提供完整用量或存在未结算调用时 `usage_complete=false`，所列 Token 只代表已记录部分，整体评测不能通过。成功要求权威调用事实或实际下载 CSV 的计算结果符合预期，不依靠模型自报成功。六个样本只构成功能基线，不代表广泛业务任务准确率；前三项为空目录检索，不代表大规模业务数据质量。真实运行缺失或失败时整体评测门禁仍未通过。

完整性依据 ModelCall 每个方向的 `provider` 来源、成功终态以及关联额度记录已经按 `provider` 来源结算；不再以数值大于零代替完整性。压缩和推理的估算值不会进入真实评测 Token 合计，Provider 明确返回零仍是有效实报。P5 的缺 usage 故障场景须验证该门禁拒绝，并与实际模型评测分开记录。

Workspace 导入来源失去授权后，目录会拒绝离线执行和文件访问。不要通过修改来源 Scope、清空来源集合或重置授权版本恢复访问；文件仍保留，可以按原入口明确删除 Workspace。来源集合在导入写入前持久化，失败导入也可能留下保守限制，以覆盖写入成功但元数据提交中断的情况。

`MCP_RESPONSE_CREDENTIAL_EXPOSED` 表示上游返回数据包含本次连接认证值，整个信封已被阻止使用。检查上游工具描述、Schema、返回正文和诊断行为，不关闭检查或把认证值写进排障工单。已发送的工具调用会按未知结果处理，需先核对业务状态，不能自动重发。

缓存输入 Token 已包含在普通输入总量中，不能再次相加；`cached_usage_complete=false` 表示部分调用未提供该细项，只能展示已报告部分。排查历史模型请求应使用 ModelCall 的 `context_snapshot_id/context_snapshot_hash`，不要用当前活动摘要代替；无摘要输入时两字段均为空。


`MODEL_USAGE_INVALID` 表示模型最终用量自相矛盾（包括跨更新的缓存数超过输入总量）。该响应的工具不会执行；Run 或压缩任务停止自动推进。诊断计数的来源为 `invalid`，不进入实报/估算 Token 合计；额度保留原预留估算，不能当作供应商实际账单。排查 Provider 的原生用量返回，不将这些值改成 Provider 实报以通过完整性门禁。P5 的 `p5-usage-consistency.json` 单独验证推理阻断、压缩不发布及无自动重试。
