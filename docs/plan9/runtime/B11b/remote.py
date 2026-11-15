"""Normal, narrowly scoped access to the real Bastion and private-member PTYs."""
from datetime import datetime,timedelta,timezone
from integration import Batch
from manage import Run


def configure(batch):
    api=batch.api;state=batch.map.setdefault('remote',{});accounts=state.setdefault('accounts',{})
    for key in ['bastion','member']:
        if not accounts.get(key):
            row=api.call('POST','/enterprise/managed-accounts',{'host_id':batch.map['hosts'][key]['host_id'],
                'username':'root','privilege_level':'administrator','credential_id':batch.map['hosts'][key]['credential_id'],
                'allowed_protocols':['shell','ssh']},key=key+'-root-account',expected=201)
            accounts[key]=row['id'];batch.save()
    if not state.get('grant_id'):
        now=datetime.now(timezone.utc)
        row=api.call('POST','/enterprise/remote-access-grants',{'subject_type':'user','subject_id':api.user['id'],
            'host_ids':[batch.map['hosts'][key]['host_id'] for key in accounts],
            'managed_account_ids':list(accounts.values()),'protocols':['shell','ssh'],'actions':['terminal'],
            'valid_from':(now-timedelta(minutes=1)).isoformat(),'valid_until':(now+timedelta(days=7)).isoformat(),'status':'draft'},
            key='b11b-review-terminal-grant',expected=201)
        state.update(grant_id=row['id'],user_id=api.user['id'],valid_until=row['valid_until']);batch.save();api.login()
    grant=api.call('GET','/enterprise/remote-access-grants/'+state['grant_id'])
    if grant['status']=='draft':
        grant=api.call('POST','/enterprise/remote-access-grants/'+grant['id']+'/enable?expected_version='+str(grant['version']));api.login()
    assert grant['status']=='enabled'
    batch.run.save('remote-authorization',{'accounts':accounts,'grant':grant,'global_rules_added':0,
        'recording':'required','command_audit':'required','file_upload':'disabled','clipboard':'disabled'})
    print('Bastion/member accounts and user/host-bound seven-day terminal grant enabled',flush=True)


if __name__=='__main__':
    run=Run();api=run.api();api.login();configure(Batch(run,api))
