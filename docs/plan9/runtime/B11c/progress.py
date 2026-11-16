"""Register deployment/verification separately from pending human acceptance."""
import json
from manage import Run,ROOT,write_json,now

run=Run();path=ROOT/'docs/plan9/progress.json';data=json.loads(path.read_text(encoding='utf-8'))
complete=(run.out/'checks.json').exists();status='ready_for_review' if complete else 'validating'
data.update(status=status,current_run_id=run.id,next_suggested_batch='B12')
data['documentation'].update(review_entry='batches/B11c-review.md',current_review='batches/B11c-review.md',issues='27-b11c-tunnel-reconnect-20261115.md')
data['delivery'].update(compose_packages='B01_through_B11b_retired_B11c_running_others_planned',
    runtime_scripts='B11c_real_tunnels_rotation_expiry_revocation_fault_recovery_browser_and_delivery_watch',
    deployment='B11c_ready_for_review' if complete else 'B11c_deployed_validating',human_acceptance='B11c_pending_user_review_B11b_retired_without_acceptance')
data['baseline'].update(formal_version='local-20261115-b11c-proof-r2_Schema54',
    source_state='main_eff32a1_plus_uncommitted_B11c_reconnect_proof_fix_runtime_and_reset_documentation',
    business_data='account_only_reset_completed_then_two_B11c_hosts_created_via_normal_product_flows',
    current_fix_release=str(run.out/'server-fix-release.json'))
if complete:
    data['validation'].update(scope='B11c_documentation_and_real_runtime',status='passed',report=str(run.out/'checks.json'),
        runtime_passed=True,runtime_result='B11c_ready_for_review_with_preserved_historical_gap_B11c_D01')
parent=next(row for row in data['batches'] if row['id']=='B11')
parent.update(status='B11c_'+status,package_status='B11a_B11b_B11c_created',runtime_status='running',self_check='B11c_passed' if complete else 'B11c_in_progress',review_status='B11c_pending')
item=next(row for row in parent['sub_batches'] if row['id']=='B11c')
item.update(status=status,package_status='created',runtime_status='running',self_check='passed' if complete else 'in_progress',
    review_status='pending',human_acceptance='pending',runs=[run.id],handoff='batches/B11c-review.md',evidence=str(run.out/'checks.json'))
if run.id not in parent['runs']:parent['runs'].append(run.id)
entry=next((row for row in data['runs'] if row['run_id']==run.id),None)
if entry is None:entry={'run_id':run.id,'batch':'B11c','record':str(run.out/'run.json')};data['runs'].append(entry)
entry.update(status=status,human_acceptance='pending')
previous=data.get('current_runtime_validation',{})
if previous.get('run_id')!=run.id and previous not in data['runtime_validation_history']:data['runtime_validation_history'].append(previous)
data['current_runtime_validation']=json.loads((run.out/'checks.json').read_text()) if complete else {'run_id':run.id,'batch':'B11c','status':status,'at':now(),'human_acceptance':'pending'}
write_json(path,data)
state=json.loads((run.out/'run.json').read_text());state.update(status=status,runtime_status='running',human_acceptance='pending');run.save('run',state)
print('Plan9 current batch registered as B11c '+status,flush=True)
