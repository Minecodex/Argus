# B11a 直连主机安装包与实际链路修正

2026-11-13，B10c 人工确认后进入 B11a。真实 SSH 接入成功，但旧主机 Collector 制品不能加载当前生成的 `argus_identity.delivery_persistent_exporters`，首次安装实际返回 `COLLECTOR_CONFIG_INVALID`。失败和原生二进制验证输出保存在本批产物中，没有删掉持久投递配置来绕过校验。

## 修正与发布

从已经使用投递修复的实际 Linux amd64 Collector 镜像 `sha256:f15e3b4f64d6d7202161ab392114e44d9c764cc11db4324c5d406cecd40a89d0` 提取同一二进制，实际验证本批候选配置，生成独立 tar.gz，通过现有 Ed25519 发布密钥签名。

发行版本为 `0.3.3-b11a-20261113`，tar.gz SHA256 为 `4e221f3e50126abd5a3b227e547b2335f614aa4dfd974bb6d45c857fce8d5be0`。版本是此次签名制品的发行标识；二进制自身仍携带原镜像的 `0.3.2-plan8` 构建信息，完整来源以镜像/文件摘要追溯。

通过产品对象存储发布制品，并运行正常 `argus-telemetry-catalog-sync` Job 登记，未直接写业务 SQL。CatalogSync 增加可选 `Platforms` / `ARGUS_OTELCOL_PLATFORMS`：本次只发布 `linux_amd64`，不把同一版本号套到未重新验证的 ARM/Windows 制品。省略时沿用原来的全部已配置平台行为；未知或重复平台拒绝。原有平台的制品、发行记录及共享目录任务保留。

该修改只影响发布 CLI，使用专用 catalog 镜像/一次性 Job；没有替换正式 Server/Worker/Web。CLI 平台选择测试与真实 Catalog Job 已验证，主机安装器仍校验签名、大小和摘要。新发行记录 ID 为 `f0566dd7-b05c-5457-861c-6ead16de4089`。

## 首次失败后的身份与网络

首次激活未完成注册时，普通 repair 不签发初始注册令牌，出现 `COLLECTOR_COMMAND_INVALID`。本次通过正常卸载、使用新制品重新安装处理；保留旧执行失败，不伪造 effective revision 或身份文件。此记录不宣称普通 repair 已支持缺失初始身份。

直连主机还受到本机 Fake-IP DNS 影响，集群服务名被解析为 `198.18.0.37`，身份注册请求 EOF。本批增加有归属标签的原生遥测 LoadBalancer Service，并仅在本批主机解析实际地址 `172.31.0.8`。证书名、CA、原生 mTLS 与持久队列保留；没有禁用 TLS，也未改用 Executor Tunnel。

最终 Host 为 `control_path=direct`，Collector 为 `route.kind=direct_argus`、`route.transport=direct`，注册/采集配置收敛。受管主机 SSH 安装、终端 Shell/OpenSSH 和原生 TLS 指标投递分别保留证据。

## 证据与范围

首次连续启用采集还留下真实 `SOURCE_REJECTED` 历史投递缺口 1999 点，详情见 B11a-D01。后续持续采集与正常终端成功不等于历史数据已补回，也不等于配置切换的源授权交叠问题已修复。保留原始日志、持久累计和未处理缺口；最终报告区分“当前业务/查询健康告警为零”和“一条历史投递缺口仍在当前列表”。

产物根目录：`C:/Users/23574/.codex/artifacts/argus-plan9/p9-b11a-20261113-01/`。

- `collector-candidate-diagnostic.log`：旧制品实际失败。
- `collector-release/actual-candidate-validation.log`：实际候选二进制验证。
- `collector-release/release.json`、`catalog-sync.log`：签名、来源、产品目录同步。
- `fresh-install-native-tls.log`、`direct-route-verification.json`：正常重装和真实 direct 路由。
- `checks.json`、`final-preservation.json`：最终验收门禁、旧发行保留、正式部署/PVC 保留。

本地正式平台的 evaluation 安全姿态仍保留既有 NetworkPolicy/外部 Egress Gateway 提示；健康检查通过不代表本批新增了生产网络隔离能力。Docker systemd 主机共享内核，容量指标按实际可见范围解释。人工查看路线见 [B11a 记录](./batches/B11a-review.md)。
