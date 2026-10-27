# 本机正式版与测试环境清理

> 后续明确授权：2026-10-26 用户要求清空后端业务数据并从头开始，已完成[业务数据重置与账号重建](./plan9/05-local-data-reset-20261026.md)。下文的数据保留检查属于此前发布/清理时点；本次旧企业、账号、模型、会话、遥测和 Workspace 已按新授权删除。当前平台管理员为 `admin`，`test` 企业管理员为 `kakj`。

2026-10-26 用户确认：保留正式构建，不要求高可用；其他 Argus 测试环境、测试数据卷、旧镜像和构建缓存清理，主环境业务数据及必要共享依赖保留。

## 保留的正式环境

- 企业门户：<https://argus.dev>；平台门户：<https://platform.argus.dev>。
- 当前版本：`local-20261026-b01-defaults-v5`，Docker Desktop / linux-amd64；B01 更新后端目录、模板、关联读取和查询，Web 与其他伴随镜像复用 `local-20261026-165617` 原摘要，Collector 保持原签名发行版。自检与开放项见[实际查看记录](./plan9/batches/B01-review.md)。
- 当前配置：`deploy/.cache/argus-local-formal.yaml`，采用受限文件权限。内部部署 profile 为 `evaluation` 的单机拓扑；产品构建使用 real API，后端没有 E2E 构建标签，生产制品扫描通过。
- `argus-system`、`argus-sandbox`、入口、证书、共享 OpenSandbox、存储驱动及 Docker Desktop 系统组件保留。
- 本地镜像仓库保留当前 7 个产品镜像、Collector 和实际模型 DNS 依赖，以及前一版本的回退引用。健康检查及未来创建 Workspace/PVC 需要的辅助镜像也保留。

单机部署验证仍记录现有环境边界：NetworkPolicy 未观察到强制拒绝、没有独立外部 Egress Gateway。这不构成 production 隔离或高可用认证，也没有通过修改 profile 名称绕过生产前件。

此前[监控筛选与仪表盘工具栏修正](./plan8/109-monitoring-filters-and-dashboard-toolbar.md)发布到平台 revision 44：25 项桌面交互/布局自动化、17 项真实 API 检查及实际门户复核通过；标准健康检查 20/20。原有业务字段和 42 个 PVC 绑定核验一致。以下清理数量保留前次 `local-20261026-013254` 清理时点，不代表后续构建后仍为零缓存或仅有九个标签。

## 17:12 重新构建与升级

按用户再次要求完成完整正式构建、镜像加载和标准安装升级：平台 revision **45**、数据服务 revision **16**、遥测管线 revision **11**。使用当前工作区源码，未提交、回退或清除已有修改；构建输入指纹 `b545295bcd5d3d3369a8c4424f2adf057e49876d865c7a779923723e217bc704` 在构建及发布后核对一致。三平台 Connector 已签名并注册；这不代表新增 Windows 或 ARM 实机验收。

- 稳定状态下标准健康检查 **20/20**、已有账号真实 API 检查 **17/17** 和实际门户交互通过。
- **10 个运行容器**的实际镜像内容与本次构建一致。Web 的 containerd 状态保留了缓存镜像的旧索引引用；已读取双方 OCI 索引并核对相同的 Linux amd64 执行清单，未仅按标签推定内容相同。
- **13 张业务表**的原有保护字段、**42 个 PVC 绑定**及 **36 个 Workspace 卷内常规文件的路径和内容摘要**前后核对一致。没有进行历史遥测事实表的全量复验，不扩展旧 Plan8 的历史验收结论。
- 前后两次只读文件观察 Pod 均已按 UID 和所有权清理；没有重建测试集群或删除业务卷。旧版本与受限配置备份保留，未执行回退。

文件预检首次遇到本机 OpenEBS Rawfile 0.15.1 的设备入口缺失：`losetup` 返回 `/dev/loop8 (lost)`，驱动将该结果直接用于创建命令而失败。核对 CSI `/dev` 对应节点设备目录后，仅补齐缺失的 `/dev/loop8`～`/dev/loop63` 共 56 个块设备入口；原有设备连接未变，未卸载设备、格式化卷或改写磁盘镜像。随后全部 36 个卷完成只读校验。本次恢复当前节点的运行前件，没有修改上游驱动；节点或虚拟机重新创建后仍需复查这些入口。

初次健康检查在切换及临时观察任务运行期间记录 **19/20**，全局就绪项未通过；该失败证据保留。观察任务清退、全局状态就绪后，使用新的证据目录重新执行完整检查并得到 **20/20**，未修改就绪判断或豁免门禁。

本轮构建、安装、前后比对及真实门户证据位于本机受限目录 `C:/Users/23574/AppData/Local/Temp/argus-local-upgrade-20261026-165617/`；当前安装配置仍为 `deploy/.cache/argus-local-formal.yaml`，目录内保留前一版本配置备份。

## 已执行清理

| 对象 | 实际结果 |
| --- | --- |
| 独立测试运行时 | 删除 `argus-monitoring` 测试集群及 Plan7 Host/Bastion/中继、旧 `argus-test-host`，合计 5 个容器 |
| 测试数据卷 | 删除 18 个闲置 MySQL 卷及 Minikube 专用卷，合计 19 个 |
| 测试网络/入口 | 删除 3 个专用 Docker 网络、3 个测试桥接 NodePort |
| Docker 旧镜像 | 删除 30 个旧/测试镜像；健康核验必需的两种 BusyBox 按原摘要重新保留 |
| 节点旧镜像 | 删除 443 个旧引用，当前运行与动态依赖保留 |
| Registry | 删除 218 个旧标签、570 个旧清单，离线 GC 后核验当前完整 OCI 引用图及层仍可读取 |
| Docker 构建缓存 | 清除约 55.15 GB，最终 Build Cache 为 0 |
| Minikube 配置 | 用户完成两个受策略阻止目录的手动删除，已核验不存在；Kubeconfig 仅保留 `docker-desktop` |

Registry 通过[官方 v3 标签删除接口](https://github.com/distribution/distribution/blob/v3.0.0/registry/handlers/manifests.go)清除旧标签，排除当前 OCI 索引及子清单后删除旧清单，再停库运行原生 GC，恢复后检查内容摘要。未使用无差别 `system prune --volumes`。

主环境中的历史主机、采集和审计记录按业务数据保留；对应外部测试机器已删除并离线。源代码、Git、签名密钥及验收证据保留。

## 验证与唯一待确认项

清理后标准健康检查 **20/20** 通过；**13 张业务表、42 个业务/Workspace PVC、36 份 Workspace 文件内容**核验一致。Docker Desktop 隐藏系统容器与共享依赖均保留。

约 0.9 GB 的 Minikube 下载缓存删除也被自动审批拒绝，已请用户手动清理 `C:/Users/23574/.minikube/cache`；最终状态见[清理汇总](../artifacts/formal-local-cleanup-20261026/completion.json)。不得把未执行的删除记为完成。

证据目录：`artifacts/formal-local-cleanup-20261026/`。首次旧 Workspace 核验器引用了已退役镜像而失败，该失败保留；使用当前不可变工具镜像重新核验后通过。健康检查所需 BusyBox 依赖恢复后重新完成标准检查，未修改原业务文件或降低断言。

Plan8 的历史 P8-A64 缺口单独保留；本次正式单机环境的构建、清理与健康验证不追溯补全旧历史数据证明。
