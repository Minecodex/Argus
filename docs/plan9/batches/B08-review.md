# B08 Linux 文件日志与 Journald 查看记录

B08 已退役，三条日志规则和应用已归档，Collector 和 Connector 已卸载，专属容器与卷已回收。下一批为 B09a。

运行 `p9-b08-20261105-01`。B07c 已退役，应用和仪表盘已归档，Jaeger 和 OTLP 接入点已停用，两把 Key 已撤销。专属容器、卷和网络已经回收，历史记录保留。

## 平台入口

- [应用文件日志仪表盘](https://argus.dev/dashboards/f099ee2b-a7c9-4c83-8902-b5c950600356?resource=7be640c8-299c-50d6-9081-a1f57f4befc4&resource_type=application) 展开「完整记录与字段」，用「搜索当前结果」查找 `caused by controlled dependency refusal`，可看到同一条多行记录。
- [主机 Journald 与服务状态仪表盘](https://argus.dev/dashboards/ffb26ef3-3ef7-48a8-9461-09d2e7d86a45?resource=01a1a987-0939-71a4-9714-6f517e2845d3&resource_type=host) 可对照同一服务的 Journal 和 systemd 失败状态。
- [主机日志查询](https://argus.dev/observe?resource_type=host&resource_id=01a1a987-0939-71a4-9714-6f517e2845d3&signal=logs) 等待采集来源显示 `journald`，点击「执行全部」。
- [监控规则](https://argus.dev/monitoring) 查看 `B08 Application JSON`、`B08 Application Multiline` 和 `B08 Host Journald`。
- [服务故障告警历史](https://argus.dev/alerts?tab=history&policy_id=5cdef600-95ee-5d99-8068-e6277bd91ff9) 在 **2026-11-05 10:29:40** 触发，**10:30:20** 恢复，时间均为北京时间。

## 实测结果

- 实际 HTTP 请求产生了业务 JSON、多行异常及 systemd Journal。应用日志归属 `orders-log`，Journal 归属 `B08-Linux-Logs`。三条采集规则均生效且有新样本。
- rename/create 和 copytruncate 各逐条核对七条记录。Collector 重启后，旧记录保持一份，新记录继续进入。单独停用 JSON 规则期间，多行和 Journald 仍有新记录，恢复后核对了停采期间写入的 JSON。
- 受控启动失败产生了实际 systemd 故障记录，告警状态随指标值从 0 到 1 再回到 0。告警已恢复，本批活跃告警为 0。
- 日志测试字符串经过实际文件采集，`authorization` 字段敏感值已脱敏。两套仪表盘共三个面板，四组浏览器检查覆盖中英、明暗、1280 和 1920 像素桌面视口。
- 低负载继续运行，最长 8 小时。需要回看演练时，可将时间范围设为 **10:27–10:33**。日志结果的下载入口列为 **B08-D01**，本轮未完成该项验收。

## 运行与证据

- [运行和清理流程](../runtime/B08/runbook.md)
- [最终核验](../../../artifacts/plan9/p9-b08-20261105-01/checks.json)
- [rename/create](../../../artifacts/plan9/p9-b08-20261105-01/rename-rotation.json) · [copytruncate](../../../artifacts/plan9/p9-b08-20261105-01/copy-rotation.json) · [重启与停采恢复](../../../artifacts/plan9/p9-b08-20261105-01/lifecycle-checks.json)
- [HTTP 503 与三种原生记录](../../../artifacts/plan9/p9-b08-20261105-01/http-503-multiline.json) · [脱敏](../../../artifacts/plan9/p9-b08-20261105-01/redaction-check.json) · [浏览器检查](../../../artifacts/plan9/p9-b08-20261105-01/browser-review.json)
- [B07c 清理核验](../../../artifacts/plan9/p9-b07c-20261104-01/cleanup-verification.json)
