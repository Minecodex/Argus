# B11b 堡垒机与隔离成员运行包

当前运行标识 `p9-b11b-20261114-01`。沿用户“继续下一阶段的验收”推进，已先归档并清理 B11a；B11a 人工结论记录为“要求推进、未明确单独确认通过”，不改写为显式 accepted。

## 实际拓扑与边界

```text
Argus Direct Executor ──SSH 安装──> B11b-Bastion-Gateway
                                          │
                      私有 SSH 安装        │ HTTPS 8445 / Gateway 9445 TLS 密文 Relay
                                          ▼
                                  B11b-Private-Linux

成员 Leaf Collector ──本机回环 / bastion_tunnel──> 堡垒机 Edge Gateway ──原生 mTLS──> Argus Ingest
```

- 两个真实 Debian/systemd/OpenSSH 容器，合计限制 3 CPU、2560 MiB。共享 Docker VM 内核；主机容量和硬件视角不代表两个独立物理机或 VM。
- 内部网络 `p9-b11b-20261114-01_inside`：`172.28.114.0/24`、`internal=true`。成员只有这一张网卡，不映射宿主机端口。堡垒机同时接入私网和借用的 kind 网络；私网公布地址 `172.28.114.2`，成员 `172.28.114.3`。
- 成员的 `control_path=bastion_relay`，SSH 执行者为 `bastion_connector`；堡垒机自身为 Direct SSH 安装与 direct 控制路径。
- 遥测明确使用 `leaf/bastion_gateway/bastion_tunnel`，并非声称直连传输；堡垒机自身为 `edge_gateway/direct_argus/direct`。后续 B11c 仍单独验证 Executor Tunnel、凭据轮换、旧预览拒绝及相关恢复。
- 网络探测从成员实际创建套接字，绕过 DNS 并使用堡垒机已证实可连接的同一平台 HTTPS、Gateway、Ingest 地址。成员均失败，堡垒机均成功且可访问成员真实 HTTP 服务，不能用错误 DNS 假装隔离。
- 本批服务 `plan9-b11b.service` 监听 18082；每 15 秒真实请求一次，负载最长 8 小时，服务和主机持续保留供查看。

## 执行阶段

在 `D:/workspace/go/Argus` 设置 `ARGUS_PLAN9_ARTIFACT_ROOT=C:/Users/23574/.codex/artifacts/argus-plan9`、`ARGUS_PLAN9_PASSWORD` 与 `PYTHONIOENCODING=utf-8`。构建缓存和 Go 临时目录使用 C 盘。

```text
python docs/plan9/runtime/B11b/run.py prepare
python docs/plan9/runtime/B11b/run.py build
python docs/plan9/runtime/B11b/run.py up
python docs/plan9/runtime/B11b/run.py network
python docs/plan9/runtime/B11b/run.py isolation
python docs/plan9/runtime/B11b/run.py bastion
python docs/plan9/runtime/B11b/run.py member
python docs/plan9/runtime/B11b/run.py collectors
python docs/plan9/runtime/B11b/run.py builtins
python docs/plan9/runtime/B11b/run.py workload
python docs/plan9/runtime/B11b/run.py remote
python docs/plan9/runtime/B11b/run.py dashboards
python docs/plan9/runtime/B11b/run.py configure_alert
python docs/plan9/runtime/B11b/run.py exercise_alert
python docs/plan9/runtime/B11b/run.py connection
python docs/plan9/runtime/B11b/run.py configure_data_alert
python docs/plan9/runtime/B11b/run.py exercise_data_pause
python docs/plan9/runtime/B11b/run.py browser
python docs/plan9/runtime/B11b/run.py delivery_watch
python docs/plan9/runtime/B11b/run.py verify
```

最终验证前运行正常 `argusctl verify --config deploy/.cache/argus-local-formal.yaml --output json --artifacts <本批目录>/formal-health`，将 JSON 标准输出保存为 `formal-health.json`；随后删除其 `formal-health/install-config.yaml` 配置副本，保留正式配置原件。

所有业务对象都走正常 API、连接测试、Preview/Confirm、安装注册与告警服务，未直接写库或构造遥测数据。选择已正常发布并验证的签名 Linux amd64 发行制品 `0.3.3-b11a-20261113`，不重复发布或修改其他架构。

## 本次安装前件修正

堡垒机代装成员时需要先下载约 72.5 MB 的签名 Connector。原实验 `/tmp` 为 64 MiB tmpfs，写满会呈现 `HOST_ONBOARDING_ARTIFACT_SIZE_MISMATCH`。保留原失败操作后改用服务专用 `TMPDIR=/var/lib/argus-connector/plan9-b11b-tmp`，0700、归属 argus-connector，位于原有可写沙箱范围。

曾尝试的 `/var/lib/plan9/...` 目录虽有普通文件权限，但不在服务 `ProtectSystem=strict` 的 ReadWritePaths 中，因此继续失败；该尝试也保留记录。最终没有降低签名/尺寸验证、没有放宽 ProtectSystem 或 ReadWritePaths。Bastion 服务启动前的固定 ExecStartPre 创建正确私有暂存目录。

沿原 Member Host ID，使用新 ConnectionTest 和正常 `preview-retry` 完成安装，不销毁重建掩盖失败。新的连接测试使用新的幂等键，避免复用已经到期的探测结果。堡垒机重新注册时保留原 Relay 地址、端口和 generation。

## 验证内容

- 8 项原生采集 healthy/fresh；逐项配置、等待真实样本后再继续，避免并发切换。
- 四套受管仪表盘分别绑定两台主机，8 个视图共 40 个非空面板；只读模板定义不复制、改写或删除。
- 实际成员服务失败触发普通业务告警，堡垒机同名服务仍正常；同一查询返回不同真实资源 ID，验证 Gateway 不覆盖 Leaf 身份。
- 短暂停止成员 Connector：观察成员控制离线与堡垒机在线，单独记录来源时间和队列；正常恢复不修改配置版本。
- 单独停止成员 Collector：两台主机控制仍在线，30 秒实际样本窗口触发真实 `no_data` 查询健康告警；恢复后健康告警恢复。`no_data` 是本次有意造成的采集暂停，未将控制在线或旧评估值当作新数据。
- 所有故障设置 110/120 秒原生自动恢复，并通过 finally 恢复；完成后服务、控制和采集均正常，测试会话关闭。
- 正常登录、严格 TLS 的实际浏览器覆盖两台主机的 Shell/OpenSSH、停靠/缩放、录像回放、告警时间线以及中英/明暗/三种桌面宽度。
- 两条持久投递路径连续观察 120 秒，无丢弃、入队失败或接收拒绝，未重置计数。正式 20 项健康与六个 PVC 身份、部署不变单独核对。

最终证据 `checks.json`、`final-preservation.json`、`member-isolation.json`、`connection-recovery.json`、`data-pause-cycle.json`、`delivery-watch.json`、`browser/` 保存在 C 盘本批目录。人工查看仍待用户确认，不自动推进。

## 后续清理

用户要求下一批后执行 `python docs/plan9/runtime/B11b/run.py retire`。先封存证据、关闭会话、退役专用授权和告警；**成员先于堡垒机**：关闭成员采集，经堡垒机新 SSH 预检及正常卸载核验后删除成员，再卸载/删除 Bastion Scope。最后撤销本批 evaluator 授权，核对 ID/标签/Service UID 后销毁 Compose 专属对象和本批遥测 Service。

保留历史会话、录像、审计、告警、共享仪表盘、发行制品、基础镜像、kind 网络及正式平台数据。未来清理脚本已提供，本轮交付不执行 B11b 退役。
