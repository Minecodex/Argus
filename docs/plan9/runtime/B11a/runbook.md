# B11a Linux 主机直连运行包

运行标识 `p9-b11a-20261113-01`。用户明确通过 B10c 后，先归档并退役 B10c，再创建本批独立 systemd Linux 主机。下一批为 B11b；本包不会自动推进。

## 环境与边界

- `compose.yaml` 启动一个真实 Debian/systemd/SSH 主机。平台通过 `172.31.0.7:22` 进行 SSH 连接测试和安装，主机 `control_path=direct`。
- Collector 使用 `direct_argus + direct`。本批 Service `argus-system/plan9-b11a-telemetry` 仅暴露原生 4317/4318；主机按实际 LB 地址解析已有证书中的服务名，端到端 TLS 不经过解密代理或隧道。
- 容器限制 1.5 CPU、1280 MiB、512 PID；systemd 需要 privileged 和私有 cgroup。共享 Docker VM 内核，hostmetrics 的内存、磁盘/CPU 口径可能反映共享 VM；不是独立物理机或独立 VM 容量验收。
- `127.0.0.1:22361` 仅作本机排障入口。正式平台访问使用 Docker 网络地址，SSH 密码随机生成，保存于 C 盘 private 目录，平台通过正常 Secret/Credential 管理。
- 实际服务 `plan9-b11a.service` 监听 18081；低频请求每 15 秒一次，后台负载最多 8 小时。服务持续运行，负载结束后仍可实时查询和连接终端。
- root 托管账号明确标为 administrator。授权仅关联本批主机、本账号和 `kakj`，允许 Shell/OpenSSH 终端，有效至 2026-11-20 22:31:41 +08:00。复用系统默认强制录像/命令审计，不新增全局访问规则。
- Shell / PTY 的实际执行身份为 `argus-connector`；OpenSSH 为 `root`。本批不测试临时调试容器、Windows、RDP、堡垒机或隧道。

## 执行

在 `D:/workspace/go/Argus` 使用 PowerShell，设置 `ARGUS_PLAN9_ARTIFACT_ROOT=C:/Users/23574/.codex/artifacts/argus-plan9`、`ARGUS_PLAN9_PASSWORD`、`PYTHONIOENCODING=utf-8`。如运行 Go 命令，将 `GOCACHE` 和 `GOTMPDIR` 指向 C 盘。

```text
python docs/plan9/runtime/B11a/run.py prepare
python docs/plan9/runtime/B11a/run.py build
python docs/plan9/runtime/B11a/run.py up
python docs/plan9/runtime/B11a/run.py network
python docs/plan9/runtime/B11a/run.py enroll
python docs/plan9/runtime/B11a/run.py workload
python docs/plan9/runtime/B11a/run.py remote
python docs/plan9/runtime/B11a/run.py dashboards
python docs/plan9/runtime/B11a/run.py configure-alert
python docs/plan9/runtime/B11a/run.py exercise-alert
python docs/plan9/runtime/B11a/browser.py
python docs/plan9/runtime/B11a/delivery_watch.py
go run ./cmd/argusctl verify --config deploy/.cache/argus-local-formal.yaml --output json --artifacts <本批目录>/formal-health
python docs/plan9/runtime/B11a/run.py verify
```

正式健康命令的 JSON 输出保存为本批 `formal-health.json`；检查后删除其 `formal-health/install-config.yaml` 配置副本，保留正式配置原件。浏览器测试正常登录，真实 API/PTY，不伪造返回或跳过证书验证。

本次遇到旧主机 Collector 安装包与当前配置不兼容，已按[签名发布记录](../../23-b11a-direct-host-artifact-20261113.md)发布 Linux amd64 `0.3.3-b11a-20261113`。这是一次产品制品发布，正常复验不重复上传/签名。`artifact_release.py` 保存校验、上传和 Catalog Job 证据，不作为每次部署必跑步骤。

如果首次安装尚未生成身份便失败，不能用普通 repair 来补签初始身份。本次保留失败记录后，使用 `integration.py reinstall_failed_activation` 的正常卸载/重装流程。它只接受 `degraded + effective_revision=0`，不得用于已成功注册的 Collector。先排除实际错误再执行，不循环重试。

## 真实检查

- 四类原生来源均 healthy/fresh、Collector desired/effective revision 相同。
- Host、Host processes、Linux system services、Collector 共 20 个非空面板。
- 实际 Shell/OpenSSH 输出 `hostname`、`id`、只读进程列表；结束后会话终止，加密录像可读取和播放，保留创建及凭据使用审计。
- 通过 `fail-start` 使真实 systemd 服务启动失败：指标 0→1→0，告警触发/恢复。120 秒原生定时恢复与 finally 双重收尾，不让测试故障留在现场。
- 对比正式部署/PVC 基线、旧发行版本及新二进制摘要；正式平台健康与本批证据分别保存。
- 启动配置切换期曾发生 SOURCE_REJECTED，1999 个历史丢失指标点保留；验证业务故障已恢复，并通过持久计数序列观察后续没有新增丢弃。不得把历史缺口清掉来满足“零告警”，本轮不宣称配置切换根因已修复。

产物在 `C:/Users/23574/.codex/artifacts/argus-plan9/p9-b11a-20261113-01/`。`checks.json` 是最终门禁，`browser-review.json` 只有全部真实用例成功才写 passed。

## 用户要求下一批后清理

```text
python docs/plan9/runtime/B11a/run.py retire
```

先终止本批活动会话、停用并归档专用授权、停用托管账号，再走正常告警退役、关闭采集、SSH 卸载核验、主机删除、凭据停用和 Secret 撤销。通过运行标签、容器 ID、Service UID 验证归属后删除 Compose 专属容器/卷/网络及本批遥测 Service。

保留会话、录像、审计、告警历史、已发布签名制品/发行版本及共享 evaluator、kind 网络、基础镜像、正式 Argus 数据。最终比较六个正式 PVC 和部署基线。当前交付不会执行本批退役命令。
