"""Local documentation references and size/format gate for the delivered batch."""
import json,re,subprocess
from pathlib import Path
from urllib.parse import unquote
from run import Run,ROOT
r=Run();paths=['docs/README.md','docs/plan9/README.md','docs/plan9/01-batch-catalog.md','docs/plan9/batches/B10a-review.md','docs/plan9/batches/B10b-review.md','docs/plan9/runtime/B10b/runbook.md','docs/plan9/19-b10b-storage-jobs-and-node-ca-20261112.md','docs/plan8/23-fixed-own-metric-instances.md']
for name in paths:
 path=ROOT/name;text=path.read_text(encoding='utf-8')
 for target in re.findall(r'\]\(([^)]+)\)',text):
  if '://' in target or target.startswith('#'):continue
  local=unquote(target.split('#',1)[0].split('?',1)[0].strip('<>'))
  assert (path.parent/local).resolve().exists(),(name,target)
progress=json.loads((ROOT/'docs/plan9/progress.json').read_text(encoding='utf-8'))
assert progress['current_run_id']==r.id and progress['next_suggested_batch']=='B10c'
assert next(x for x in progress['batches'] if x['id']=='B10')['runtime_status']=='B10b_running'
files=subprocess.check_output(['git','diff','--name-only','--diff-filter=ACM'],cwd=ROOT,text=True).splitlines()
files+=subprocess.check_output(['git','ls-files','--others','--exclude-standard'],cwd=ROOT,text=True).splitlines()
for name in files:
 path=ROOT/name
 if path.suffix in ('.go','.ts','.tsx') and path.is_file():assert len(path.read_text(encoding='utf-8').splitlines())<=2000,name
assert subprocess.run(['git','diff','--check'],cwd=ROOT,capture_output=True).returncode==0
r.save('documentation-gate',{'passed':True,'documents':len(paths),'local_links_exist':True,'source_file_line_limits':True,'diff_check':True,'current':'B10b','next':'B10c','human_acceptance':'pending'})
print('Review links, current/next batch, source size and diff checks passed',flush=True)
