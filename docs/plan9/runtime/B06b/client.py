"""Actual Elasticsearch indexing/search and bounded, reversible scenarios."""
import argparse
import base64
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime,timezone
import json
from pathlib import Path
import signal
import time
import urllib.error
import urllib.request
import uuid

CREDS=json.loads(Path('/var/lib/plan9/private/elasticsearch.json').read_text())
INDEX='p9-business'
STOP=False
def stop(*_):
    global STOP
    STOP=True
signal.signal(signal.SIGTERM,stop)
signal.signal(signal.SIGINT,stop)
def now():return datetime.now(timezone.utc).isoformat()
def http(side,method,path,body=None,role='app',allowed=(200,201)):
    assert side in ['a','b'] and role in CREDS[side] and path.startswith('/')
    data=body if isinstance(body,bytes) else json.dumps(body).encode() if body is not None else None
    request=urllib.request.Request('http://elasticsearch-'+side+':9200'+path,data=data,method=method)
    request.add_header('Authorization','Basic '+base64.b64encode((role+':'+CREDS[side][role]).encode()).decode())
    request.add_header('Content-Type','application/x-ndjson' if isinstance(body,bytes) else 'application/json')
    try:
        with urllib.request.build_opener(urllib.request.ProxyHandler({})).open(request,timeout=20) as response:status,raw=response.status,response.read(8388608)
    except urllib.error.HTTPError as error:status,raw=error.code,error.read(4096)
    assert status in allowed,(method,path,status)
    return json.loads(raw) if raw and status<400 else {'status':status}
def record(name,**values):
    p=Path('/var/lib/plan9/'+name+'.json');tmp=p.with_suffix('.tmp');tmp.write_text(json.dumps({'observed_at':now(),**values}));tmp.replace(p)
def document(side,index):return {'group':side,'sequence':index,'counter':0,'text':'actual searchable Elasticsearch business record '+side+' '+str(index)}
def bulk(side,ids,delete=False):
    lines=[]
    for i,key in enumerate(ids):
        lines.append(json.dumps({('delete' if delete else 'index'):{'_index':INDEX,'_id':key}}))
        if not delete:lines.append(json.dumps(document(side,i)))
    result=http(side,'POST','/_bulk?refresh=true',('\n'.join(lines)+'\n').encode())
    assert len(result['items'])==len(ids)
    for item in result['items']:
        value=next(iter(item.values()));assert 'error' not in value and value['status'] in ([200,404] if delete else [200,201])
    return len(ids)
def concurrent_bulk(side,prefix,total,delete=False):
    assert total<=200
    chunks=[[prefix+str(i) for i in range(start,min(start+50,total))] for start in range(0,total,50)]
    with ThreadPoolExecutor(max_workers=4) as pool:return sum(f.result() for f in [pool.submit(bulk,side,chunk,delete) for chunk in chunks])
def replicas(side,value):
    assert value in [0,1]
    result=http(side,'PUT','/'+INDEX+'/_settings',{'index':{'number_of_replicas':value}},role='operator');assert result['acknowledged']
def native(side,kind=None):
    health=http(side,'GET','/_cluster/health',role='monitor');stats=http(side,'GET','/'+INDEX+'/_stats/docs,indexing,search,store',role='monitor');nodes=http(side,'GET','/_nodes/stats/jvm,fs',role='monitor');settings=http(side,'GET','/'+INDEX+'/_settings',role='operator');count=http(side,'GET','/'+INDEX+'/_count')
    return {'observed_at':now(),'side':side,'health':{k:health[k] for k in ['cluster_name','status','number_of_nodes','active_primary_shards','active_shards','unassigned_shards']},'index':{'name':INDEX,'searchable_documents':count['count'],'replicas':settings[INDEX]['settings']['index']['number_of_replicas'],'primary_stats':stats['indices'][INDEX]['primaries']},'nodes':[{ 'name':n['name'],'heap_used_bytes':n['jvm']['mem']['heap_used_in_bytes'],'disk_available_bytes':n['fs']['total']['available_in_bytes']} for n in nodes['nodes'].values()]}
def verify(side,kind=None):
    expected=200 if side=='a' else 80
    found=http(side,'POST','/'+INDEX+'/_search',{'query':{'match':{'text':'actual'}},'size':5,'track_total_hits':True});assert found['hits']['total']['value']==expected and all(h['_source']['group']==side for h in found['hits']['hits'])
    denied_search=http(side,'POST','/'+INDEX+'/_search',{'query':{'match_all':{}}},role='monitor',allowed=(403,))
    denied_write=http(side,'PUT','/p9-forbidden-create',{'settings':{'number_of_shards':1,'number_of_replicas':0}},role='monitor',allowed=(403,))
    assert http(side,'HEAD','/p9-forbidden-create',role='elastic',allowed=(404,))['status']==404
    denied_admin=http(side,'GET','/_security/user',role='app',allowed=(403,))
    return {'observed_at':now(),'side':side,'documents':expected,'real_search_hits':len(found['hits']['hits']),'monitor_document_search_denied':denied_search['status']==403,'monitor_index_creation_denied':denied_write['status']==403,'app_admin_access_denied':denied_admin['status']==403,'native':native(side)}
