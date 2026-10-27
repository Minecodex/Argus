# 八场正式模型完成与剩余定向修复

2026-10-22，r51 完整运行已经结束。原 Docker Desktop 的副本、资源及存储身份恢复核验通过（59.13 秒），server 1/1，本轮隔离 Namespace 清零。整轮仍失败、未发布。

## 实际通过

八场真实 Provider 模型均通过，用量及 cached usage 完整：K8s 恢复 9 次、单实例独立停采 7 次、SDK Key 私密交付 13 次、应用/共享数据库分析 9 次、安全表单与后续确认合计 22 次、真实后台告警健康 5 次、同会话撤权合计 10 次、主机仪表盘 Workspace 16 次。

撤权前实际生成两份来源绑定的原生查询文件并读取；撤权后文件下载和 Workspace read 拒绝，当前描述后读取原应用被实际领域拒绝，另一个有权应用读取成功。不是用 Describe/输入错误冒充权限拒绝。上轮告警契约和自动查询交付缺口已在正式平台链路通过。

两 MySQL、SQL Server、Redis、Nginx 的逐目标真实账户授权链通过：正常创建用户、修改临时密码、MFA、未授权 403、授予后真实历史样本、撤权及正常重新登录后再次 403。新增 Memcached、Apache、HAProxy、ZooKeeper、RabbitMQ、MongoDB、Elasticsearch、Kafka 均通过正式创建/有效配置、真实数据、默认面板、目标授权与独立启停。仍须逐组合整合认证负向等组件证据，不能据此提前锁定全部支持矩阵。

大列表专项用 80 个实际测试应用、80 个没有伪观测时间的声明服务，通过真实 PostgreSQL/HTTP 和生产 Vite 门户验证分页、跨页选择、搜索、取消/Escape、父草稿、环境继承及候选撤权。中英明暗四种组合通过；注册/会话和撤权注入为明确受控夹具，未提交监控变更，不扩大成正式登录或 A06 全部分支完成。

## 保留的失败

1. K8s 持久队列恢复：原始 16 条日志在上游 Agent 完全停止时全部到达查询，日志队列 0、发送 16、失败 0；总队列仍有 3 个 metrics 请求，接收端持续返回准入暂不可用。没有调高三分钟恢复或五分钟重试上限，也没有删除总排空断言。指标具体准入原因仍须定位。
2. PostgreSQL 17.11 已有效配置并有 Ingest 样本，但首次指定指标查询为空。驱动不应把 Ingest 观测等同于 Writer/查询可见；新增与默认视图相同的 45 秒查询可见性等待，继续要求正确目标和采集实例标签。
3. Oracle 23.5.0.24.07 配置有效但没有数据。目标监控登录、版本和禁止业务写入已验证；真正 Receiver 失败原因未在清理前保存。新增每组失败后、目标尚存在时的脱敏日志捕获，待定向复验，不称为资源不足或已修好。
4. 五类探测在启动第一个实际目标时失败。其 Docker 目标错误使用了集群访问镜像地址；改用现有本地镜像映射机制，并保存脱敏启动错误。尚待正式链路复验。

最终诊断中的 PostgreSQL/Oracle DNS 错误发生在目标清理之后，不能把它当成最初失败原因。

## 独立确证的 UID 修复

pinned Kubernetes apimachinery v0.35.3 的 UID 类型包含非 UUID，真实 Native 捕获中也存在静态 Pod 的哈希 UID。原对象准入使用 UUID 格式校验，错误拒绝可信库存中已登记的 Pod。现改为有界、非空、无控制/空白字符的原样值，再按企业/集群/对象类型/Namespace 精确查询可信库存；不通过名称推断或获得新授权。

真实 PostgreSQL 的正确、跨 Namespace/集群/类型、空值和控制字符、Namespace 重建回归，以及真实 Writer/ClickHouse Namespace 查询过滤通过。该修复不等于解释了上面暂时不可用的指标请求，后者仍单独处理。

## 复验范围

增加 `--plan8-phase integrations|kubernetes` 和仅在 integrations 局部验收下使用的 `--plan8-integrations`。局部验收保留正式主机接入与真实平台前件，明确输出 partial scope、实际模型数量和所选服务，不声明整计划或发布通过。默认 all 仍执行完整阶段；未知/重复分组和跨模式组合拒绝。

此入口用于定位和修复剩余问题，最终仍需冻结源码/生成物/镜像/Collector/视图版本、完整 73 组及 76 项和发布复核。Windows 实机按用户决定暂缓，11 类资源与 SkyWalking 原生协议继续暂缓，原 Docker Desktop CNI readiness 问题仍单列。

证据：`artifacts/plan8-calico-completion-20261022-r51/`、`plan8-r51-live-admission-inventory.json`、`plan8-r51-revocation-domain-denial-audit.json`、`plan8-picker-live-r3-sealed-evidence.json`、`plan8-opaque-kubernetes-uid-query-regression.log`。r51 源码指纹为 `0768a0bf3aebeacf75a6bdf5200394f604c998c3d1720a7db7e30ff9fdda7fe2`；修复后新源码不得冒用这个指纹。
