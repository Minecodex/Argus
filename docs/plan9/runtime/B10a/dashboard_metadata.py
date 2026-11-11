"""Correct the batch caption through the ordinary dashboard draft/publish flow."""
import json
from run import Run
DESCRIPTION='B10a Kubernetes 工作台真实观测 / Real Kubernetes workbench observations'
r=Run();a=r.api();mapping=json.loads(r.resources.read_text(encoding='utf-8'));board=mapping['dashboards']['workloads'];path='/dashboards/'+board['id']
before=a.call('GET',path);original=before['revision']['spec']
if before['dashboard']['description']!=DESCRIPTION:
 draft=a.call('POST','/dashboard-drafts',{'dashboard_id':board['id'],'name':before['dashboard']['name'],'description':DESCRIPTION,'spec':original,'proposed_bindings':[{'resource_type':'kubernetes_cluster','resource_id':r.cluster_id}]},key='correct-batch-caption-create-v2',expected=201)
 saved=a.call('PATCH','/dashboard-drafts/'+draft['id'],{'expected_version':draft['draft_version'],'dashboard_id':board['id'],'name':before['dashboard']['name'],'description':DESCRIPTION,'spec':original,'proposed_bindings':[{'resource_type':'kubernetes_cluster','resource_id':r.cluster_id}]})
 a.confirm(a.call('POST','/dashboard-drafts/'+draft['id']+'/preview',{'expected_version':saved['draft_version']},key='correct-batch-caption-publish',expected=201))
 board['draft_id']=draft['id'];r.save('resource-map',mapping)
after=a.call('GET',path);assert after['dashboard']['description']==DESCRIPTION and after['revision']['spec']==original
r.save('dashboard-caption-update',{'dashboard_id':board['id'],'description':DESCRIPTION,'revision_number':after['revision']['revision_number'],'spec_preserved':True,'normal_preview_confirm':True})
print('Correct batch description published without changing the dashboard panels',flush=True)
