# 认证边界、查询窗口与精确制品

2026-10-22，r52 为明确标记的局部验收，保留正式安装、门户、Host 接入和真实查询前件，未运行八场业务模型。原部署恢复43.30秒通过，server1/1、本轮Namespace清零。

- PostgreSQL 的原主指标与默认面板已通过，剩余失败是读者授权后的查询时间窗。驱动曾用首次 Ingest 样本时间冻结窗口，而指定指标可能稍后才出现。现活跃目标使用当前五分钟，停采目标继续使用实际最后样本的历史范围。固定时钟回归覆盖两者及不同路由；仍要求完整 403/200真实样本/403。
- Oracle 在目标存在时取得 ORA-01017，排除了清理后 DNS 错误。真实原生组件串行对照：17、25、30 字节密码通过；31、41、42 字节失败，其中31字节在 SQLPlus 的真实登录为正向、Receiver拒绝。私密编译回归证明这些长度的密码字节保持一致。Collector0.159使用go-ora/v2 2.9.0；已核对官方0.162及主干仍引用同一驱动，不把依赖相同等同于新版本已实测。长密码组合是否新增未支持项已向用户确认，当前保持待决定，不计通过，也不自动套用暂缓规则。
- 探测错误日志确证 Docker 使用 host.docker.internal 集群镜像地址尝试 HTTPS。原修复误复用会过滤 runtime 的辅助镜像函数，已提取独立地址转换，清理选择与地址转换分开；只转换受控本地 registry，平台/仓库/tag/digest保留，外部和非法端口不转换。新增直接测试 core runtime 路径。
- K8s 暂不可用定位补上固定的安全阶段原因码，区分源配置、Namespace UID、对象映射与控制不可用。保留原重试/永久拒绝语义，不记录 SQL、原始属性、名称、凭据或请求体。当前尚待新的真实K8s局部故障定位，未放宽UID授权或删除队列断言。
- 验收矩阵改为对照当前 SourceSnapshot 的实际平台二进制哈希、Collector/发行版本和默认视图定义版本/摘要。不能用任意64位字符串代替当前制品。APM与日志允许复用真实应用查询和日志入口，不伪造每协议专属模板；冻结物料随汇总保存为 frozen-materials.json。

现有Oracle测试账号收敛到已验证的25字节随机ASCII组合，仍有128位随机性；这只是兼容组合复验前件，不把失败的31字节以上组合改写为通过，不限制或修改既有业务库账号。长密码的发布范围等待用户回答。[发布规则](./10-rollout-and-status.md)不允许测试失败自行豁免。

证据：`artifacts/plan8-calico-focused-20261022-r52/`、`plan8-oracle-auth-compatibility.json`、`plan8-oracle-password-30.log`、`plan8-oracle-password-31.log`、`plan8-oracle-private-bytes.log`、`plan8-r53-query-window-regression.log`、`plan8-r53-material-binding.log`。完整支持矩阵、76项和统一发布仍未完成。

上游核对：[官方0.162 Oracle Receiver依赖](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/v0.162.0/receiver/oracledbreceiver/go.mod)、[驱动官方仓库](https://github.com/sijms/go-ora)。依赖事实与实际兼容试验分别记录。
