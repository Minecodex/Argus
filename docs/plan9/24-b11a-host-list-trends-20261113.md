# B11a 主机列表趋势与详情字段调整

用户反馈：移除主机列表的最近心跳，将控制路径放到详情，列表加入异步 CPU、内存、磁盘趋势。

实现见[主机列表趋势设计](../plans/frontend-redesign/host-list-resource-trends-20261113.md)。复用 Kubernetes 列表的 MiniTrend、指标布局和页面可见性处理；主机仍通过已有带授权的范围查询接口取数，不改 Collector 配置，不新增查询服务。

## 查看

1. 打开 [主机列表](https://argus.dev/hosts)，查看 **B11a-Linux-Direct**。
2. 基础行先显示，三个指标随后独立加载；列表不再展示“控制路径”“最近心跳”。
3. 鼠标移到曲线可查看时间、实际采样时间和使用率；内存/磁盘同时显示真实用量及容量，磁盘显示选定文件系统。CPU 按整机汇总比例解释，不把一个汇总序列当成一个 CPU 核。
4. 刷新、筛选和切换页面检查请求生命周期；三组指标的失败互不遮挡，原有 `…` 操作继续使用。
5. 点击主机名称，详情概览中检查完整控制路径和最近心跳。

本轮保留 B11a 运行状态和人工验收待确认状态，不推进下一批。启动配置阶段的 1999 点历史投递缺口仍保留，本次列表调整未修改或确认该告警。

## 记录

运行入口 `runtime/B11a/host_list_review.py publish|browser|verify`。所有构建副本、冻结源码、镜像摘要、实际页面及数据保留检查放在：

`C:/Users/23574/.codex/artifacts/argus-plan9/p9-b11a-20261113-01/host-list-trends-v1/`

`final-review.json` 只在实际镜像、真实浏览器门禁和数据保留核验通过后生成。当前范围覆盖 17 项相关逻辑/共用曲线交互测试、12 项既有目录页面回归，以及 6 项部署后的真实页面用例（中英/明暗、三种桌面宽度、CPU 延迟、指标失败恢复、筛选取消）。

最终 Web 镜像：`host.docker.internal:5001/argus/argus-web:local-20261113-b11a-host-list-v3@sha256:3d49c8bcfb2d834d26de5fd4fa462a2b02df44dfccaca10ceb08bc38dcc83603`。本轮实测纠正了额外工具权限判断，并在共享 MiniTrend 修复鼠标悬浮及逐点键盘查看，未改动后端权限规则。

本次只更新 Web；正常运行的 B11a SSH/PTY、监控来源、仪表盘、规则、历史和正式 PVC 保持保留。正式部署的实际版本以 `web-release.json` 和对应真实 Pod imageID 为准。
