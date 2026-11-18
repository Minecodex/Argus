# B13a：仪表盘、关联范围与权限组合

运行号：`p9-b13a-20261117-01`。由“继续下一轮 B13a”授权；该指令同时接受并退役 B12 专属环境。B12 会话、文件、报告、模型、账单与审计记录继续保留。

## 场景与边界

- 两台独立 systemd/SSH/Connector/Collector 容器主机，Orders 为 production、Reports 为 staging。
- 两个真实监控目标：共享 Redis，以及 Reports 专用 Memcached。两个 HTTP 服务确实请求这些依赖，流量每五秒一次，上限八小时；缓存容量 16/8 MiB、键带 TTL。
- 两台主机共享 Docker Linux 内核；CPU/内存指标不能被解释为两台独立物理机的容量。共享目标去重按平台资源身份验证。
- 本轮验证默认仪表盘、个人副本编辑发布、资源关联、全局与面板局部筛选、空/无权范围、时间刷新、共享依赖去重及归档后的历史。保留一个真实基础服务告警；复杂告警策略和撤权生命周期分别属于 B13b、B13c。
- 测试成员仅有 Orders 主机、Orders 应用和共享 Redis 的数据范围；后台评估主体单独配置。无外部通知、无付费模型调用、无业务数据库直写。
- 控制连接使用现有 Executor Tunnel，遥测直连；本地两个 LoadBalancer 的同域不同端口不以弱化 TLS 或全局网络规则解决。

## 执行

从主工作区运行，Python UTF-8，登录密码仅通过 `ARGUS_PLAN9_PASSWORD` 环境变量传入。私密文件位于当前用户 ACL 保护的外部 artifacts，不写入仓库。

1. 审核并执行 B12 `retirement.py`；核对正常 API 卸载、Compose 归属、正式服务/PVC 基线不变。
2. `manage.py prepare/build/up/workload`；镜像固定为实际 image ID，Compose 归属标签限定清理范围。
3. `integration.py onboard/collectors/builtins/targets/applications/dashboards`；全部使用正式产品预览、确认和执行路径。
4. `dashboards.py publish`、`access.py reader/evaluator`、`verify.py scope`：发布组合仪表盘和个人副本、建立专属成员并执行范围矩阵。
5. `alerts.py configure/exercise`：有自动恢复保护的实际服务告警。`archive.py`：逐项解除依赖，正式归档 Memcached 目标，比较同一绝对时间的历史原始结果。
6. `remote.py`、`verify.py watch`、真实 `plan9-b13a-real.spec.ts` 和 `argusctl verify`。`postgres_tests.py` 只使用专属临时数据库，自动清理；`handoff.py` 在全部证据存在后同步人工查看状态。

本轮已知问题、实际状态和最终链接见 [查看记录](../../batches/B13a-review.md)。初始化历史缺口不清零：Orders 1,642、Reports 568 个指标点，当前观察期增量为零。归档验证后当前有 9 项健康采集，Memcached 通过保留的绝对时间链接查看。不要在归档后直接运行需要初始四资源范围的 `verify.py scope` 冒充同一测试时点；初始与归档后证据分开保留。

## 保留与退役

本轮环境保留给用户查看；用户接受后才启动 B13b。正式 Argus、共享 Workspace、账户和历史不清理。归档目标保留历史身份；采集不再恢复成其他目标。后续按 resource-map 中的 UUID 与 Compose 标签逐项处理，不按名称前缀批量删除。
