# B13c 真实撤权、凭据轮换与生命周期

运行号 `p9-b13c-20261118-01`。按用户指令先封存 B13b，清理其专属资源与授权，恢复前一后台评估配置。此前批次的证据、告警与会话作为历史保留。

两台独立 systemd/SSH/Connector/Collector 主机，Redis、Memcached 与两个真实负载应用。以资源清单的实际身份和专属标签核对每项变更。读者使用独立普通登录，后台评估和管理员使用各自权限。遥测 TLS 与产品预览、确认、执行路径保持完整。

1. `manage.py prepare/build/up/workload`，随后 `integration.py network/onboard/collectors/builtins/targets/applications/dashboards`。新容器从固定镜像创建，原生采集须取得数据。
2. 用独立读者在真实页面验证当前 Host、同范围查询、真实下载及已连接终端。在管理员侧变更该读者的明确授权，检查已开启视图、原查询、下载和会话；保留授权范围内的控制资源作正向对照。
3. 在本批主机实际轮换操作系统密码及平台 Secret，核对旧材料、旧预览失效和新版本连接。监控认证轮换使用 Redis 原生认证与正常独立采集配置路径。
4. 分别暂停采集和停止真实服务，保留各自实际状态和恢复证据。检验受管仪表盘和编辑发布的个人副本各自的版本与引用。通过正式入口处理目标、采集、绑定、应用和主机的归档或删除。
5. 完成正常登录的桌面真实浏览器验证、隔离回归、发布和健康检查，按实际状态写查看路线、清理清单和证据。B13b 的人工确认单独记录，E01/E02 按需启动。

## 查看与复验顺序

本轮成果及当前链接见 [B13c 查看记录](../../batches/B13c-review.md)。撤权前运行 `prepare_access.py` 与 `history.py`，再用 `plan9-b13c-revoke-real.spec.ts` 观察真实状态。仅在本轮应用仍在线时顺序运行 `rotation.py ssh/redis` 与 `lifecycle.py independence/copies`，避免拿已经退役的 Reports 重试连接。

实际临时目标仍有关联时，启动 `plan9-b13c-lifecycle-real.spec.ts` 的 `blocked` 阶段。随后运行 `lifecycle.py prepare-target`，在该测试的 `archive` 阶段执行门户确认，再运行 `lifecycle.py verify-target` 和 `decommission.py`。`review` 阶段只读，复查归档数据与保留的 Orders 对照。`verify.py current/watch` 与 `handoff.py` 依次记录当前状态、投递和最终验收。