def initialize(side,kind=None):
    deadline=time.monotonic()+90
    while True:
        try:version=http(side,'GET','/',role='elastic')['version']['number'];break
        except (AssertionError,urllib.error.URLError):
            if time.monotonic()>deadline:raise
            time.sleep(2)
    roles={'monitor':{'cluster':['monitor'],'indices':[{'names':['p9-*'],'privileges':['monitor']}]},'app':{'cluster':[],'indices':[{'names':['p9-*'],'privileges':['read','write']}]},'operator':{'cluster':[],'indices':[{'names':['p9-*'],'privileges':['manage']}]}}
    for role,spec in roles.items():
        http(side,'PUT','/_security/role/p9-'+role,spec,role='elastic');http(side,'PUT','/_security/user/'+role,{'password':CREDS[side][role],'roles':['p9-'+role]},role='elastic')
    if http(side,'HEAD','/'+INDEX,role='elastic',allowed=(200,404))['status']==404:
        http(side,'PUT','/'+INDEX,{'settings':{'number_of_shards':1,'number_of_replicas':0},'mappings':{'dynamic':'strict','properties':{'group':{'type':'keyword'},'sequence':{'type':'long'},'counter':{'type':'long'},'text':{'type':'text'}}}},role='elastic')
    total=200 if side=='a' else 80;assert concurrent_bulk(side,'base-',total)==total
    key='protocol-'+uuid.uuid4().hex
    try:
        http(side,'PUT','/'+INDEX+'/_doc/'+key+'?refresh=true',document(side,999));assert http(side,'GET','/'+INDEX+'/_doc/'+key)['_source']['sequence']==999
        http(side,'POST','/'+INDEX+'/_update/'+key+'?refresh=true',{'doc':{'counter':2}});assert http(side,'GET','/'+INDEX+'/_doc/'+key)['_source']['counter']==2
    finally:http(side,'DELETE','/'+INDEX+'/_doc/'+key+'?refresh=true',allowed=(200,404))
    health=http(side,'GET','/_cluster/health?wait_for_status=green&timeout=30s',role='monitor');assert health['status']=='green'
    return {'version':version,'actual_index_get_update_delete_search':True,**verify(side)}
def background(side,kind=None):
    start=time.monotonic();operations=0
    while not STOP and time.monotonic()-start<28800:
        key='base-'+str(operations%40);result=http(side,'POST','/'+INDEX+'/_update/'+key,{'doc':{'counter':operations+1}});assert result['result'] in ['updated','noop']
        found=http(side,'POST','/'+INDEX+'/_search',{'query':{'term':{'group':side}},'size':5,'track_total_hits':True});assert found['hits']['total']['value']>=80
        operations+=1;record('background-'+side,acknowledged_update_search_cycles=operations)
        for _ in range(20 if side=='a' else 70):
            if STOP:break
            time.sleep(.1)
    return {'acknowledged_update_search_cycles':operations}
def pressure(side,kind):
    assert side=='a' and kind in ['health','documents'];start=time.monotonic();marker=Path('/var/lib/plan9/stop-elastic-'+kind)
    try:
        if kind=='health':replicas(side,1)
        else:assert concurrent_bulk(side,'pressure-',200)==200
        record('pressure-'+kind,ready=True,extra_documents=200 if kind=='documents' else 0,requested_replicas=1 if kind=='health' else 0,maximum_seconds=180)
        while not STOP and time.monotonic()-start<180 and not marker.exists():time.sleep(.25)
    finally:
        if kind=='health':replicas(side,0)
        else:concurrent_bulk(side,'pressure-',200,True)
    record('pressure-'+kind,ready=False,recovered=True,elapsed_seconds=round(time.monotonic()-start,3));return {'recovered':True}
def recover(side,kind=None):
    replicas(side,0);concurrent_bulk(side,'pressure-',200,True);return native(side)
if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('action',choices=['initialize','native','verify','background','pressure','recover']);p.add_argument('side',choices=['a','b']);p.add_argument('--kind',choices=['health','documents']);a=p.parse_args();print(json.dumps(globals()[a.action](a.side,a.kind)),flush=True)
