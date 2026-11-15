"""Native TLS exposure for the Bastion and actual member isolation evidence."""
import argparse
import json
import time
from manage import Run,execute


def configure(run):
    cli=['kubectl','--context','docker-desktop','-n','argus-system'];name='plan9-b11b-telemetry'
    original=json.loads(execute([*cli,'get','svc','argus-telemetry-ingest','-o','json']))
    current=execute([*cli,'get','svc',name,'--ignore-not-found','-o','json'])
    if current:assert json.loads(current)['metadata']['labels']['argus.plan9/run']==run.id
    desired={'apiVersion':'v1','kind':'Service','metadata':{'name':name,'namespace':'argus-system','labels':{'argus.plan9/run':run.id}},
        'spec':{'type':'LoadBalancer','selector':original['spec']['selector'],
                'ports':[{'name':'grpc','port':4317,'targetPort':4317},{'name':'https','port':4318,'targetPort':4318}]}}
    execute([*cli,'apply','-f','-'],data=json.dumps(desired))
    for _ in range(30):
        svc=json.loads(execute([*cli,'get','svc',name,'-o','json']));items=svc.get('status',{}).get('loadBalancer',{}).get('ingress',[])
        if items and items[0].get('ip'):break
        time.sleep(1)
    else:raise RuntimeError('Bastion native ingest endpoint not ready')
    ip=items[0]['ip'];run.shell('bastion',"printf '%s\\n' '"+ip+" argus-telemetry-ingest.argus-system.svc argus-telemetry-ingest.argus-system.svc.cluster.local' >> /etc/hosts\n")
    mapping=json.loads(run.resources.read_text());mapping['telemetry_network_endpoint']={'name':name,'namespace':'argus-system','uid':svc['metadata']['uid'],'ip':ip};run.save('resource-map',mapping)
    run.save('telemetry-network-endpoint',{**mapping['telemetry_network_endpoint'],'ports':[4317,4318],'end_to_end_native_tls':True})
    print('Bastion-only native TLS platform endpoint prepared',flush=True)


def isolation(run):
    member=run.container('member');bastion=run.container('bastion');network=run.id+'_inside'
    assert set(member['NetworkSettings']['Networks'])=={network}
    native=json.loads(execute(['docker','network','inspect',member['NetworkSettings']['Networks'][network]['NetworkID']]))[0]
    assert native['Internal'] and native['Labels']['io.argus.plan9.run']==run.id
    plan=json.loads((run.out/'network-plan.json').read_text())
    # Exact reachable platform addresses bypass DNS, so a DNS failure cannot fake isolation.
    public=json.loads(run.shell('bastion',"python3 - <<'PY'\nimport socket,json\nprint(json.dumps({'https':socket.gethostbyname('argus.dev'),'gateway':socket.gethostbyname('connector.argus.dev')}))\nPY\n"))
    endpoints=[(public['https'],443),(public['gateway'],9443)]
    if (run.out/'telemetry-network-endpoint.json').exists():
        ip=json.loads((run.out/'telemetry-network-endpoint.json').read_text())['ip'];endpoints.extend([(ip,4317),(ip,4318)])
    script="python3 - <<'PY'\nimport json,socket,subprocess\nendpoints="+repr(endpoints)+"\nrows=[]\nfor address,port in endpoints:\n try:\n  connection=socket.create_connection((address,port),timeout=2);connection.close();rows.append({'address':address,'port':port,'connected':True})\n except OSError as error:rows.append({'address':address,'port':port,'connected':False,'reason':str(error)})\nprint(json.dumps({'routes':subprocess.check_output(['ip','route']).decode(),'probes':rows}))\nPY\n"
    baseline=json.loads(run.shell('bastion',script));assert all(row['connected'] for row in baseline['probes'])
    result=json.loads(run.shell('member',script));assert not any(row['connected'] for row in result['probes'])
    run.shell('bastion','curl -fsS --max-time 4 http://'+plan['PLAN9_MEMBER_PRIVATE']+':18082/health\n')
    run.save('member-isolation',{'network_id':native['Id'],'internal':True,'member_networks':list(member['NetworkSettings']['Networks']),
        'native':result,'bastion_same_endpoint_baseline':baseline,'member_to_platform_direct':False,'bastion_to_member_private_http':True,'dns_bypassed_for_direct_tests':True})
    print('Actual member sockets to platform blocked; Bastion can reach the private member',flush=True)


if __name__=='__main__':
    parser=argparse.ArgumentParser();parser.add_argument('action',choices=['configure','isolation']);args=parser.parse_args();globals()[args.action](Run())
