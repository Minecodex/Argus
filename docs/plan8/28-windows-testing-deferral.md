# Windows 实机测试暂缓

2026-10-19，用户回复“windows 暂时不测试把”。从该决定起暂缓 Windows 实机准备与验收，其余 Plan8 实现和测试继续。

保留 Windows 11/Server 的接入和 Collector 实现、配置验证及构建。现有 Windows amd64 Collector 已构建，并读取过原生 feature gate；这些仅是构建/组件证据，不能替代正式平台安装、服务启停/升级/回滚、主机指标、Windows Event Log 或 IIS 验收。

实际只读预检记录为 Windows 11 Pro amd64 build 26200、非管理员。未执行 Prepare，没有为本轮启用 IIS、安装 Argus 服务或建立测试站点/事件源。现有证据：`artifacts/plan8-windows-local-preflight-driver.log`、`artifacts/windows-local/plan8-windows11-20261019/preflight.json`。

`scripts/plan8-windows-local.ps1` 保留 Preflight/Prepare/Restore 驱动；用户恢复测试前不执行管理员准备。本机驱动仅面向 Windows 11，不能为 Windows Server 出具实测结论。其 Restore 只清理已记录归属的站点、应用池和事件源；IIS/WAS 功能变化单独保留基线，不能声称已自动完全恢复系统功能。

| 范围 | 实现/构建 | 实机验收 |
| --- | --- | --- |
| Windows 11 接入、服务生命周期、主机指标 | 保留，部分构建/组件检查通过 | 用户暂缓，未验证 |
| Windows Event Log、IIS | 保留原生配置与实现 | 用户暂缓，未验证 |
| Windows Server | 保留实现 | 未验证，本期不作为 Server 实机发布门禁 |

进度以 `deferred_by_user` 表达执行状态，以 `not_verified` 表达验收事实；不填 passed，不把 Windows 加入“无合格内置 Receiver”的 12 项暂缓目录。此次决定不表示永久验收豁免。发布汇总仍需逐项列明实际支持和未验证范围。
