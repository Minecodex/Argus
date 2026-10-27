# 文件日志的运行目录与来源保护

新增文件日志规则禁止直接声明 Argus 身份、凭据、状态等运行目录。宽泛匹配仍由原生 Receiver 强制排除这些位置；额外使用 filelog 内置 filter 检查实际解析后的文件路径，软链接不能把运行目录内容带入导出队列。该检查发生在 JSON 解析之前，错误采用 quiet 策略。

JSON 业务字段继续提取，但文件原始路径及解析路径先通过原生 copy 操作保存，解析后通过 move 恢复。日志正文不能伪造这些来源字段。旧 file-log Profile 的固定路径也使用相同保护；没有新建自定义采集插件或外部进程。

- [规则与配置单层验证](../../artifacts/plan8-runtime-path-protection-unit.log)：Linux/Windows 路径、大小写及直接声明拒绝。
- [真实原生 Receiver](../../artifacts/plan8-native-runtime-path-lineage-guard.log)：在 Linux 容器内创建普通文件及指向运行目录的软链接；受保护文件正文试图伪造路径，仍被丢弃。普通文件能够上报，正文伪造的路径不能覆盖真实来源。

原生测试使用认证组件 Sink，不是正式平台部署；Windows 目前只验证路径规则，尚未在本机正式接入中通过。权限、旧配置迁移、系统日志/事件日志及最终范围继续以 T08/T16 为准。
