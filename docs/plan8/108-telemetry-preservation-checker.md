# 遥测保留校验工具修正与历史证据限制

## 问题与边界

旧临时校验器对 ClickHouse 的 Map 输出直接做序列化摘要，Map 键顺序变化可能导致同一逻辑内容产生不同摘要。旧基线只保留摘要，无法在事后还原原始目录内容并重新排序。R88 的 P8-A64 因 322 条历史目录记录未获完整证明而保持未通过。

这不是数据已经丢失的证明。检查现有 PostgreSQL 加密恢复包、ClickHouse 备份记录、detached parts 和 shadow 冻结目录，未发现可用于恢复该元数据的本地内容；精确旧样本对应的 Kafka 分区 2/offset 4580 也早于 Broker 最早保留的 4582。不能拿后到的旧时间戳样本、新采集基线或当前系统健康代替这项历史证明。

## 实现

维护工具进入仓库：

- [规范化与流式比较](../../scripts/plan8/preservation_core.py)：Map 按键排序；数组顺序、值、类型和重复行数量继续参与校验。重复 Map 键拒绝处理，避免静默丢弃信息。
- [采集与复核入口](../../scripts/plan8/telemetry_preservation.py)：读取真实 Namespace、表 UUID 和列类型；只输出摘要及审计结果，认证信息经进程内存和 stdin 传入。
- [回归与实际 ClickHouse 验证](../../scripts/plan8/test_preservation.py)：字段重排、值/资源/单位/数组变化、重复行丢失、损坏或乱序摘要、旧格式、错误目标、归档篡改和真实停写前件。

新版使用 `argus.typed_tuple.canonical_maps/v2` 格式。验证前检查格式、目标身份、原始归档摘要和路径；旧格式直接拒绝，不自动迁移、不修改旧结果。出现缺失仍检查后续表，整体结果保持失败。

工具不会自动缩容或修改数据库。采集前后都读取 Writer Deployment 和 Pod，必须真正停止写入；维护流程应记录原副本并在 `finally` 中恢复。查询限制为单线程、512 MiB、180 秒，客户端有超时和退出码核对；该工具不是灾难恢复备份，也不直接生成发布批准。

## 使用

在已受控暂停写入的维护窗口采集，完成后恢复 Writer：

```powershell
python scripts/plan8/telemetry_preservation.py capture `
  --context docker-desktop --namespace argus-system `
  --output artifacts/preservation-new-capture
```

恢复正常写入后，针对同一目标验证新基线：

```powershell
python scripts/plan8/telemetry_preservation.py verify `
  --context docker-desktop --namespace argus-system `
  --baseline artifacts/preservation-new-capture/manifest.json `
  --output artifacts/preservation-new-verify
```

保留已有输出目录，复验使用新目录。失败返回非零退出码，不能忽略异常继续发布。

## 验证与版本声明

12 项专项检查通过，包含 3 项真实 ClickHouse 查询验证。原环境上的新基线 30 表完整采集、恢复写入后复核均通过，Writer 已恢复到原副本并就绪。旧格式拒绝、写入仍运行时拒绝采集也通过实际 CLI 检查。结果见[实际观察](../../artifacts/plan8-preservation-tool-validation-20261026/observation.json)及[30 表复核清单](../../artifacts/plan8-preservation-tool-validation-20261026/verify/manifest.json)。这是新基线的实测结果，不关闭旧 P8-A64。

本次修改维护脚本与状态文档，没有修改 R88 产品运行代码或重新发布产品镜像。R88 的冻结材料与历史验收记录保留原指纹；新增工具只声明其本轮专项验证范围，不将整个仓库的新状态追溯标记为 R88 全量验收通过。
