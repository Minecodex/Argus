# 压缩断点受控对照运行包

这是尚未接入生产的业务无关引用机制原型，以及 B12 历史真实查询文件的固定断点实验。完整结果和限制见[对照记录](../../31-b12-compaction-controlled-comparison-20261116.md)。

文件分工：

- `context_references.py`：通用引用、边界、Hash 与调用身份原型，不识别 B12 资源。
- `prepare.py`：本次 B12 场景适配器，正常登录/下载原件，固定输入。
- `sandbox.py`：临时 Namespace/Pod、离线读/计算及按 UID 清理。
- `model_client.py`：有界真实供应商调用与 provider 用量采集。
- `run.py`：两组共用提示、工具、数据和受控断点，按固定次序执行。
- `report.py`：离线核对与汇总；不请求模型或执行工具。

本次 OUT 默认为 `C:/Users/23574/.codex/artifacts/argus-b12-context-compare-20261116`。完整数据和消息位于受限 `private/`；密钥只从原有受限文件读取，源码、Pod、日志和报告不包含原值。

无需模型调用的复核，在本目录执行：

```powershell
$env:PYTHONUTF8='1'
$env:PYTHONDONTWRITEBYTECODE='1'
python -m unittest -v test_context_references test_model_client test_report
python report.py
```

`run.py` 会产生供应商费用。现有对照已完成并封存，检测到 comparison.json 即拒绝重新执行，防止覆盖证据和重复付费。本次测试不自动开启新的对照。准备新实验时应单独确定新输出目录、正常登录、模型与预算，并保证两个分支共用输入和清理归属一致。

不要将原型脚本当作生产权限或命令幂等实现。生产适配必须复用现有领域授权、服务端引用和一致性提交，不能把模型/外部任意 JSON 当作可信引用声明。
