"""Final public documentation, source size and review-ready handoff evidence."""
from pathlib import Path
from datetime import datetime,timezone
import json,re,subprocess
from urllib.parse import unquote
ROOT=Path.cwd();out=ROOT/'artifacts/plan9/p9-b10a-20261111-01'
paths=['docs/README.md','docs/plan8/79-unverified-namespace-attribution.md','docs/plan9/README.md','docs/plan9/01-batch-catalog.md','docs/plan9/batches/B09b-review.md','docs/plan9/batches/B10a-review.md','docs/plan9/runtime/B10a/runbook.md','docs/plan9/15-b10a-workbench-lifecycle-20261111.md','docs/plan9/16-b10a-pod-logs-and-terminal-20261111.md','docs/plan8/47-pod-log-sessions.md','docs/plan9/17-b10a-quiet-pod-logs-and-actions-20261111.md','docs/plan9/18-b10a-terminal-matrix-and-log-options-20261111.md']
links=0;errors=[]
for relative in paths:
 file=ROOT/relative;text=file.read_text(encoding='utf-8');assert '\ufffd' not in text
 for target in re.findall(r'\]\(([^)]+)\)',text):
  if target.startswith(('http:','https:','#','mailto:')):continue
  target=unquote(target.strip('<>').split('#',1)[0].split('?',1)[0])
  links+=1
  if not (file.parent/target).exists():errors.append({'file':relative,'target':target})
source=['internal/kubernetesinventory/manifest_projection.go','internal/kubernetesinventory/manifest_projection_test.go','internal/kubernetesinventory/projection.go','internal/kubernetesworkbench/mutation.go','internal/kubernetesworkbench/stream_authorization.go','internal/kubernetesworkbench/stream_sessions_integration_test.go','internal/pagination/cursor.go','internal/pagination/id_cursor_test.go','internal/telemetry/namespace_metric_identity.go','internal/telemetry/namespace_metric_identity_integration_test.go','internal/telemetry/namespace_attribution_test.go','web/apps/enterprise/e2e/plan9-b10a-real.spec.ts','web/apps/enterprise/e2e/plan9-b10a-dashboards-real.spec.ts','web/apps/enterprise/src/components/kubernetes/log-session-dialog.tsx','web/apps/enterprise/src/components/kubernetes/use-pod-log-session.ts','web/apps/enterprise/src/components/kubernetes/stream-session-dialog.tsx','web/packages/ui/src/log-viewer.tsx','internal/kubernetespodlogs/read.go','internal/kubernetespodlogs/initial_tail.go','internal/kubernetesworkbench/log_sessions.go']
sizes={name:len((ROOT/name).read_text(encoding='utf-8').splitlines()) for name in source};assert all(x<=2000 for x in sizes.values())
assert not errors,errors
progress=json.loads((ROOT/'docs/plan9/progress.json').read_text(encoding='utf-8'));assert progress['current_run_id']==out.name and progress['next_suggested_batch']=='B10b'
checks=json.loads((out/'checks.json').read_text(encoding='utf-8'));assert checks['passed']
record={'passed':True,'utf8_files':len(paths),'local_links_checked':links,'broken_links':errors,'source_line_counts':sizes,'current_run':progress['current_run_id'],'human_acceptance':'pending','recorded_at':datetime.now(timezone.utc).isoformat()}
(out/'documentation-validation.json').write_text(json.dumps(record,indent=2),encoding='utf-8')
handoff=f"""# B10a 可查看

已清理 B09b，B10a 环境保留，等待用户验收。

- [资源工作台](https://argus.dev/kubernetes/{checks['cluster_id']}?tab=resources)，Namespace 选择 b10a-workloads。
- [四个仪表盘](https://argus.dev/kubernetes/{checks['cluster_id']}?tab=dashboards)，23 个面板都有有效数据。
- [查看路线、告警时间与事实边界](../../../docs/plan9/batches/B10a-review.md)。
- [最终自检](./checks.json) 与 [完整投递观察](./delivery-watch.json)。

三项原生采集、真实操作和告警触发/恢复、日志、终端、CLI 转发、6 项部署页面流程通过。修复后 311.8 秒投递失败增量为 0；修复前 133 点缺口和对应事件保留。正式健康 20 项通过，6 个原有 PVC 绑定保留。

人工验收仍 pending；下一批是 B10b，未自动部署。
"""
(out/'handoff.md').write_text(handoff,encoding='utf-8')
print(json.dumps({'passed':True,'files':len(paths),'links':links,'largest_changed_source':max(sizes.values())},indent=2))
