# 上游 CRD 安装 Hook 的镜像冻结

状态：安装器修复已实现，包回归及锁定上游 Chart 的真实 Helm 渲染通过，完整安装待下一轮验证。

r36 已越过先前 Strimzi 冷镜像阻塞，其 Operator 正常 Ready；随后 Altinity 0.27.3 的 pre-install CRD Job 长期未就绪，在固定 15 分钟上限失败。上游 Chart 默认将 `bitnami/kubectl:latest` 用作安装工具，这既带来额外网络前件，也不能证明可复现版本。本轮业务、NetworkPolicy 探针和新增模型场景均未运行，不计通过。

失败后先提前恢复原 Desktop 服务，再由外层 finally 幂等复验，原副本、身份及存储通过，server 1/1，本轮 Kind Namespace 清零。原失败记录位于 `artifacts/plan8-calico-completion-20261021-r36/`。

安装器现将已有 `argus-remote-runtime` 镜像纳入摘要解析，按 `repository:tag@sha256` 给上游 `crdHook.image` 提供冻结引用。该运行时已包含 `/bin/sh` 和本仓库构建的 kubectl；本轮制品本来就需要构建及装载。它只替代安装工具容器，不增加独立采集服务、不 fork Collector 或上游 Chart。

保留上游 CRD Job 的命令、服务账号、RBAC 和 `kubectl apply --server-side=true --force-conflicts` 行为，不跳过 CRD 安装，不绕过策略或 readiness。已有共享 CRD 的原有验证路径保持。

`artifacts/plan8-pinned-crd-hook-render-r2.log` 验证实际锁定 Chart 的 Hook 清单包含完全相同的冻结引用、没有原 `bitnami/kubectl:latest`，并保留原应用命令。首次单测用了不适合本仓库 Helm 4 的旧渲染 API，已改用已有 action/release 干运行模式，未改变测试要求。

下一轮必须实际执行该 Job 并继续安装、网络强制阻断和所有业务阶段；渲染通过不能代替安装通过。仍未完成的模型场景、支持组合、故障容量和最终发布继续各自保留门禁。
