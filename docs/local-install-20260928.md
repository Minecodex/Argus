# 本地正式构建安装记录（2026-09-28）

**本记录已被后续部署替代：Argus 当前位于 Docker Desktop 自带的 `docker-desktop` 集群，独立集群已不存在。现行入口、配置和证据见 [Docker Desktop 安装记录](./docker-desktop-install-20260928.md)。以下保留此前安装与清理过程。**

用途：保留一套真实 API、数据库与遥测服务，供人工点击验收。基于 `9dd209b` 与本轮审计测试类型修复、可配置 Template Host；镜像使用正式构建，未启用 E2E build tags。安装采用单节点 evaluation 配置，不能称为高可用 production Profile 已开放或通过生产隔离验收。

## 入口与初始化

- 企业门户：<https://argus.dev>
- 平台管理与首次初始化：<https://platform.argus.dev>
- 工具卡片：<https://cards.argus.dev>
- 产物下载：<https://artifacts.argus.dev>
- Connector：`grpcs://argus.dev:9443`。

未创建测试账号、企业或模型配置。先使用 `artifacts/local-install-20260928/initialize-platform.url` 或同目录 `setup-link.txt` 中的初始化链接设置平台管理员，再管理企业和企业管理员。首次链接有效至北京时间 2026-09-29 19:34，初始化完成后失效；文件仅当前 Windows 用户可读，不应提交或分享。

安装开始时本机原有 hosts 映射覆盖四个业务域名。管理员方式添加 `templates.argus.dev` 未成功，最终配置采用 `cards.argus.dev`。用户手动编辑 hosts 后，复核只匹配到 `templates.argus.dev`，因此还需保留其他项目内容并补全以下一行；已通过显式 loopback 地址验证服务端入口，普通域名访问仍取决于此本机解析配置：

```text
127.0.0.1 argus.dev platform.argus.dev cards.argus.dev artifacts.argus.dev templates.argus.dev
```

安装器新增可选 `spec.exposure.templateHost`，统一驱动证书、Ingress、允许的 Origin、前端运行配置与自检，默认域名保持不变，并拒绝与门户/产物同源。

## 部署位置与维护

- 独立本地 kind 集群：`argus-local`，上下文 `kind-argus-local`。现有 Docker Desktop 集群的 Judex OpenSandbox 控制器镜像不满足 Argus 锁定版本，因此独立部署，没有更改它的控制器或 CRD。
- kubeconfig：`artifacts/local-install-20260928/kubeconfig.yaml`。默认 kubectl 上下文仍为 `docker-desktop`。
- 安装配置：`deploy/.cache/argus-install-local.yaml`。
- 业务命名空间：`argus-system`、`argus-observability`、`argus-sandbox`；另有入口、Workspace 存储和证书命名空间。
- 本地节点限制 8 GiB，保留持久卷；节点、镜像仓库与限定只发现 Argus 集群的 LB 辅助容器随 Docker 自动启动。
- HTTPS CA 已加入当前用户 Root 信任库；公开 CA 位于上述 artifacts 目录。端口 80/443/9443 绑定本机 loopback。

维护命令在仓库根目录运行，仅在当前 PowerShell 进程设置独立 kubeconfig：

```powershell
$env:KUBECONFIG = Join-Path (Get-Location) 'artifacts/local-install-20260928/kubeconfig.yaml'
./build/argusctl.exe status --config deploy/.cache/argus-install-local.yaml --output json
```

不要删除该集群或运行 `uninstall --delete-data`，否则会丢失之后人工创建的数据。用户要求清理的 Argus 旧测试 Namespace 在部署前已不存在；其他项目带 e2e 名称的 Namespace 保留。

## 验证边界

完整前端类型检查、审计测试、argusctl 测试和契约测试通过。服务部署自检首次为 19/20，OpenSandbox 冷启动拉取镜像超时；镜像就绪后复验 20/20，失败探测 Sandbox 按用途标签、UID 和 resourceVersion 核对删除。最终 cards 域名配置更新后再次 **20/20 通过，退出码 0**，证据为 `artifacts/local-install-20260928/verify-cards/verify.json`。安装结束后删除了核验归属的临时镜像加载 DaemonSet；节点缓存镜像与持久卷保留。无 Argus 测试 Namespace 或探测 Sandbox 残留。

首次安装时，共享集群原有 11 个 PVC 的 UID 和状态核对无变化。未暂停其他项目服务。NetworkPolicy 未证明执行 deny、未配置外部 Egress Gateway、沙箱共享容器运行时的本地限制保留。初始化与真实业务点击测试交由用户完成；安装自检不能替代 PlanV2 当前整版浏览器及模型联合验收。

## 后续按用户明确范围清除 Agentx 测试部署

用户随后明确选择“只部署 Argus，移除 Agentx 测试部署”，并追加删除 `agentx-deps`。已通过 `agentxctl uninstall --target all --purge-data --yes` 卸载旧部署，使用当前 Helm Values 固定三个测试 Namespace；额外的 `agentx-deps` 核对归属后通过带 UID/resourceVersion 前置条件的请求删除。

- 已删除：`agentx-e2e-control-pvtest01`、`agentx-e2e-deps-pvtest01`、`agentx-e2e-runtime-pvtest01`、`agentx-deps`。
- 9 个 Agentx PVC 及对应 PV 全部回收，相关 Helm Release、IngressClass 和集群级资源无残留。
- Judex 两个 PVC UID 未变，Deployment/StatefulSet 全部 Ready；共享证书服务及集群公共组件保留。
- Argus 独立集群仍使用 `argus-system`、`argus-observability`、`argus-sandbox` 等正式名称，安装状态 `ready=true`，严格 TLS 的 HTTPS 健康请求返回 200。默认上下文仍是 `docker-desktop`。
- 清理证据：`D:/workspace/rust/Agentx/.local/artifacts/cleanup-20260928-argus-only/result.json`；Argus 状态：`artifacts/local-install-20260928/status-after-agentx-cleanup.json`。
