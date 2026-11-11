"""Remove only repeated B10a browser fixture attempts; keep one fixture per UI variant."""
from datetime import datetime,timezone
import json
from run import Run,NAMESPACE
r=Run();a=r.api();mapping=json.loads(r.resources.read_text(encoding='utf-8'))
assert r.get('namespace',NAMESPACE)['metadata']['uid']==mapping['namespaces'][NAMESPACE]['uid']
items=r.get('configmaps','-n',NAMESPACE,'-l','plan9_batch=b10a')['items'];removed=[];kept={}
for variant in ('zh-light','zh-dark','en-light','en-dark'):
 rows=[x for x in items if x['metadata']['name'].startswith('b10a-ui-'+variant)]
 assert rows,variant
 rows.sort(key=lambda x:x['metadata']['creationTimestamp'],reverse=True)
 keep=rows[0];kept[variant]={'name':keep['metadata']['name'],'uid':keep['metadata']['uid']}
 assert keep['data']['mode']=='browser-updated'
 for row in rows[1:]:
  assert row['metadata']['namespace']==NAMESPACE and row['metadata']['labels']['plan9_batch']=='b10a'
  r.change(a,'configmap',row['metadata']['name'],'delete');removed.append({'name':row['metadata']['name'],'uid':row['metadata']['uid']})
mapping['ui_configmaps']=kept;r.save('resource-map',mapping)
r.save('browser-fixture-cleanup',{'kept':kept,'removed':removed,'normal_preview_confirm':True,'recorded_at':datetime.now(timezone.utc).isoformat()})
print('Kept the four successful desktop variants; removed only repeated owned browser fixtures',flush=True)
