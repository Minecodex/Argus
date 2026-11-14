"""Native end-to-end TLS exposure for the owned direct host (no relay)."""
import json
import time
from manage import Run,execute


def configure(run):
    cli=['kubectl','--context','docker-desktop','-n','argus-system']
    name='plan9-b11a-telemetry'
    original=json.loads(execute([*cli,'get','svc','argus-telemetry-ingest','-o','json']))
    current=execute([*cli,'get','svc',name,'--ignore-not-found','-o','json'])
    if current:assert json.loads(current)['metadata']['labels']['argus.plan9/run']==run.id
    desired={'apiVersion':'v1','kind':'Service','metadata':{'name':name,'namespace':'argus-system',
        'labels':{'argus.plan9/run':run.id}},'spec':{'type':'LoadBalancer','selector':original['spec']['selector'],
        'ports':[{'name':'grpc','port':4317,'targetPort':4317},{'name':'https','port':4318,'targetPort':4318}]}}
    execute([*cli,'apply','-f','-'],data=json.dumps(desired))
    for _ in range(30):
        svc=json.loads(execute([*cli,'get','svc',name,'-o','json']))
        ingress=svc.get('status',{}).get('loadBalancer',{}).get('ingress',[])
        if ingress and ingress[0].get('ip'):break
        time.sleep(1)
    else:raise RuntimeError('Native telemetry exposure unavailable')
    ip=ingress[0]['ip']
    run.shell('direct',"printf '%s\\n' '"+ip+" argus-telemetry-ingest.argus-system.svc argus-telemetry-ingest.argus-system.svc.cluster.local' >> /etc/hosts\n")
    mapping=json.loads(run.resources.read_text(encoding='utf-8'))
    mapping['telemetry_network_endpoint']={'namespace':'argus-system','name':name,'uid':svc['metadata']['uid'],'ip':ip}
    run.save('resource-map',mapping)
    run.save('telemetry-network-endpoint',{**mapping['telemetry_network_endpoint'],'ports':[4317,4318],
        'end_to_end_native_tls':True,'tls_termination_proxy':False,'collector_tunnel':False})
    print('Native TLS telemetry endpoint ready at '+ip,flush=True)


if __name__=='__main__':configure(Run())
