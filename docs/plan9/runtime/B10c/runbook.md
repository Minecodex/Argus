# B10c：Helm、扩展资源、RBAC 和节点维护

当前运行标识：`p9-b10c-20261113-01`。本批从用户“继续 plan9 下一轮检查”启动；只清理已核对的 B10b，正式 Argus 持续运行。

Helm 沿用当前 API/CLI 能力验收。门户检查 Helm 生成的工作负载、真实 CRD/扩展实例、RBAC、节点、日志/终端、仪表盘及告警。CRD 是独立集群的基础设施前件，实例 CRUD 和受验维护动作均走正常平台预览/确认。

## 固定场景

- 独立 Kind v0.32.0、Kubernetes v1.36.1 的一个控制节点和两个 worker，固定节点镜像摘要沿用运行脚本。内存分别限制为 3/2/2 GiB，CPU 每节点 2 核；Kind 共用 Docker VM，节点报告容量不是新增物理硬件。
- 独立 TLS/Basic Helm 仓库和 Distribution Registry 3.0.0，固定镜像摘要、真实 `.tgz` 和原生 ORAS SDK 推送。仅挂载本批源认证及 TLS 文件，不挂载平台会话或管理 kubeconfig。本机发布端使用仅绑定 `127.0.0.1` 的 23443/23500 端口，平台源地址使用独立 Docker 私网 HTTPS 地址。
- 仓库预览后关闭真实源，再确认验证冻结 Chart；仓库与 OCI 的实际安装、升级、回滚、卸载、Hook 进程日志。保留 `b10c-repo` 和 `b10c-oci` 供查看。
- `checks.b10c.argus.test` 的实际 CRD 和实例；实例创建/编辑/删除。无控制器的 CR 只展示实际配置和未知状态，不写造假的健康 status。
- 命名空间内 Role/RoleBinding/ServiceAccount 的实际配置；读权限变更及拒绝提权。管理凭据只允许修改一个专用 worker，控制节点和系统工作负载写入保持拒绝。
- 通过正常平台动作完成 cordon、PDB 阻塞、预算恢复后的真实 Eviction/迁移、uncordon、标签与临时污点。Collector 网关及其持久状态先放在控制节点，避免把其本地卷作为可丢弃数据处理。
- 低频真实 HTTP 请求最多运行 8 小时；可用副本告警使用真实 `2 → 0 → 2` 变更及恢复。120 秒 UID/副本前置条件保护定时器兜底恢复，不向外部联系人发送通知。

## 执行

Python 依赖为本机已有的 requests、PyYAML、cryptography、bcrypt；预检记录实际版本。大文件放 C 盘，运行时通过 `ARGUS_PLAN9_ARTIFACT_ROOT` 指定绝对产物根目录。密码通过 `ARGUS_PLAN9_PASSWORD` 传入，用户名默认 `kakj`；不写入源码或日志。

按顺序运行：

```powershell
python docs/plan9/runtime/B10c/run.py prepare
python docs/plan9/runtime/B10c/run.py preflight
python docs/plan9/runtime/B10c/run.py transition
python docs/plan9/runtime/B10c/run.py sources
python docs/plan9/runtime/B10c/run.py cluster
python docs/plan9/runtime/B10c/run.py bootstrap
python docs/plan9/runtime/B10c/run.py foundation
python docs/plan9/runtime/B10c/run.py helm
python docs/plan9/runtime/B10c/run.py extensions
python docs/plan9/runtime/B10c/run.py nodes
python docs/plan9/runtime/B10c/run.py monitoring
python docs/plan9/runtime/B10c/run.py publish-web
python docs/plan9/runtime/B10c/run.py verify
python docs/plan9/runtime/B10c/browser.py
python docs/plan9/runtime/B10c/delivery_watch.py
python docs/plan9/runtime/B10c/run.py final-checks
python docs/plan9/runtime/B10c/run.py handoff
```

发生未知结果或部分执行失败时，先查询已保存的操作 ID 和原生对象，不直接重跑整个写入阶段。预览/确认的幂等键、UID、Namespace 身份、版本与服务端授权保持生效。

本次使用 `ARGUS_PLAN9_ARTIFACT_ROOT=C:/Users/23574/.codex/artifacts/argus-plan9`。部署后的正式健康检查保存为本批 `formal-health-after-fix.log`；交付阶段要求它的 20 项检查、API 自检、真实页面和至少 305 秒网关投递观察全部通过。投递观察读取实际网关持久账本，并检查三项指标采集的时间戳持续推进；不代表逐一核对所有节点 Agent 的账本。

正式健康复验使用现有 `go run ./cmd/argusctl verify --config deploy/.cache/argus-local-formal.yaml --output json --artifacts <本批产物目录>/health-after-fix`，将 JSON 标准输出保存为 `formal-health-after-fix.log`；构建缓存和临时目录指向 C 盘。核对后清除产物中的 `install-config.yaml` 配置副本，保留正式原配置。`final-checks` 对照基线检查六个 PVC、其他部署、实际 Pod imageID、冻结生产源码、节点恢复及采集状态，并生成 `final-preservation.json`。

真实结果、查看路线、修复版本及环境限制见 [B10c 查看记录](../../batches/B10c-review.md)。本批保留运行；会话退出后再次执行只读检查时，通过环境变量提供正常登录凭据即可重新登录。不要直接重跑创建、Helm 生命周期或故障阶段。

同批前端修复使用新的 `ARGUS_PLAN9_WEB_TAG` 创建独立冻结目录和不可变镜像，不覆盖旧制品；当前为 `local-20261113-b10c-scope-error-v2`，详见 [范围提示反馈](../../21-b10c-namespace-scope-error-20261113.md)。`final-checks` 读取最新发布记录指向的源码目录。真实权限拒绝应按页面引导显式选择 Namespace，不为了默认全部范围可读而扩大测试凭据权限。

每个节点的 kubelet 使用本集群签发的 serving 证书并显式配置公开 CA。仅在本批 API Server 设置最大 168 小时令牌时效，读取真实签发过期时间并记录；不改变原生权限。配置格式及上限语义来自 [Kubernetes kubeadm v1beta4](https://kubernetes.io/docs/reference/config-api/kubeadm-config.v1beta4/) 和 [API Server 参数](https://kubernetes.io/docs/reference/command-line-tools-reference/kube-apiserver/)。

## 清理

只有用户要求下一轮或销毁时执行 `run.py retire`。先停用本批策略，正常卸载 release/删除受验对象并收回源，再按 run 标签、容器 ID、Namespace/CRD UID 清理独立 Kind 和 Compose。保留审计/告警历史、借用镜像缓存及正式服务和六个 PVC。清理后同时核对平台与原生环境。

## 资源列表趋势专项

运行 `trends_review.py fixtures` 通过正常平台预览/确认准备三类真实 Pod，`trends_review.py check` 核对 18 组实际指标，`trends_browser.py` 验证独立加载与真实延迟/中断；结束后使用 `trends_review.py cleanup` 按 UID 清理临时 Pod。运行前先检查现有 `resource-trends-v1/fixtures.json`，保留旧身份与清理记录，不覆盖活跃场景。当前前端为 `local-20261113-b10c-resource-trends-v5`，后端为 `local-20261113-b10c-resource-trends-v2`；详细证据见 [专项记录](../../22-b10c-progressive-resource-trends-20261113.md)。
