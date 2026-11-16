"""Keep acceptance/cleanup and incomplete real-model validation explicit."""
import json
from manage import Run,ROOT,write_json,now

run=Run();old=run.out.parent/'p9-b11c-20261115-01';cleanup=json.loads((old/'cleanup-verification.json').read_text())
assert cleanup['status']=='destroyed'
previous=json.loads((old/'run.json').read_text());previous.update(status='destroyed',runtime_status='destroyed',human_acceptance='accepted',
    cleanup_record=str(old/'cleanup-verification.json'),retired_at=cleanup['at']);write_json(old/'run.json',previous)
path=ROOT/'docs/plan9/progress.json';data=json.loads(path.read_text(encoding='utf-8'))
parent=next(row for row in data['batches'] if row['id']=='B11');parent.update(status='B11c_accepted_and_retired',runtime_status='destroyed',review_status='B11c_accepted')
last=next(row for row in parent['sub_batches'] if row['id']=='B11c');last.update(status='destroyed',runtime_status='destroyed',review_status='accepted',human_acceptance='accepted',cleanup_record=str(old/'cleanup-verification.json'))
for item in data['runs']:
    if item['run_id']==previous['run_id']:item.update(status='destroyed',human_acceptance='accepted')
if not any(row.get('run_id')==previous['run_id'] for row in data['cleanup_records']):
    data['cleanup_records'].append({'run_id':previous['run_id'],'status':'destroyed','record':str(old/'cleanup-verification.json'),'at':cleanup['at']})
batch=next(row for row in data['batches'] if row['id']=='B12')
completed=(run.out/'checks.json').exists();status='ready_for_review' if completed else 'validating'
batch.update(status=status,package_status='created',self_check='passed' if completed else 'real_model_workflow_in_progress',
    runtime_status='running',review_status='pending',human_acceptance='pending',runs=[run.id],handoff='batches/B12-review.md')
if not any(row['run_id']==run.id for row in data['runs']):data['runs'].append({'run_id':run.id,'batch':'B12','status':status,'record':str(run.out/'run.json'),'human_acceptance':'pending'})
data.update(status=status,current_run_id=run.id,next_suggested_batch='B13a')
data['documentation'].update(review_entry='batches/B12-review.md',current_review='batches/B12-review.md')
data['delivery'].update(compose_packages='B01_through_B11c_retired_B12_running',deployment='B12_'+status,
    runtime_scripts='fresh_host_Redis_HTTP_native_monitoring_real_APIMart_conversation_and_Workspace',real_model_calls='actual_APIMart_glm_5.3_flash_in_progress' if not completed else 'actual_APIMart_glm_5.3_flash_verified',human_acceptance='B11c_accepted_B12_pending')
if data['current_runtime_validation'].get('run_id')!=run.id:
    data['runtime_validation_history'].append(data['current_runtime_validation'])
data['current_runtime_validation']=json.loads((run.out/'checks.json').read_text()) if completed else {'run_id':run.id,'batch':'B12','status':status,'at':now(),'human_acceptance':'pending','no_complete_delivery_claim':True}
data['baseline'].update(formal_version='local-20261116-b11c-retirement_Schema54',business_data='B11c_retired_fresh_B12_objects_created_through_normal_API')
write_json(path,data)
state=json.loads((run.out/'run.json').read_text());state.update(status=status,runtime_status='running',human_acceptance='pending');run.save('run',state)
print('B11c accepted/destroyed and B12 real-model validation registered separately',flush=True)
