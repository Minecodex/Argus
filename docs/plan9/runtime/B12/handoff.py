"""Render the final route from actual model, job, file and native command evidence."""
from datetime import datetime,timedelta,timezone
import json
from manage import Run,ROOT

run=Run()
def load(name):return json.loads((run.out/(name+'.json')).read_text(encoding='utf-8-sig'))
def local(value):return datetime.fromisoformat(value.replace('Z','+00:00')).astimezone(timezone(timedelta(hours=8))).strftime('%Y-%m-%d %H:%M:%S')
m=json.loads(run.resources.read_text());checks=load('checks');usage=load('model-usage-final');repair=load('model-repair-proof');host=m['hosts']['direct'];redis=m['targets']['redis']
report=load('model-published-report-proof');original=load('failed-observe-run')
links=' / '.join('['+row['name']+'](https://argus.dev/dashboards/'+row['id']+'?resource='+host['host_id']+')' for key,row in m['dashboards'].items() if key!='redis')
event=next(row for row in repair['repair']['history']['items'] if row['id']==repair['fault']['firing_alert']['id'])
text=f'''# B12 用户查看路线：真实模型、监控与操作闭环

运行 `{run.id}`。**已部署并完成自动自检，保留环境，等待用户验收。** B11c 已按用户“没问题，开始下一轮”记为通过并正常卸载/销毁；历史审计、告警和录像保留。B13 尚未启动。

## 推荐查看顺序

1. 打开 [本批真实对话](https://argus.dev/?c={m['conversation']['id']})，沿用企业 `test` 的 `kakj` 正常登录。模型为 **B12 APIMart GLM-5.3-Flash**，当前配置 128K 应用上下文、4096 输出上限、低思考档位，同一个供应商/模型，没有 replay/mock 或自动换模型。
2. 对话中先可看到保留的容量/压缩失败与后续真实重试，再看 Workspace 分析报告、Host/Redis 面板取数结论，以及“已取消”和“执行完成”的普通命令卡片。展开卡片查看目标、root 账号、原始脚本、原生退出码和 HTTP 结果。
3. 下载 **B12 Redis INFO 附件分析报告**。真实文件名 `b12-analysis.md`，2226 字节；[正常鉴权下载]({report['url']})。原始 CSV 是此 Redis 的8次实际 INFO，Workspace 实跑代码得到 `command_delta=37`、`peak_memory_bytes=1562696`，下载字节与结果已核对；没有将监控命令冒充业务吞吐。
4. 打开 [B12-Cache-Host](https://argus.dev/hosts/{host['host_id']})、[主机列表](https://argus.dev/hosts) 和 [中间件监控](https://argus.dev/monitoring)，查看新主机、B12-Redis、五项原生采集及实时小图。此批为新身份，未复用 B11c Host/Collector。主机控制与遥测均实际使用 Executor Tunnel；SSH 安装执行路径是 Direct Executor。
5. 查看四套 Host 默认仪表盘：{links}。另有 [Redis 默认仪表盘](https://argus.dev/dashboards/{m['dashboards']['redis']['id']}?resource={redis['target_id']})。共 **31 个面板**，模型实际读取 Host/Redis 的查询任务结果，不仅列目录。
6. 打开 [本批告警历史](https://argus.dev/alerts?tab=history&policy_id={m['alert_policies']['service']['id']})，看 **B12 cache orders service failed** 的真实触发、恢复与时间线。当前 HTTP 服务和 Redis 均健康，本批业务告警已恢复，没有外部通知投递。

## 实际行为与时间（北京时间）

| 环节 | 已核实的实际结果 |
| --- | --- |
| 模型兼容性 | 基础响应、流式、工具调用、结构化输出四项均真实通过；模型和密钥通过正常平台 API 创建，密钥只写、加密保存 |
| 面板查询 | 模型已完成 Host/Redis 两个范围的真实取数任务，窗口及单位见对话；Host CPU/内存与 Redis 内存/命令速率/命中率均有真实数据证据 |
| Workspace | 实际挂载文件、执行代码、写入并发布报告；计算实例回收后，文件仍可正常鉴权下载 |
| 取消 | 第一张模型生成的恢复卡片经 UI 取消，原生状态 cancelled、结果仍 pending；没有派发执行，物理服务当时仍 failed |
| 受控故障 | {local(repair['fault']['started_at'])} 对本批 HTTP 服务主进程注入 SIGKILL；Redis 与 Collector 保持运行，600 秒恢复定时器兜底 |
| 确认执行 | 新模型请求经真实卡片确认；{local(repair['repair']['execution']['results'][0]['observed_at'])} 原生命令退出0，stdout 包含 active、health_http_status=200、Redis PONG，发生在恢复定时器期限之前 |
| 告警 | 实际告警 {local(event['started_at'])} 触发，{local(event['resolved_at'])} 恢复；本轮未手工制造告警行 |
| 最终状态 | 新 Run succeeded；HTTP 200、服务 active；五项采集 healthy/fresh；连续120秒无丢弃、拒绝或入队失败；正式健康20/20 |

## 用量、预算与保留失败

平台目前记录 **{usage['request_count']} 次调用**（其中压缩 {usage['compaction_count']} 次），实报输入 **{usage['input_tokens']:,}**、输出 **{usage['output_tokens']:,}** Token，按配置价格累计 **{usage['amount']:.8f} 美元**。此数不包含兼容性探测，供应商实际账单以 APIMart 为准。参考价来源：[APIMart GLM 基础价格](https://apimart.ai/ru/model/llm/glm)。模型专属部门、用户月度额度均为1；本批按调用、Token 与十分钟 Run 窗口观察阈值停止，不自动增加额度。

初始 32K 配置曾被容量门禁正确拒绝，输入和草稿保留，尚未派发聊天模型调用。调整到官方支持范围内的128K后，首个只读 Run `{original['run_id']}` 在默认思考档位上发生 `CONTEXT_COMPACTION_FAILED`，失败压缩输出达到4096 Token上限。该失败和已消耗用量均保留；同模型低思考档位兼容性复测后，后续业务步骤完成。

**累计 `usage_complete` / `cached_usage_complete` 仍为 false**：聚合包含这条保留的失败调用，不将它改成成功。只读核对所有调用的输入、输出和缓存字段来源均为 provider，当前没有估算 Token；不是把未知缓存量补零。对话中的工具发现协议错误和模型自纠也保留，因此不声称所有中间调用一次成功。

几次浏览器失败为测试定位/等待问题：发布下载入口按报告标题命名；普通命令卡片不是旧完整计划布局；确认后状态投影短暂滞后时不能重复寻找确认按钮。测试已修正，使用原有已完成记录只读复查，未重放模型回复或再次执行修复命令。截图覆盖中英、明暗及1280/1440/1920桌面宽度。

## 环境与证据

新场景为一个 systemd/OpenSSH Host、一个受限 Redis 和实际 HTTP 缓存服务，合计2 CPU、1472 MiB；每5秒真实请求，最长8小时。主机共享 Docker VM 内核，硬件容量口径不代表独立物理机。Redis 应用账号仅允许本批键前缀，监控账号只允许 INFO/CONFIG GET/PING，不读取业务键。

Workspace 通过独立平台管理员正常配置本地 backend、固定摘要镜像及离线 Profile（500m CPU、512 MiB、600秒、配置网络none），企业最大并发1、月计算秒数3600。保留共享运行基座、账号和正式六个 PVC；本批未做业务 SQL 写入。清理 B11c 时修正的历史依赖投影已发布并通过真实正常卸载，未强制忘记未核实目标。

证据目录：`{run.out}`。重点是 `checks.json`、`complete-workflow-ledger.json`、`model-native-query-proof.json`、`model-analysis.json`、`model-workspace-report.md`、`model-cancellation-proof.json`、`model-repair-proof.json`、`model-usage-final.json`、`delivery-watch.json`、`formal-health.json` 及各 `browser-*`。`private/` 不公开或提交，密钥不进入 Markdown/截图/源码。

环境继续保留，自动自检不代替用户确认。收到下一批指令后按 [运行包](../runtime/B12/runbook.md) 归档、停止本批调用/负载、退役 Host/监控并销毁专属容器卷；正常保留聊天、交付文件、计费和审计历史，以及用户提供的模型配置和共享 Workspace 基座。
'''
(ROOT/'docs/plan9/batches/B12-review.md').write_text(text,encoding='utf-8');(run.out/'handoff.md').write_text(text,encoding='utf-8')
print('B12 actual conversation/report/monitoring/recovery route and billing caveats written',flush=True)
