# Plan6 本地升级与管理员设置

当前正式环境为 `docker-desktop / argus-system`，平台 Helm release 为 `argus-platform`。2026-10-12以l01同组六候选镜像保留values升级三个release至revision10，包含h14主机/堡垒机追加修复；当前源码完整all、独立PG/前端/契约、正式镜像/数据保留/门户验收通过。镜像来源、源码与部署结果见 [本次最新发布](../../artifacts/plan6-local-release-20261012/final-latest-release.json)；[r02/revision9记录](../../artifacts/plan6-review-20261011/final-plan6-acceptance.json)保留为历史基线。

## 升级前件

- 当前源码的契约、Go、独立 PostgreSQL、前端及 Mock 门禁通过；视觉报告无缺失、待审和未关闭业务差异。
- 同一源码的 `--suite plan6 --plan6-phase all` 通过，所有必要模型与真实浏览器前件齐全。
- 保存该运行的 `source-before-build.json`、`source-after-acceptance.json`、`image-provenance.json` 与 `candidate-*.json`；候选镜像 manifest digest 与验收 Pod 使用的 manifest 一致。保存 backend、web、minio、otelcol、workspace 和 workspace-egress 六项同组镜像，工作台不沿用旧组。
- 刷新正式 Helm values、工作负载副本/UID、PVC/Secret 标识和关键表数据基线，保存 PostgreSQL 备份。敏感 values 和备份放在私有证据目录，不粘贴到日志或会话。

## 保留数据的升级路径

先检查 `helm status argus-platform -n argus-system --kube-context docker-desktop`。出现 pending-operation 时停止升级并诊断，禁止卸载重装。

将同组验收镜像导入节点 containerd，保留 manifest digest，核对节点镜像记录。覆盖文件只包含验收对应的 `images.backend`、`images.web`、工作台镜像引用及必要的版本值，保留当前 runtime、PKI、存储和用户配置。

```powershell
helm upgrade argus-platform ./deploy/helm/argus-platform `
  --kube-context docker-desktop --namespace argus-system `
  --reuse-values --values <已核验的镜像覆盖文件> `
  --wait --timeout 15m
```

若需要暂停降压，只操作已核对 Namespace/release/UID 的 Argus 工作负载；成功和失败路径均恢复原副本并检查健康。Judex、共享 OpenSandbox 控制器及共享存储保持其现有所有权。

升级后核对迁移、服务健康、Pod 实际 digest、关键业务表与 PVC/Secret UID；验证正式门户六 Tab、会话宿主确认、组织权限/审计和视觉状态。恢复及清理完成后才能完成 T08。

## 告警管理员设置

1. 在“组织与权限”管理 ServiceAccount，授予告警评估所需的明确资源和 Query 权限，再在告警设置中选择该账号。资源标签只过滤，不授予权限。
2. 在“通知配置”创建加密凭据引用及渠道；检查路由、分组等待、重复/恢复通知和模板。需要新建策略预选时，在渠道“凭据与提醒选项”设置“新建策略默认选用此渠道”，预览后确认；停用渠道不会被预选，编辑/复制不覆盖原目标。模型仅使用凭据引用。
3. 在“告警策略”选择资源、增加规则，执行试算与 Preview，核对范围、组合、频率、持续/恢复条件和渠道后确认；取消保持配置不变。
4. 查询时同时检查范围、最近评估、健康和完整性；零事件不能直接认定为健康。确认、认领和评论独立于自动恢复。
5. 在通知详情检查批次、尝试和受理结果；Unknown 的受控补发产生独立执行事实。组织审计可关联 Run、ToolCall、PendingAction 和 Execution。

本轮使用真实 HTTP/SMTP 协议接收器验收四种适配器。钉钉、企业微信、飞书测试群及真实邮箱保持独立待配置状态；配置后另行核对实际签名、提醒目标及收信记录。

## 2026-10-11 接续发布核对

完整门禁必须使用正式构建，并覆盖 system 与 observability 实际 Namespace 内的八类核心服务；前件 Replay 镜像单独记录。活动 pinned 引用也检查 manifest digest，正在退出的旧 Pod 保存删除时间，不算当前副本。主 Registry 的六项完整候选在临时清理前复制成功后，再归档全部 OCI manifests、config、layers 和 attestation。

升级入口包括 argus-platform（backend/web、Connector/Collector/Workspace IO 与 egress）、argus-data（MinIO），以及 argus-sandbox（egress 与对应 serverConfig）。全部使用 --reuse-values 和最小覆盖文件；保持现有 execd、第三方 server/controller、PKI、Secret、PVC 和各存储配置。Workspace 的批准 image/profile 是平台业务配置，只按实际现存记录核对，不通过镜像升级伪造业务配置。

正式浏览器验收使用已有账号的凭据或已登录会话。用户已提供现有账号凭据，本轮通过仅当前用户可读的私有配置传入浏览器进程，凭据与会话令牌不进入报告。正式基线暂无模型/Workspace/resource配置，不能把隔离环境的测试配置描述为正式配置。完成后分别报告隔离完整业务与模型、正式服务/数据/页面及外部渠道的验收范围。

2026-10-11 用户已授权Docker Desktop重启，包括共享Judex的短暂中断。重启后六Namespace、33工作负载/PVC UID、原副本和卷绑定全部保持且就绪，新发布端口返回200；见本轮docker-restart-recovery-verification.json与docker-restart-new-port-probe.json。重启遇到旧零字节socket，采用目录改名完整保留，不删除配置、卷或项目数据。源码与镜像门禁继续使用同一冻结指纹。

Pod运行时imageID可能标识index、linux/amd64 manifest或config。只有spec固定到验收root，且完整raw bytes/hash描述符链明确指向该runtime digest，才判定匹配；候选复制与OCI归档始终保留原root，包括attestation。

## 本轮发布结果（2026-10-11）

最终完整all退出0后，argus-data、argus-sandbox、argus-platform依次升级至revision8，迁移version11通过；10核心服务的runtime digest与验收描述符链匹配，4域名严格Host/SNI/证书验证通过。114原资源UID/PVC/业务Secret内容及14表业务ID保留；旧Helm v7状态superseded、config一致。正式门户稳定采集72+3通过，临时PG按UID前置条件删除，5旧辅助容器/3专属测试卷清理，正式Registry及Judex保留。原正式业务配置空态保持，外部渠道目标需另行配置。最终证据见 [final-plan6-acceptance.json](../../artifacts/plan6-continuation-20261009-w/final-plan6-acceptance.json)。

## P6-UI-R01接续发布结果

r02完整all、GLM10+4、真实浏览器9/9和228组视觉通过后，刷新备份、117资源与17表基线，三个release升级至revision9。保留Never拉取策略，预装验收原root并核对规范digest、完整tag@digest及tag引用；缺别名的初始等待经补导入恢复，无卸载重装。迁移version11、10工作负载/6描述符链、4严格TLS域名、正式72+3页面、业务数据与UID、Argus恢复和Judex共享资源核对全部通过。凭据清除，测试库及导入器按所有权清理。[本次最终验收](../../artifacts/plan6-review-20261011/final-plan6-acceptance.json)。
