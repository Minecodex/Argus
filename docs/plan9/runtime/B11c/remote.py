"""Narrow manual access; no new human identities or global access rules."""
from datetime import datetime,timedelta,timezone
from integration import Batch
from manage import Run

def configure(batch):
    api=batch.api;state=batch.map.setdefault('remote',{});accounts=state.setdefault('accounts',{})
    for key in ['bastion','member']:
        if key not in accounts:
            row=api.call('POST','/enterprise/managed-accounts',{'host_id':batch.map['hosts'][key]['host_id'],'username':'root',
                'privilege_level':'administrator','credential_id':batch.map['hosts'][key]['credential_id'],'allowed_protocols':['shell','ssh']},key=key+'-account',expected=201)
            accounts[key]=row['id'];batch.save()
    if not state.get('grant_id'):
        now=datetime.now(timezone.utc)
        grant=api.call('POST','/enterprise/remote-access-grants',{'subject_type':'user','subject_id':api.user['id'],
            'host_ids':[batch.map['hosts'][key]['host_id'] for key in accounts],'managed_account_ids':list(accounts.values()),
            'protocols':['shell','ssh'],'actions':['terminal'],'valid_from':(now-timedelta(hours=1)).isoformat(),
            'valid_until':(now+timedelta(days=7)).isoformat(),'status':'draft'},key='review-terminal-grant',expected=201)
        state.update(grant_id=grant['id'],user_id=api.user['id'],valid_until=grant['valid_until']);batch.save();api.login()
    grant=api.call('GET','/enterprise/remote-access-grants/'+state['grant_id'])
    if grant['status']=='draft':
        grant=api.call('POST','/enterprise/remote-access-grants/'+grant['id']+'/enable?expected_version='+str(grant['version']));api.login()
    assert grant['status']=='enabled'
    batch.run.save('remote-authorization',{'accounts':accounts,'grant':grant,'new_global_rules':0,'new_human_users':0,
        'recording':'required','command_audit':'required','file_upload':'disabled','clipboard':'disabled'})
    print('Actual host/account-scoped Shell and SSH grant enabled; recording and command audit required',flush=True)

def close_tests(batch):
    api=batch.api;ids={row['host_id'] for row in batch.map['hosts'].values()}
    for row in api.call('GET','/enterprise/remote-access-sessions?scope=active')['items']:
        if row['host_id'] not in ids or not row.get('reason','').startswith('B11c '):continue
        api.call('POST','/enterprise/remote-access-sessions/'+row['id']+'/terminate',{'reason':'B11c owned automated review cleanup'},key='cleanup-'+row['id'])
        api.wait('/enterprise/remote-access-sessions/'+row['id'],lambda value:value['status']=='terminated',timeout=90)
    grant=api.call('GET','/enterprise/remote-access-grants/'+batch.map['remote']['grant_id'])
    if grant['status']=='disabled':
        api.call('POST','/enterprise/remote-access-grants/'+grant['id']+'/enable?expected_version='+str(grant['version']));api.login()
    batch.run.save('owned-test-session-cleanup',{'closed':True,'grant_restored_enabled':True})

if __name__=='__main__':
    import argparse
    parser=argparse.ArgumentParser();parser.add_argument('action',choices=['configure','close_tests'],nargs='?',default='configure');args=parser.parse_args()
    run=Run();api=run.api();api.login();globals()[args.action](Batch(run,api))
