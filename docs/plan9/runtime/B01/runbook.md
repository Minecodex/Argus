# B01 运行包

历史运行：`p9-b01-20261026-01`，已于 2026-10-27 通过 `retire.py` 正式卸载并删除主机、停用专属凭据，再清理两个容器、两个卷和网络。证据保留，当前查看 [B02a](../../batches/B02a-review.md)。下文记录原运行流程。

## 文件与运行条件

- `compose.yaml`：两台主机，每台最多 1.5 CPU / 1280 MiB，独立 cgroup、网络和测试卷；SSH 仅映射本机 22221/22222。
- `Dockerfile`：固定 Debian 基础摘要；借用已部署 Argus remote-runtime 的公开 CA bundle 引导 HTTPS 包安装，不复制其程序或凭据。
- `workload.py`：实际 HTTP 服务/请求、CPU 哈希、内存分配、有界文件 I/O 和网络请求。没有 OTLP 生成器、模拟指标、模型或 Argus 数据库操作。
- `systemd/`：可控业务服务、保持单元可见的依赖，以及最长 24 小时的低频真实 HTTP 请求。
- `manage.py`：运行准备、Compose 校验/启动、身份检查、实际负载与故障恢复。所有主机操作先核对 run 所有权。
- `platform.py`、`scenario.py` 和 `../common/api.py`：正常 API 的接入、固定指标、关联、后台评估身份/授权、策略、故障历史与启停。
- 镜像及实际包版本见 `images.lock.json`，运行摘要在 artifacts 的 `image-lock.json`；`compose.env` 使用实际镜像 ID。
- 本机需要 Python 3、requests、Docker Compose；准备阶段只读核对 Kubernetes 工作负载/PVC。子进程采用字节输入，Windows 使用 Docker Desktop 的实际 Compose 插件入口。

systemd 容器需要 `privileged` 和私有 cgroup；不挂宿主 Docker Socket、服务目录或 cgroup 目录。观测范围仍是共享 Linux 内核与容器可见资源，不表示两台独立 VM。

## 本次已执行的启动方式

从仓库根目录运行。`prepare` 用于新 run 的准备，不用于覆盖已完成接入的状态。

```powershell
python docs/plan9/runtime/B01/manage.py prepare --run p9-b01-20261026-01
docker build --tag argus-plan9-host:b01-20261026 --file docs/plan9/runtime/B01/Dockerfile docs/plan9/runtime/B01
python docs/plan9/runtime/B01/manage.py up --run p9-b01-20261026-01
python docs/plan9/runtime/B01/manage.py status --run p9-b01-20261026-01
```

主机 root 密码随机生成并存放在本次 `private/host-credentials.json`，目录权限仅当前用户与 SYSTEM；不在命令行、仓库或报告展示原值。后续接入通过正式 Secret/Credential API 登记引用，不写业务表。

## 实际接入与 API 执行

平台所在集群至 `host.docker.internal:22221/22222` 的真实 TCP 探测已通过；专用探测 Namespace 已删除。正式 API 的 HTTPS 使用当前 `argus-trust-bundle` 验证通过，旧本机缓存 CA 未复用。

旧数据和账号已按用户指令[重建](../../05-local-data-reset-20261026.md)，平台 `admin` 与企业 `kakj` 均通过正常 API 和实际门户登录。会话位于受保护的 artifacts/private，过期时正常登录；凭据通过 stdin 输入，不写入脚本/文档。

已执行：本批 SSH 凭据→连接测试→Host 预览/确认（`executor_tunnel`）→Collector（`direct_argus` / `executor_tunnel`）→四类采集→视图关联→专属后台评估权限/策略→实际故障/负载/恢复→启停→门户 E2E。资源 ID、版本和时间记录在本次 artifacts。

`platform.py` 支持 `onboard`、`collectors`、`builtins`、`dashboards`、`alerts`、`exercise`、`lifecycle`、`retain` 和 `review-scenario`，需要 `--session` 与 `--ca`。`review-scenario` 顺序验证并保留故障，结果在 `scenario-checks.json`；恢复记录从 history 读取并绑定本次 firing 实例 ID。

## 真实负载与故障命令

负载在接入和策略基线准备后执行，默认 120 秒，系统服务额外保留 30 秒退出保护。本次 CPU 为 120 秒，其余各 60 秒。四种 mode 为 `cpu`、`memory`、`disk`、`network`，一次一种。

```powershell
python docs/plan9/runtime/B01/manage.py load --host host-a --mode cpu
python docs/plan9/runtime/B01/manage.py fault --host host-a
python docs/plan9/runtime/B01/manage.py recover --host host-a
```

`fault` 只令本批 HTTP 服务真实退出，SSH/systemd/Connector 不受影响。自动化进程每两秒向另一台主机请求一次，24 小时后停止增长性流量；等待用户不进行持续资源压力。服务输出进入真实 journal。

## 清理

用户要求销毁时先按 Plan9 正式生命周期完成平台资源收尾，再核对 `resource-map.json` 与 Docker 当前标签/ID，清理该 Compose 项目的容器、两个卷和网络。保留审计与证据，不清理正式平台或全局镜像缓存。本次销毁已执行，`retire.py` 支持完成状态的重复核验。

Compose 字段依据 [Docker 官方服务规范](https://docs.docker.com/reference/compose-file/services/)；本机 Compose v5.5.1 渲染检查与实际启动已通过。
