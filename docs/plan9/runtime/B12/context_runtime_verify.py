"""Normal product APIs and real model; retain original B12 and verify a new chat."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import time

from manage import Run, ROOT, now
from common.api import Api

OUT = Path('C:/Users/23574/.codex/artifacts/argus-b12-generic-runtime-20261116/b12-native')
OWNER = 'argus-b12-generic-runtime-20261116'
STATE = OUT/'state.json'


class Verification(Run):
    def __init__(self):
        self.id=OWNER;self.out=OUT;self.private=OUT/'private';self.resources=STATE
        self.private.mkdir(parents=True,exist_ok=True)
        subprocess.run(['icacls',str(self.private),'/inheritance:r','/grant:r',subprocess.check_output(['whoami'],text=True).strip()+':(OI)(CI)F','*S-1-5-18:(OI)(CI)F'],capture_output=True,check=True)
        path=self.private/'session.json'
        if not path.exists():path.write_text(json.dumps({'origin':'https://argus.dev','csrf':'','user':{},'cookies':[]}),encoding='utf-8')
        self.api=Api(self,path,ROOT/'deploy/.cache/argus-trust-bundle.pem',{'enterprise_username':'kakj','password':os.environ['ARGUS_PLAN9_PASSWORD']})
        self.api.login()
        self.state=json.loads(STATE.read_text(encoding='utf-8')) if STATE.exists() else {}

    def save_state(self):self.save('state',self.state)

    def usage(self):
        rows=self.api.call('GET','/enterprise/model-usage');rows=rows['items'] if isinstance(rows,dict) else rows
        return next(x for x in rows if x['model_id']==self.state['model_id'])

    def budget(self,run=None):
        usage=self.usage();base=self.state['baseline_usage']
        if usage['request_count']-base['request_count']>=self.state.get('verification_call_budget',60) or float(usage['amount'])-float(base['amount'])>=.20 or usage['input_tokens']+usage['output_tokens']-base['input_tokens']-base['output_tokens']>=2000000:
            if run:self.api.call('POST','/runs/'+run+'/cancel',key='budget-stop-'+run)
            raise RuntimeError('Finite native model verification budget reached; quota was not increased')
        return usage

    def events(self):
        result=[];cursor=0
        for _ in range(20):
            rows=self.api.call('GET','/conversations/'+self.state['conversation_id']+'/ledger?limit=200&cursor='+str(cursor))['items'];result+=rows
            if len(rows)<200:return result
            assert rows[-1]['sequence']>cursor;cursor=rows[-1]['sequence']
        raise RuntimeError('Native ledger budget exhausted; do not silently truncate')

    def send(self,stage,prompt,files=None):
        if self.state.get(stage+'_run'):return self.state[stage+'_run']
        self.budget();body={'content':prompt,'file_ids':files or []}
        trial=self.api.call('POST','/conversations/'+self.state['conversation_id']+'/preflight',body)
        assert trial['ready'],trial.get('error_code')
        result=self.api.call('POST','/conversations/'+self.state['conversation_id']+'/messages',body,key=stage+'-message-'+self.state['conversation_id'],expected=202)
        ident=result['run']['run_id'];self.state[stage+'_run']=ident;self.state[stage+'_prompt']=prompt;self.save_state();return ident

    def wait_run(self,ident):
        for index in range(180):
            run=self.api.call('GET','/runs/'+ident)
            if run['status'] in ['succeeded','failed','cancelled','timed_out']:
                self.save('run-'+ident,run);assert run['status']=='succeeded',(run['status'],run.get('error_code'),run.get('stop_reason'));return run
            self.budget(ident)
            if index%5==0:print('Native Run '+ident+': '+run['status']+'; total calls='+str(self.usage()['request_count']),flush=True)
            time.sleep(3)
        self.api.call('POST','/runs/'+ident+'/cancel',key='timeout-stop-'+ident)
        raise RuntimeError('Native verification Run time window exhausted')


def prepare(v):
    if v.state.get('conversation_id'):return
    original=Run();mapping=json.loads(original.resources.read_text(encoding='utf-8'))
    v.state['model_id']=mapping['model']['id'];v.state['baseline_usage']=v.usage();v.state['original_conversation_id']=mapping['conversation']['id']
    chat=v.api.call('POST','/conversations',{'title':'B12 通用上下文与压缩恢复 · 正式复验','selected_model_id':v.state['model_id']},key='new-native-context-verification',expected=201)
    v.state['conversation_id']=chat['id'];v.state['started_at']=now();v.state['mapping']={k:mapping[k] for k in ['hosts','targets','dashboards']};v.save_state()
    rows=v.api.call('GET','/conversations/'+mapping['conversation']['id']+'/workspace/files');rows=rows['items'] if isinstance(rows,dict) else rows
    original_file=next(x for x in rows if x['name']=='b12-native-redis.csv')
    response=v.api.http.get(v.api.origin+'/api/v1/conversations/'+mapping['conversation']['id']+'/workspace/files/'+original_file['id']+'/content',timeout=45);response.raise_for_status();body=response.content
    assert hashlib.sha256(body).hexdigest()==original_file['content_hash']
    upload=v.api.call('POST','/conversations/'+chat['id']+'/workspace/uploads',{'name':original_file['name'],'byte_size':len(body)},key='same-native-csv',expected=201)
    response=v.api.http.put(v.api.origin+'/api/v1/conversations/'+chat['id']+'/workspace/uploads/'+upload['id']+'/content',data=body,headers={'X-CSRF-Token':v.api.csrf,'Idempotency-Key':'context-runtime-csv-content','Content-Type':'application/octet-stream'},timeout=120)
    assert response.status_code in [200,201],response.status_code
    v.state['csv_file']=response.json();v.state['csv_sha256']=original_file['content_hash'];v.save_state()
    v.save('native-verification-budget',{'new_provider_calls_max':40,'new_actual_tokens_max':2000000,'new_configured_amount_max':.20,'native_monthly_quota_increased':False,'original_chat_mutated':False})
    print('New native conversation and identical original CSV prepared: '+chat['id'],flush=True)


def query(v):
    prompt=('B12 相同场景的正式复验，只读取已发布面板，不搜索其他资源、不执行主机命令、不修改配置。'
      '对象为 B12-Cache-Host（host:01a1e1dd-d568-724d-8d0b-40098d812b43）与 B12-Redis（monitoring_target:3b24204d-9d9d-5c2a-b3fa-e37d621183a8）。'
      'Host 仪表盘 cee08ea9-0beb-5c7b-a006-e27d11e61d39，取 CPU 使用率、内存使用率、已用内存3个面板；'
      '固定 UTC 窗口 2026-11-16T01:51:50.866700613Z 至 2026-11-16T01:56:50.866700613Z。'
      'Redis 仪表盘 6f0cd210-ab49-5d65-be71-f63eb4535beb，取内存、命令速率、命中率3个面板；'
      '固定 UTC 窗口 2026-11-16T01:53:30.988110026Z 至 2026-11-16T01:58:30.988110026Z。'
      '这是保留历史窗口的复验，不评价当前健康。本阶段仅完成真实取数，等待两个任务文件交付齐全后简短告知，暂不读取文件计算、不导出报告。后续消息会继续分析，避免重复取数。')
    ident=v.send('query',prompt);v.wait_run(ident)
    events=v.events();v.save('ledger-after-query',{'items':events})
    jobs=[e['payload']['projection']['summary'] for e in events if e.get('run_id')==ident and e['event_type']=='tool_call_result' and e['payload'].get('target_tool_id')=='telemetry.dashboard.query.get' and e['payload']['status']=='succeeded' and e['payload']['projection']['summary'].get('status')=='complete']
    unique={x['id']:x for x in jobs};assert len(unique)==2,'Both native query tasks must really complete'
    assert sum(len([f for f in x['files'] if f['kind']=='data']) for x in unique.values())==6
    v.state['jobs']=list(unique.values());v.save_state();print('Two native jobs and six data files complete',flush=True)


def kube(*args,data=None):
    p=subprocess.run(['kubectl','--context','docker-desktop',*args],input=data,capture_output=True,text=True,encoding='utf-8',timeout=120)
    if p.returncode:raise RuntimeError('Kubernetes verification operation failed: '+p.stderr[:800])
    return p.stdout.strip()


def compact_and_analyze(v):
    if v.state.get('analysis_completed'):return
    worker=json.loads(kube('get','deployment','argus-worker','-n','argus-system','-o','json'))
    (v.private/'worker-before-controlled-compaction.json').write_text(json.dumps(worker),encoding='utf-8')
    job_name='argus-b12-context-compactor-verify';job_uid=None
    active=kube('exec','-n','argus-system','argus-postgresql-0','--','psql','-U','argus','-d','argus','-Atq','-c',"SELECT count(*) FROM runs WHERE status IN ('pending','running','waiting_input','waiting_approval','waiting_system')")
    assert active=='0','Do not pause an executor serving another active Run'
    try:
        kube('scale','deployment/argus-worker','-n','argus-system','--replicas=0')
        pods=json.loads(kube('get','pods','-n','argus-system','-l','app.kubernetes.io/name=argus-worker','-o','json'))['items']
        if pods:kube('wait','pod','-n','argus-system','-l','app.kubernetes.io/name=argus-worker','--for=delete','--timeout=90s')
        prompt=('继续这个 B12 只读分析任务。刚才两份查询任务已经完成，不能重复取数，也不能重新查询指标或扫描其他资源。'
          '请使用当前 Workspace 中已交付的6份真实面板数据和各自 manifest，实际执行离线代码计算每份数据的样本数、最小值、最大值、末值，保留资源、UTC时间范围及单位。'
          '同时实际用代码读取 b12-native-redis.csv：最后减最初 total_commands_processed 得到 command_delta，used_memory_bytes 最大值为 peak_memory_bytes；监控命令包含在增量中，不能当纯业务吞吐。'
          '写入 /workspace/b12-context-verification.md，报告包含来源和计算过程、6个面板的完整统计 JSON（每项包含 panel_id、target_id、point_count、value_min、value_max、last_value）以及CSV两个原始整数字段，并发布为可下载文件。'
          '最后简短总结并引用真实交付。具体取证和计算方式由你决定；不执行任何主机命令、不改配置、不重复发现相同资源。')
        ident=v.send('analysis',prompt,[v.state['csv_file']['id']])
        before_compaction=max(e['sequence'] for e in v.events())
        v.api.call('POST','/runs/'+ident+'/compact',key='forced-native-current-run-compaction',expected=202)
        spec=worker['spec']['template']['spec'];spec['restartPolicy']='Never';spec['terminationGracePeriodSeconds']=3
        for c in spec['containers']:
            if c['name']=='argus-worker' or c['name']=='worker' or c['command'][0].endswith('argus-worker'):
                c['command']=['/usr/local/bin/argus-worker'];c['args']=['--pool=compaction']
        job={'apiVersion':'batch/v1','kind':'Job','metadata':{'name':job_name,'namespace':'argus-system','labels':{'io.argus.plan9.run':OWNER}},
             'spec':{'backoffLimit':0,'activeDeadlineSeconds':240,'template':{'metadata':{'labels':{'app.kubernetes.io/name':job_name,'io.argus.plan9.run':OWNER}},'spec':spec}}}
        kube('create','-f','-',data=json.dumps(job));job_uid=json.loads(kube('get','job',job_name,'-n','argus-system','-o','json'))['metadata']['uid']
        for index in range(80):
            events=v.events();compacted=[e for e in events if e['event_type']=='context_compacted' and e['sequence']>before_compaction]
            if compacted:
                event=compacted[-1];through=event['payload']['source_through_sequence']
                registered=[e['sequence'] for e in events if e['event_type']=='workspace_file_added']
                assert through>=max(registered),'The source prefix must cover the delivered files'
                v.state['forced_snapshot']=event['payload'];v.state['forced_snapshot_event_sequence']=event['sequence'];v.save_state();break
            v.budget();time.sleep(2)
        else:raise RuntimeError('Controlled native compaction did not complete in its bounded window')
    finally:
        if job_uid:
            actual=json.loads(kube('get','job',job_name,'-n','argus-system','-o','json'))
            assert actual['metadata']['uid']==job_uid and actual['metadata']['labels']['io.argus.plan9.run']==OWNER
            kube('delete','job',job_name,'-n','argus-system','--wait=true','--timeout=60s')
        current=json.loads(kube('get','deployment','argus-worker','-n','argus-system','-o','json'));assert current['metadata']['uid']==worker['metadata']['uid']
        kube('scale','deployment/argus-worker','-n','argus-system','--replicas='+str(worker['spec']['replicas']))
        kube('rollout','status','deployment/argus-worker','-n','argus-system','--timeout=90s')
    v.wait_run(v.state['analysis_run']);v.state['analysis_completed']=True;v.save_state();v.save('ledger-after-analysis',{'items':v.events()});v.save('usage-after-analysis',v.usage())
    print('Actual native analysis after controlled compaction completed',flush=True)


if __name__=='__main__':
    parser=argparse.ArgumentParser();parser.add_argument('phase',choices=['prepare','query','analysis']);args=parser.parse_args();verification=Verification()
    {'prepare':prepare,'query':query,'analysis':compact_and_analyze}[args.phase](verification)
