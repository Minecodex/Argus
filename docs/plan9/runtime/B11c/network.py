"""Real target-netns egress rejection; never modifies the platform or host firewall."""
import argparse
import json
import time
from manage import Run,execute

def service(namespace,name):
    return json.loads(execute(['kubectl','--context','docker-desktop','-n',namespace,'get','service',name,'-o','json']))

def configure(run):
    mapping=json.loads(run.resources.read_text(encoding='utf-8'));name='plan9-b11c-telemetry'
    original=service('argus-system','argus-telemetry-ingest')
    existing=execute(['kubectl','--context','docker-desktop','-n','argus-system','get','service',name,'--ignore-not-found','-o','json'])
    if existing:assert json.loads(existing)['metadata']['labels']['argus.plan9/run']==run.id
    desired={'apiVersion':'v1','kind':'Service','metadata':{'name':name,'namespace':'argus-system','labels':{'argus.plan9/run':run.id}},
        'spec':{'type':'LoadBalancer','selector':original['spec']['selector'],
                'ports':[{'name':'grpc','port':4317,'targetPort':4317},{'name':'https','port':4318,'targetPort':4318}]}}
    execute(['kubectl','--context','docker-desktop','apply','-f','-'],data=json.dumps(desired))
    for _ in range(40):
        value=service('argus-system',name);addresses=value.get('status',{}).get('loadBalancer',{}).get('ingress',[])
        if addresses and addresses[0].get('ip'):break
        time.sleep(1)
    else:raise RuntimeError('Owned native ingest endpoint unavailable')
    https=service('argus-ingress','argus-ingress')['status']['loadBalancer']['ingress'][0]['ip']
    gateway=service('argus-system','argus-connector-gateway-public')['status']['loadBalancer']['ingress'][0]['ip']
    ingest=addresses[0]['ip']
    hosts={https:['argus.dev','artifacts.argus.dev'],gateway:['connector.argus.dev'],ingest:['argus-telemetry-ingest.argus-system.svc','argus-telemetry-ingest.argus-system.svc.cluster.local']}
    run.shell('bastion',"python3 - <<'PY'\nfrom pathlib import Path\nentries="+repr(hosts)+"\np=Path('/etc/hosts');names={name for values in entries.values() for name in values}\nlines=[line for line in p.read_text().splitlines() if not names.intersection(line.split()[1:])]\nlines.extend(address+' '+' '.join(values) for address,values in entries.items())\np.write_text('\\n'.join(lines)+'\\n')\nPY\n")
    mapping['telemetry_network_endpoint']={'name':name,'namespace':'argus-system','uid':value['metadata']['uid'],'ip':ingest}
    run.save('resource-map',mapping)
    endpoints=[{'name':'https','address':https,'port':443},{'name':'control','address':gateway,'port':9443},
               {'name':'otlp-grpc','address':ingest,'port':4317},{'name':'otlp-http','address':ingest,'port':4318}]
    run.save('network-endpoints',endpoints)
    print('Actual platform endpoints mapped only inside the owned Bastion; native ingest probe service ready',flush=True)

def probes(run,key):
    endpoints=json.loads((run.out/'network-endpoints.json').read_text())
    script="python3 - <<'PY'\nimport json,socket\nendpoints="+repr(endpoints)+"\nfor endpoint in endpoints:\n try:\n  sock=socket.create_connection((endpoint['address'],endpoint['port']),timeout=2);sock.close();endpoint['connected']=True\n except OSError as error:endpoint.update(connected=False,error=type(error).__name__)\nprint(json.dumps(endpoints))\nPY\n"
    return json.loads(run.shell(key,script))

def restrict(run):
    if (run.out/'actual-network-isolation.json').exists():
        assert not any(row['connected'] for row in probes(run,'bastion')+probes(run,'member'))
        return
    before=probes(run,'bastion');assert all(row['connected'] for row in before),before
    plan=json.loads((run.out/'network-plan.json').read_text())
    root=run.container('bastion');member=run.container('member')
    assert root['HostConfig']['NetworkMode']!='host' and member['HostConfig']['NetworkMode']!='host'
    assert set(member['NetworkSettings']['Networks'])=={run.id+'_inside'}
    network=json.loads(execute(['docker','network','inspect',run.id+'_inside']))[0]
    assert network['Internal'] and network['Labels']['io.argus.plan9.run']==run.id
    run.shell('bastion',"iptables -N P9_B11C_EGRESS\niptables -A P9_B11C_EGRESS -m conntrack --ctstate ESTABLISHED,RELATED -j RETURN\niptables -A P9_B11C_EGRESS -o lo -j RETURN\niptables -A P9_B11C_EGRESS -d "+plan['PLAN9_MEMBER_PRIVATE']+" -p tcp --dport 22 -j RETURN\niptables -A P9_B11C_EGRESS -j REJECT\niptables -I OUTPUT 1 -j P9_B11C_EGRESS\n")
    run.shell('member',"iptables -N P9_B11C_MEMBER\niptables -A P9_B11C_MEMBER -d "+plan['PLAN9_BASTION_PRIVATE']+" -p tcp -m multiport --dports 4317,4318 -j REJECT\niptables -I OUTPUT 1 -j P9_B11C_MEMBER\n")
    blocked=probes(run,'bastion');isolated=probes(run,'member')
    assert not any(row['connected'] for row in blocked+isolated)
    rules={key:run.shell(key,'iptables-save\n') for key in ['bastion','member']}
    run.save('actual-network-isolation',{'root_positive_baseline':before,'root_blocked':blocked,'member_blocked':isolated,
        'member_internal_network':True,'member_direct_gateway_otlp_rejected':True,'rules':rules,'only_owned_network_namespaces_modified':True})
    print('Actual Bastion outbound platform sockets rejected; member internal-only and direct gateway OTLP denied',flush=True)

if __name__=='__main__':
    parser=argparse.ArgumentParser();parser.add_argument('action',choices=['configure','restrict']);args=parser.parse_args();globals()[args.action](Run())
