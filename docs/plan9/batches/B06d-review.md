# B06d Oracle 查看记录

运行 `p9-b06d-20261029-01`，最终部署 `local-20261029-b06d-v2`。B06c 已按正常流程清理，旧容器、卷、网络和当前采集配置无残留，企业账号、共享定义、存储和历史保留。

两个独立 Oracle Database Free 23.5 实例（实测 23.5.0.24.7），A 为场景负载、B 为健康对照。每实例六个面板，共 12 个面板取数成功，6 项真实浏览器 E2E、16 项共享图表回归、16 项企业翻译/数值检查、双语源码检查及 20 项平台健康检查通过。正式 6 个 PVC 和 2 个 OpenSandbox Pod 保留，当前 6 个采集配置 healthy/fresh，Collector 收敛。

**当前两条 Oracle 业务告警均已恢复；平台仍保留一条初次接入产生的发送缺口事件，未替用户确认或伪造恢复。见下方 B06d-D01。** 人工验收待用户进行，自动检查不代替人工通过。

## 查看顺序

1. 在[监控列表](https://argus.dev/monitoring)搜索 `B06d`，查看 `B06d-Oracle-A/B`，分别打开默认仪表盘。
2. 六个面板依次为会话数、CPU、PGA、表空间用量、表空间容量上限、事务提交速率。会话包含原生 USER/BACKGROUND 和 ACTIVE/INACTIVE 维度。
3. 在[告警历史](https://argus.dev/alerts?tab=history)搜索 `B06d`，查看 user sessions high 与 commit rate high 的真实触发、恢复、趋势和时间线。
4. B06c 保留的 no_data 历史事件现在显示“告警查询暂无数据”，有原因说明和关联查询；不再显示默认 `> 0`、空趋势或 0 分钟。

建议场景时间窗口：**2026-10-29 15:44–15:49（Asia/Shanghai）**。当前 A/B 分别保持 200/80 行，正常低频事务继续运行，压力已结束。

## 实际场景

| 场景 | 核验结果 |
| --- | --- |
| 专用 schema | FREEPDB1 内创建 PLAN9_APP.samples，200/80 行，schema 限额 16 MiB |
| 原生事务 | python-oracledb thin 连接实际插入、读取、更新、删除；第二个会话读取已提交行；回滚后原值不变 |
| 最小权限 | app/monitor 创建表返回 ORA-01031；monitor 读取业务表返回 ORA-00942；只授权原生采集必需的 SYS 视图 |
| 会话压力 | A 新增 12 个真实连接，USER 会话达到 14，超过阈值 10；关闭后 <= 6 并恢复 |
| 提交压力 | A 执行真实 UPDATE+COMMIT，平台 1 分钟速率达到约 12.40 次/秒；压力中的原生累计计数差分约 19.04 次/秒，窗口包含的升压阶段不同；停止后 <= 2 并恢复 |
| 健康对照 | B 保持 80 行，正常事务、来源和目标隔离，告警只绑定 A |
| 独立停采 | A 暂停时两策略不评估，B 继续有新样本；A 恢复后有效版本、样本和评估正常 |

阈值在场景前冻结，每 10 秒评估，触发/恢复持续 20 秒，无外部通知接收人。压力最长 180 秒/2000 次操作，systemd 另设 190 秒上限。正常负载 A 每 2 秒、B 每 7 秒，每进程最多 8 小时；到期只停客户端，服务和采集保留，不自动进入 B07a。

## B06d-D01：初次接入的审计冲突与发送缺口

15:30:54 Connector 日志记录审计链 `SQLSTATE 40001` 并发冲突；Collector 卡在旧配置，后续发布重载也出现隧道中断。已通过正式 Collector `preview-repair` 和确认流程恢复，随后两 Oracle 采集均生效且样本新鲜。

平台托管事件 `1fc78dbc-9221-4166-933f-4f1c70a213ec` 保留 **87,705 个指标点的原生最终发送失败计数**；最后一次失败观察为 **15:40:43**。这是 Collector 共享管道计数，精确受影响目标与样本时间未知，不能断言全是 Oracle 指标，也不能把它当作已去重的业务数据丢失数量。最终复核时计数未再增加，当前业务告警已恢复。

该记录没有被删库、清空或人工假置 recovered；当前告警中仍可查看并由用户确认。底层审计竞争未在本批改动，问题保持开放。当前链路恢复不等于历史发送缺口消失，也不等于底层缺陷已修复。

初始运行器还修正了 Oracle 保留字查询别名、客户端 systemd 路径。两个修正均在执行真实场景前完成，运行文件与最终源码 SHA256 一致，可复现主机镜像已重建；实际运行 image ID、覆盖文件哈希及重建 ID 分别记录。

## 页面修正

- 查询健康事件根据真实事件类型与原始原因显示中英文标题，恢复后仍保留原始 no_data 含义；详情显示原因和关联查询，隐藏无意义的业务阈值与空趋势，恢复时间线显示“查询恢复”。
- 短于一分钟的持续时间显示秒；本次以原 B06c 的 10 秒真实事件复验。
- 共享图表对低值自适应小数精度，CPU 现在能显示 0.0010、0.0020 等实际刻度；未修改原始数据或查询。
- Oracle 六个面板补齐中英文标题，四种语言/主题组合及 1280/1920 桌面布局复验通过。

## 认证与容量范围

使用 Plan8 已验证的 Oracle 23.5/Collector 0.159 组合，随机 29 字节 ASCII 凭据；31/41/42 字节组合仍未支持，不截断、替换密码或放宽认证掩盖失败。[镜像维护者](https://github.com/gvenzl/oci-oracle-free)已将 23.5 列为不再更新，本批仅隔离复验历史支持组合。

Oracle 数据端口仅本批私有网络可达；未验收 RAC、Data Guard 或 Oracle TLS，Argus API/门户保持 CA 校验。表空间上限取原生配置及自动扩展值，不代表容器磁盘剩余量或 Free 版容量限制。B03-D01 仍单独保留，不在本批宣称解决。

## 当前对象入口

- [B06d-Oracle-A 默认仪表盘](https://argus.dev/dashboards/53f8b45b-8b18-51a5-a103-bf3953293690?resource=d268b63c-94f0-584f-9f71-cc2c4d45be59&resource_type=monitoring_target)
- [B06d-Oracle-B 默认仪表盘](https://argus.dev/dashboards/53f8b45b-8b18-51a5-a103-bf3953293690?resource=bbd332bf-f80a-5975-9643-3d620ecd63f0&resource_type=monitoring_target)
- [采集主机](https://argus.dev/hosts/01a18691-4460-7842-b84b-61d4479cffdb)
- [B06d Oracle user sessions high 历史](https://argus.dev/alerts?tab=history&policy_id=98dddca8-99d4-5aba-b8e2-4c1544508e92)
- [B06d Oracle commit rate high 历史](https://argus.dev/alerts?tab=history&policy_id=7fe74daf-ff2c-58ef-8d9b-7ad4eed93ed2)

## 证据与清理

- [运行包与清理命令](../runtime/B06d/runbook.md)
- [资源与身份](../../../artifacts/plan9/p9-b06d-20261029-01/resource-map.json)
- [A 事务与权限](../../../artifacts/plan9/p9-b06d-20261029-01/a-protocol-preflight.json)
- [B 事务与权限](../../../artifacts/plan9/p9-b06d-20261029-01/b-protocol-preflight.json)
- [告警触发与恢复](../../../artifacts/plan9/p9-b06d-20261029-01/scenario-checks.json)
- [提交计数交叉核对](../../../artifacts/plan9/p9-b06d-20261029-01/commit-rate-native-proof.json)
- [独立停采恢复](../../../artifacts/plan9/p9-b06d-20261029-01/lifecycle-checks.json)
- [正常 Collector 修复结果](../../../artifacts/plan9/p9-b06d-20261029-01/collector-repair.json)
- [保留的发送缺口](../../../artifacts/plan9/p9-b06d-20261029-01/delivery-gap-final.json)
- [最终指标与来源](../../../artifacts/plan9/p9-b06d-20261029-01/monitoring-verification.json)
- [真实浏览器](../../../artifacts/plan9/p9-b06d-20261029-01/browser-review.json)
- [最终状态](../../../artifacts/plan9/p9-b06d-20261029-01/final-state.json)
- [检查汇总](../../../artifacts/plan9/p9-b06d-20261029-01/checks.json)
