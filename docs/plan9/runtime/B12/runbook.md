# B12 真实模型闭环运行包

运行 `p9-b12-20261116-01`。用户通过 B11c 后正常停采、卸载 Collector、卸载 Host/Scope 并销毁旧专属 Compose，保留历史。B12 使用新的主机身份、Redis、HTTP 缓存服务、原生采集和真实 APIMart `glm-5.3-flash`。

执行目录 `D:/workspace/go/Argus`。凭据在 C 盘受 ACL 保护的 `private/` 输入；`ARGUS_PLAN9_PASSWORD` 只用于正常登录，CA 为 `deploy/.cache/argus-trust-bundle.pem`。禁止在日志、Compose 渲染、Markdown 或截图写入模型密钥。

```text
python docs/plan9/runtime/B12/manage.py prepare
python docs/plan9/runtime/B12/manage.py build
python docs/plan9/runtime/B12/manage.py up
python docs/plan9/runtime/B12/network.py
python docs/plan9/runtime/B12/integration.py enroll
python docs/plan9/runtime/B12/integration.py builtins
python docs/plan9/runtime/B12/manage.py workload
python docs/plan9/runtime/B12/integration.py target
python docs/plan9/runtime/B12/integration.py dashboards
python docs/plan9/runtime/B12/model.py
python docs/plan9/runtime/B12/workspace.py
python docs/plan9/runtime/B12/scenario.py configure
python docs/plan9/runtime/B12/scenario.py dataset
python docs/plan9/runtime/B12/browser.py observe
python docs/plan9/runtime/B12/browser.py analysis
python docs/plan9/runtime/B12/browser.py analysis_review
python docs/plan9/runtime/B12/browser.py metrics
python docs/plan9/runtime/B12/browser.py repair_flow
python docs/plan9/runtime/B12/browser.py repair_review
python docs/plan9/runtime/B12/verify.py watch
python docs/plan9/runtime/B12/verify.py verify
python docs/plan9/runtime/B12/handoff.py
python docs/plan9/runtime/B12/progress.py
```

这些是显式阶段，不能整段盲目重跑。已创建身份和模型复用实际 ID；故障和模型调用须核对现有 Run、卡片和预算。`analysis_review` / `repair_review` 只读复核已有文件与已完成执行，不增加用户消息或再次派发命令。普通卡片状态短暂滞后时排除已确认 ID，禁止重复确认。

本次保留了初始32K容量拒绝、默认思考档位的压缩失败和各浏览器失败；最终为同模型128K/低思考档位。原定四个业务步骤因模型省略面板数值而补充一次明确只读请求，共五条用户消息，未增加1美元平台金额额度。调用、Token、每Run24次普通推理、压缩调用及超时的观察界限在 `model-budget.json`；压缩也计入总体调用观察，不以普通推理上限替代总费用限制。

模型配置使用正常兼容性测试创建/更新；后台模型额度只允许本模型、当前部门/用户。Workspace 通过独立平台管理员正常 API 配置本地 OpenSandbox、固定摘要分析镜像和有限计算额度；企业模型或用户不能指定任意 Profile、扩大网络权限。系统配置 `network_mode=none`，共享运行时的部署降级项仍保留，不宣称生产隔离已单独验收。

HTTP/Redis 流量每5秒一次、最长8小时、64个键/600秒TTL上限；容器2 CPU、1472 MiB。故障只向 `plan9-b12.service` 主进程发 SIGKILL，保留 Redis/Collector，并预置600秒恢复定时器和 finally 恢复。真实模型普通 connect 卡片先取消验证无派发，再由新卡片确认执行；原生 stdout/退出码、物理 HTTP、告警恢复都必须通过，不能只根据模型文字或卡片标题判断。

正式健康使用已有 `argusctl.exe verify --config deploy/.cache/argus-local-formal.yaml --output json --artifacts <本批>/private/formal-health`，保留私有配置副本。最终核对同一持久投递实例连续120秒零失败、五项采集新鲜、31面板、模型账单、共享部署和六个正式PVC。缓存完整性/累计失败标记按实展示；不改库抹除失败。

用户确认并要求下一批后执行 `python docs/plan9/runtime/B12/retirement.py`：先关闭本批未完成 Run、停止负载与故障定时器，归档本批告警和监控，通过正常 SSH/卸载/删除流程退役新Host，撤销仅本批 evaluator 数据授权，再按容器ID、卷标签和Service UID删除专属环境。保留聊天、Workspace/交付、计费和审计历史；保留用户提供的模型配置及共享 Workspace 基座，不全局 prune、不清正式数据库或遥测历史。

本批真实页面、绝对时间、失败与金额说明见 [用户查看路线](../../batches/B12-review.md)。
