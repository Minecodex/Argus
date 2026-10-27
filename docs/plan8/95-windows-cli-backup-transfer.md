# Windows CLI 备份与恢复文件传输

2026-10-23，来自 R73 发布演练的实际失败。

旧镜像的 local-hardening 安装、Schema 19 七个数据库角色初始化、固定旧版七项门户及身份测试、Secret／凭据验收数据均已通过。加密备份阶段调用 `kubectl cp pod:/tmp/snapshot C:\Users\...\snapshot` 失败：kubectl 将本地盘符中的冒号按远程文件标记解析，返回两端必须包含一个本地地址的错误。R73 原记录保留为失败，原部署恢复核验 41.11 秒通过；原业务目标未升级。

## 公共修正

备份与恢复共用 `podFileTransfer`：先在调用方解析本机绝对路径，再以该文件父目录作为单个 kubectl 子进程的工作目录，只把文件名交给 `kubectl cp`。Pod、Namespace、显式维护容器和传输方向保持独立参数；不修改全局工作目录。相对及多文件 `KUBECONFIG` 和显式 kubeconfig 参数先按调用方目录解析，保留原先的配置选择。

OpenBao 快照、MinIO／ClickHouse 归档的收取和回传均经过公共路径。PostgreSQL dump／restore 从已有数据库容器环境读取密码，命令参数不再包含密码值。配置、数据库 Schema、采集协议和 Collector 插件边界没有变更。

## 验证范围

新增真实 Namespace 与双容器文件传输验证：上传、下载包含全部字节值的 2 MiB 文件，使用指定的维护容器；在带空格的本地路径执行实际备份归档、删除验收文件、恢复归档并比对 SHA-256；同时验证相对 kubeconfig、调用方工作目录与环境保持不变。Namespace 按 UID 与专属标签校验后删除。

该验证加入既有 `postgres-backup-restore` 具名组，完整驱动仍为 89 组。它验证的是 Windows 上 CLI 对 Linux Kubernetes Pod 的文件传输，不代表 Windows 主机监控、事件日志、IIS 或 Windows Server 实机已经通过；这些仍按用户决定暂缓。

本机先行真实 Kind 回归已通过，耗时 47.47 秒，实际本地路径为 C 盘、调用目录为 D 盘。2 MiB 二进制与归档恢复一致，指定维护容器、相对 kubeconfig、原 CWD 和环境均通过，Namespace 已按所有权清理，见 `artifacts/plan8-kubectl-transfer-live-r74.log`。加密归档篡改拒绝、独立恢复目标及 CLI 包相关单元回归通过。

实际回归结果、新冻结源码、完整组件与平台验收、发布／恢复及原目标升级分别记入 [进度](./progress.json)和[交付记录](./57-completion-delivery.md)。公共修正的局部回归不能替代完整发布门禁。
