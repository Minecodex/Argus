"""User- and host-scoped normal remote terminal authorization."""
from datetime import datetime,timedelta,timezone
from integration import Batch
from manage import Run


def configure(batch):
    api=batch.api;state=batch.map.setdefault('remote',{})
    if not state.get('account_id'):
        account=api.call('POST','/enterprise/managed-accounts',{
            'host_id':batch.host,'username':'root','privilege_level':'administrator',
            'credential_id':batch.map['hosts']['direct']['credential_id'],
            'allowed_protocols':['shell','ssh']},key='b11a-root-account',expected=201)
        state['account_id']=account['id'];batch.save()
    if not state.get('grant_id'):
        now=datetime.now(timezone.utc)
        grant=api.call('POST','/enterprise/remote-access-grants',{
            'subject_type':'user','subject_id':api.user['id'],'host_ids':[batch.host],
            'managed_account_ids':[state['account_id']],'protocols':['shell','ssh'],'actions':['terminal'],
            'valid_from':(now-timedelta(minutes=1)).isoformat(),'valid_until':(now+timedelta(days=7)).isoformat(),
            'status':'draft'},key='b11a-review-grant',expected=201)
        state.update(grant_id=grant['id'],user_id=api.user['id'],valid_until=grant['valid_until']);batch.save()
    grant=api.call('GET','/enterprise/remote-access-grants/'+state['grant_id'])
    if grant['status']=='draft':
        grant=api.call('POST','/enterprise/remote-access-grants/'+grant['id']+'/enable?expected_version='+str(grant['version']))
    assert grant['status']=='enabled',grant
    batch.run.save('remote-authorization',{'account_id':state['account_id'],'grant':grant,
        'global_rules_added':0,'recording':'required by built-in default','command_audit':'required by built-in default'})
    print('Root managed account and seven-day host/user-scoped terminal grant ready',flush=True)


if __name__=='__main__':
    run=Run();api=run.api();api.login();configure(Batch(run,api))
