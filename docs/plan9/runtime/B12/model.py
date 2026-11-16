"""Normal tested model registration and native, non-unlimited quota enforcement."""
import json
import requests
from manage import Run,now

NAME='B12 APIMart GLM-5.3-Flash'
BASE='https://api.apimart.ai/v1'

def configure(run,api):
    budget={'provider':'APIMart','model':'glm-5.3-flash','base_url':BASE,'max_user_messages':5,
        'one_extra_read_only_step_after_model_omitted_metrics':True,
        'max_model_calls_per_run':24,'max_compatibility_attempts':3,'max_provider_calls_including_compatibility':108,
        'reasoning_effort':'low','configuration_retest_uses_existing_total_call_and_amount_budget':True,
        'context_window_cap':128000,'output_token_cap_per_call':4096,'planned_total_tokens_limit':3500000,
        'native_usage_call_watch_threshold':96,'native_usage_token_watch_threshold':3200000,'compaction_included_in_usage_watch':True,
        'native_department_and_user_monthly_amount_cap':1,'price_unit':'USD per million tokens; APIMart account charged by its own bill',
        'input_price_per_million':0.075,'output_price_per_million':0.25,'run_timeout_seconds':600,
        'automatic_budget_increase':False,'pricing_source':'https://apimart.ai/ru/model/llm/glm',
        'authorization':'User explicitly supplied APIMart key/model and requested the real complete flow; this bounded test uses no additional model/provider'}
    run.save('model-budget',budget)
    models=api.call('GET','/enterprise/ai-models')['items'];existing=next((row for row in models if row['name']==NAME),None)
    if not existing:
        secret=(run.private/'apimart-key.txt').read_text(encoding='utf-8').strip()
        assert secret and '\n' not in secret
        response=api.call('POST','/enterprise/ai-models/test-and-create',{'name':NAME,'base_url':BASE,'model_id':'glm-5.3-flash',
            'api_protocol':'chat_completions','api_key':secret,'reasoning_effort':'provider_default',
            'context_window_tokens':128000,'max_output_tokens':4096,'input_price_per_million':0.075,'output_price_per_million':0.25},
            key='apimart-compatibility-create-v1',expected=201,timeout=240)
        run.save('model-compatibility',response)
        assert response['compatible'] and response.get('model'),'Real compatibility failed; no usable model fabricated'
        existing=response['model']
    if existing['context_window_tokens']<128000:
        run.save('model-before-capacity-correction',existing)
        existing=api.call('PUT','/enterprise/ai-models/'+existing['id'],{'context_window_tokens':128000,'expected_version':existing['version']},
            key='official-supported-context-cap',timeout=240)
        run.save('model-capacity-corrected',existing)
    if existing['capabilities'].get('reasoning_effort')!='low':
        existing=api.call('PUT','/enterprise/ai-models/'+existing['id'],{'reasoning_effort':'low','expected_version':existing['version']},
            key='bounded-low-reasoning-profile',timeout=240)
        run.save('model-low-reasoning-tested',existing)
    assert existing['model_id']=='glm-5.3-flash' and existing['base_url'].rstrip('/')==BASE and existing['status']=='enabled' and existing['health_status']=='healthy'
    mapping=json.loads(run.resources.read_text());mapping['model']={'id':existing['id'],'name':NAME,'model_id':existing['model_id'],'revision':existing['revision']};run.save('resource-map',mapping)
    context=api.call('GET','/enterprise/auth/session');department=api.user.get('department_id') or context['session']['department_id']
    quotas=api.call('GET','/enterprise/model-quotas')
    for kind,ident in [('department',department),('user',api.user['id'])]:
        previous=next((row for row in quotas if row['model_id']==existing['id'] and row['subject_type']==kind and row['subject_id']==ident),None)
        body={'model_id':existing['id'],'subject_type':kind,'subject_id':ident,'monthly_amount':1}
        if previous:body['expected_version']=previous['version']
        api.call('POST','/enterprise/model-quotas',body,key='bounded-b12-quota-'+kind)
    run.save('model-selected',{'at':now(),'model':mapping['model'],'quotas':api.call('GET','/enterprise/model-quotas'),
        'native_price_snapshot':{'input':existing['input_price_per_million'],'output':existing['output_price_per_million']},'key_printed':False})
    print('Actual APIMart compatibility passed; model registered with finite department/user quotas',flush=True)

if __name__=='__main__':
    run=Run();api=run.api();api.login();configure(run,api)
