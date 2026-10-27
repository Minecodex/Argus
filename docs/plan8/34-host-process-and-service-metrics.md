# 主机进程与系统服务监控

2026-10-19，在[完成度审计](./32-implementation-audit.md)之后补充实现。Linux 原生采样和默认查询通过；Windows 实机按用户决定暂缓，T09 和全计划仍未整体验收。

## 实现范围

新增三个独立固定实例：`host_process_metrics`、`host_systemd_metrics`、`host_windows_service_metrics`。它们复用 Host 权威身份、自身能力列表、版本/启停/状态、权限、原生工具与默认模板，不创建另一份监控目标。

| 固定能力 | 内置实现 | 平台和采集范围 |
| --- | --- | --- |
| 主机进程 | 已有 hostmetrics 的 process；Linux 另有 processes | Linux amd64/arm64、Windows amd64；只读取 Collector 身份可见的进程 |
| Linux 系统服务 | upstream systemdreceiver 0.158.0 | Linux amd64/arm64；通过系统 D-Bus 读取已加载的单元，CPU 指标还要求可读 cgroup v2 |
| Windows 系统服务 | upstream windowsservicereceiver 0.158.0，运行类型 windows_service | Windows amd64；按 Collector 身份读取服务状态，本轮没有实机验证 |

来源核对：[Host metrics](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/v0.158.0/receiver/hostmetricsreceiver)、[Systemd](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/v0.158.0/receiver/systemdreceiver)、[Windows service](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/v0.158.0/receiver/windowsservicereceiver)。后二者在该版本为 alpha，不能由“已内置”推定所有 OS/权限组合均已验收。

没有引入独立 exporter、外部采集服务或 Receiver fork。Argus 不替用户安装/启动系统 D-Bus；不可读取时配置应用失败或缺样本，沿用实际失败/回滚与新鲜度状态。测试镜像内安装并启动 D-Bus 仅属于自有隔离环境准备。

进程采集显式关闭完整命令行、启动命令、可执行路径和用户名属性。保留 PID、可执行名称及实际 CPU/内存/I/O 数值；权限限制不通过植入数值补齐。

systemd 的单元可以在停止后被系统卸载，原生 Receiver 不保证继续输出该单元。测试通过实际依赖保留一个已加载单元，分别验证运行和停止，另一个单元实际失败。产品说明明确已加载单元范围，不将未出现的状态补成 0。

固定能力的公开 API Key 枚举现在由运行时目录同步生成，防止新增能力仅出现在门户、却被 API 拒绝。三平台 Builder 和分发能力表同步更新。

## 默认仪表盘

新增 `host.processes`、`host.systemd`、`host.windows_services`，当前受管默认模板共 12 套，固定指标定义共 9 个。Windows 模板仅完成实现，未取得真实 Windows 取数证明。

| 模板 | 内容及参考 |
| --- | --- |
| 进程 | 按可执行名称聚合 CPU、物理/虚拟内存及 I/O；布局参考 [249 Named processes](https://grafana.com/grafana/dashboards/249-named-processes/) |
| Linux 服务 | 已加载单元 active/failed、服务 CPU 与自动重启；布局参考 [1860 Node Exporter Full](https://grafana.com/grafana/dashboards/1860-node-exporter-full/) |
| Windows 服务 | 状态值和 running 计数；布局参考 [14694 Windows Exporter Dashboard](https://grafana.com/grafana/dashboards/14694-windows-exporter-dashboard/) |

参考模板使用的 exporter 没有进入产品依赖；查询已按实际内置 Receiver 字段和单位重写。Linux 8 个查询均由本轮实际数据验证，缺样本保持无数据。

## 验证证据

| 证据 | 结果与边界 |
| --- | --- |
| `artifacts/plan8-process-services-collector-build.log` | Linux amd64/arm64、Windows amd64 构建成功；不等于三平台实际安装验收 |
| `artifacts/plan8-process-services-unit.log` | 领域/编译/模板/运行时包通过；覆盖参数遮蔽、平台限制、来源独立和全部关闭 |
| `artifacts/plan8-native-domains-process-postgres.log` | 真实 PostgreSQL 自身实例稳定身份、权限与独立生命周期通过；缺少组件的平台不被显示成可用 |
| `artifacts/plan8-process-service-native-loaded-unit.log` | 实际 Linux 进程和 systemd active/failed/inactive、命令行 canary 未采入、独立停采及全关闭运行通过 |
| `artifacts/plan8-process-service-template-queries.log` | 同批原生数据经正常化、真实 Writer/ClickHouse/PromQL 的 8 个默认查询通过 |
| `artifacts/plan8-native-domains-process-typecheck.log` | 所有前端应用类型检查通过 |

初次缺 D-Bus 的容器失败及停止后单元卸载导致缺状态的失败记录保留；没有修改上游 Receiver 或放宽实际状态断言。新镜像归属 `plan8-20261019-process-services`，测试容器使用 private cgroup namespace，未挂载可写主机服务目录或主机 cgroup 树。

本次仍为 Linux 容器可见范围及真实后端取数，未经过正式主机安装/注册。Scheduler、Controller Manager、etcd、Host/Node Claim、Namespace 指标及完整部署验收继续未完成。Windows 状态仍见[用户暂缓决定](./28-windows-testing-deferral.md)。
