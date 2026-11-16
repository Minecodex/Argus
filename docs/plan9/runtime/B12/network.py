"""Target-only mappings to the actual normal TLS platform ingress endpoints."""
import json
import time
from manage import Run,execute
from B11c.network import service

def configure(run):
    name='plan9-b12-telemetry';mapping=json.loads(run.resources.read_text());original=service('argus-system','argus-telemetry-ingest')
    existing=execute(['kubectl','--context','docker-desktop','-n','argus-system','get','service',name,'--ignore-not-found','-o','json'])
    if existing:assert json.loads(existing)['metadata']['labels']['argus.plan9/run']==run.id
    desired={'apiVersion':'v1','kind':'Service','metadata':{'name':name,'namespace':'argus-system','labels':{'argus.plan9/run':run.id}},
        'spec':{'type':'LoadBalancer','selector':original['spec']['selector'],'ports':[{'name':'grpc','port':4317,'targetPort':4317},{'name':'https','port':4318,'targetPort':4318}]}}
    execute(['kubectl','--context','docker-desktop','apply','-f','-'],data=json.dumps(desired))
    for _ in range(40):
        value=service('argus-system',name);addresses=value.get('status',{}).get('loadBalancer',{}).get('ingress',[])
        if addresses and addresses[0].get('ip'):break
        time.sleep(1)
    else:raise RuntimeError('Owned telemetry ingress was not allocated')
    https=service('argus-ingress','argus-ingress')['status']['loadBalancer']['ingress'][0]['ip']
    gateway=service('argus-system','argus-connector-gateway-public')['status']['loadBalancer']['ingress'][0]['ip'];ingest=addresses[0]['ip']
    entries={https:['argus.dev','artifacts.argus.dev'],gateway:['connector.argus.dev'],ingest:['argus-telemetry-ingest.argus-system.svc','argus-telemetry-ingest.argus-system.svc.cluster.local']}
    run.shell('direct',"python3 - <<'PY'\nfrom pathlib import Path\nentries="+repr(entries)+"\np=Path('/etc/hosts');names={name for values in entries.values() for name in values}\nlines=[line for line in p.read_text().splitlines() if not names.intersection(line.split()[1:])]\nlines.extend(address+' '+' '.join(values) for address,values in entries.items())\np.write_text('\\n'.join(lines)+'\\n')\nPY\n")
    mapping['telemetry_network_endpoint']={'name':name,'namespace':'argus-system','uid':value['metadata']['uid'],'ip':ingest};run.save('resource-map',mapping)
    run.save('network-endpoints',entries)
    print('Actual TLS ingress mapped in the new host only; formal networking unchanged',flush=True)

if __name__=='__main__':configure(Run())
