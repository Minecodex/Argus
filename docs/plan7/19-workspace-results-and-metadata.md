# Workspace 业务结果目录与元数据

2026-10-18 用户确认：远程输出不再按执行请求／步骤 UUID 分目录。模型按业务需求组织 `/workspace/results` 下的目录和文件，平台把追溯编号集中记录到一个 `results/meta.json`。

## 落盘和检索

AI 在目标 `ARGUS_OUTPUT_DIR` 下创建业务相对路径，例如 `nginx/before/status.txt`，收集后原样映射到 `/workspace/results/nginx/before/status.txt`。AI 也可以用 Workspace 的 read／write／edit／bash 继续分析、整理和创建业务文件。不新增逐文件传输声明或路径授权参数，远端 input／output 仍按每次 connect 隔离。

平台只保留 `results/meta.json` 这个索引位置。它记录原始结果文件 ID、执行请求／Run／步骤／资源／会话 ID、业务路径、原始输出路径、大小、Hash、采集时间和来源引用。索引不记录凭据或内部对象存储地址，模型可直接读取它定位业务文件。用户自建的业务文件无需伪造这些追溯字段。

## 服务边界与可靠性

- 远程执行服务继续封存原始字节和可信执行事实；Workspace 服务负责业务路径选择、导入、索引和下载记录。数据库新增 Workspace 结果映射账本，模型可编辑的 `meta.json` 仅是其派生副本，不成为授权或原始证据来源。
- 结果映射在写入前持久登记，重试使用同一文件 ID 与业务路径。重复收集不生成副本，内容或来源变化拒绝冒充原结果；已经修改的 Workspace 文件不被恢复流程覆盖。
- 业务相对路径保持模型命名。已有不同结果／用户文件占用同名时，在原目录生成 `name (2).ext` 等可读名称并在索引中记录实际路径，不覆盖旧文件、不引入 UUID 子目录。`results/meta.json` 为平台索引保留；同名远端业务文件改用可读后缀。
- 路径、字节和 Hash 仍由原有固定文件 RPC 校验；租约／fence 串行化 Workspace 写入，来源必须先登记且在整个工作目录上持续校验。原授权、离线、满额停止写入、会话删除及不可变交付边界保持不变。
- 索引从服务端已完成的映射重建并原子替换；失败可在原导入重试中恢复，不重新执行远程命令。停止后的已批准输出继续沿用受控收集入口。

旧目录不自动清理或迁移。本规则用于新收集结果，旧证据仍按其原路径读取；不以删除已有业务文件实现布局调整。

## 验证

2026-10-18 本轮完成以下验证，详细日志及复现入口见[验收记录](../../artifacts/workspace-results-20261018/acceptance.md)：

- 独立 PostgreSQL 安装到 migration 20，`up → down → up` 通过。Workspace、Remote Execution、Tool Runtime、argus-dev 与契约五个相关 Go 包回归通过，另有 Workspace 最终回归和 Go vet 通过。
- 实际本机 PowerShell 在隔离临时目录创建 before／after 文件，经原生执行器输出收集、内容校验、PostgreSQL 映射和真实 Workspace 文件 RPC 后，业务目录和唯一 meta 索引正确。数据／授权使用独立夹具；封存服务对实际业务路径返回及幂等的测试使用内存对象存储，不扩大为真实 S3 端到端验收。
- 文件／目录重名、Unicode 长文件名、重复收集、原子改名后崩溃恢复、保留用户修改、伪造 meta 重建、撤权、损坏字节、配额失败后的索引恢复、失效 writer fence 和路径逃逸均通过。
- 使用现有 Coding Plan `glm-5.3` 和真实 Provider／文件 RPC 运行一次有界工具循环，**4 次调用，输入 5002／输出 222 Token**。模型实际读取 meta 与两份证据，正确生成 `/workspace/results/nginx/analysis/comparison.json`，数值和随机证据标记均匹配。该测试是独立工具循环，不是已部署 Agent 的完整配额／会话业务验收。
- **14/14 Playwright** 通过：业务结果与 meta 中英／深浅色 4 项、原目录树 8 项、MCP／上传保留 2 项。新 UI 场景使用实际数据库／RPC 导出的夹具，但浏览器 API 仍为 mock。相关前端单测 **13/13**、类型、Lint、i18n／样式、正式 real 构建及包体积门禁通过。[浅色效果](../../artifacts/workspace-results-20261018/light.png)／[深色效果](../../artifacts/workspace-results-20261018/dark.png)。

本机 DNS 初始返回 198.18/15 Fake-IP，真实模型首轮在 SSRF 地址校验处被拒绝。复验使用公开 DoH 核验的公网解析，仍保留原公网地址限制、TLS 主机名验证和重定向校验；没有修改生产网络策略或记录模型密钥。

当前部署尚未更新，本轮未启动临时 Kubernetes namespace 或重跑完整生产 Agent／对象存储业务验收；已有目录与业务文件未迁移或删除。临时数据库按本次独立归属清理。
