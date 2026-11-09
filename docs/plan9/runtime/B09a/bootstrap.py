"""Dedicated-cluster API credentials, native observer RBAC and endpoint DNS."""
import base64
import copy
import json
import yaml

from manage import API_SERVER,APP_NS,ROOT,RUN,execute,write_json


def bootstrap(run):
    namespace='plan9-access'
    labels={'argus.plan9/run':RUN}
    objects=[
        {'apiVersion':'v1','kind':'Namespace','metadata':{'name':namespace,'labels':labels}},
        {'apiVersion':'v1','kind':'Namespace','metadata':{'name':'argus-telemetry',
            'labels':{**labels,'app.kubernetes.io/part-of':'argus'}}},
    ]
    script=(ROOT/'deploy/scripts/connector-install.sh').read_text(encoding='utf-8')
    raw=script.split("<<'EOF'",1)[1].split('\n',1)[1].split('\nEOF',1)[0]
    rows=[row for row in yaml.safe_load_all(raw.replace('ARGUS_NAMESPACE',namespace)) if row]
    selected={
        'argus-kubernetes-connector','argus-kubernetes-connector-reader',
        'argus-kubernetes-connector-collector-manager','argus-otelcol-reader',
    }
    objects.extend(row for row in rows if row['kind'] in ('ServiceAccount','Role','RoleBinding','ClusterRole','ClusterRoleBinding')
                   and row['metadata']['name'] in selected)
    for row in objects:
        row['metadata'].setdefault('labels',{}).update(labels)
    # Direct management uses the same explicit namespace-scoped manager role
    # and read-only observation roles as the normal connector installer.
    objects.append({'apiVersion':'rbac.authorization.k8s.io/v1','kind':'ClusterRoleBinding',
        'metadata':{'name':'plan9-access-native-reader','labels':labels},
        'roleRef':{'apiGroup':'rbac.authorization.k8s.io','kind':'ClusterRole','name':'argus-otelcol-reader'},
        'subjects':[{'kind':'ServiceAccount','name':'argus-kubernetes-connector','namespace':namespace}]})
    objects.extend([
        {'apiVersion':'rbac.authorization.k8s.io/v1','kind':'ClusterRole',
         'metadata':{'name':'plan9-telemetry-namespace','labels':labels},
         'rules':[{'apiGroups':[''],'resources':['namespaces'],'resourceNames':['argus-telemetry'],
                   'verbs':['get','update','patch']}]},
        {'apiVersion':'rbac.authorization.k8s.io/v1','kind':'ClusterRoleBinding',
         'metadata':{'name':'plan9-telemetry-namespace','labels':labels},
         'roleRef':{'apiGroup':'rbac.authorization.k8s.io','kind':'ClusterRole','name':'plan9-telemetry-namespace'},
         'subjects':[{'kind':'ServiceAccount','name':'argus-kubernetes-connector','namespace':namespace}]},
    ])
    run.apply(objects)
    write_json(run.out/'bootstrap-rbac.json',objects)
    private=run.private/'platform-kubeconfig.yaml'
    node=run.id+'-control-plane'
    metadata=json.loads(execute(['docker','inspect',node]))[0]
    assert metadata['Config']['Labels']['io.x-k8s.kind.cluster']==run.id
    address='https://'+metadata['NetworkSettings']['Networks']['kind']['IPAddress']+':6443'
    if not private.exists():
        token=run.kubectl('create','token','argus-kubernetes-connector','-n',namespace,'--duration=24h')
        admin=yaml.safe_load(run.kubeconfig.read_text(encoding='utf-8'))
        cluster=admin['clusters'][0]['cluster']
        config={'apiVersion':'v1','kind':'Config','current-context':'plan9','clusters':[{
            'name':'plan9','cluster':{'server':address,'certificate-authority-data':cluster['certificate-authority-data']}}],
            'users':[{'name':'plan9','user':{'token':token}}],
            'contexts':[{'name':'plan9','context':{'cluster':'plan9','user':'plan9','namespace':APP_NS}}]}
        private.write_text(yaml.safe_dump(config),encoding='utf-8')
    node=RUN+'-control-plane'
    gateway='192.168.65.254'
    dns=run.get('configmap','coredns','-n','kube-system')
    original=dns['data']['Corefile']
    marker='# Plan9 Argus endpoints'
    if marker not in original:
        hosts=f'''        {marker}
        hosts {{
            {gateway} argus.dev telemetry.argus.dev artifacts.argus.dev connector.argus.dev
            fallthrough
        }}
'''
        assert '.:53 {' in original
        dns['data']['Corefile']=original.replace('.:53 {\n','.:53 {\n'+hosts,1)
        run.apply([dns])
        run.kubectl('rollout','restart','deployment/coredns','-n','kube-system')
        run.kubectl('rollout','status','deployment/coredns','-n','kube-system','--timeout=90s')
    hosts='server = "http://host.docker.internal:5001"\n[host."http://host.docker.internal:5001"]\n  capabilities = ["pull", "resolve"]\n'
    execute(['docker','exec','-i',node,'sh','-ec',
        'mkdir -p /etc/containerd/certs.d/host.docker.internal:5001; cat > /etc/containerd/certs.d/host.docker.internal:5001/hosts.toml'],
        data=hosts)
    write_json(run.out/'network-setup.json',{'api_server':address,'dns_endpoint':gateway,
        'registry':'http://host.docker.internal:5001','scope':'dedicated_kind_cluster'})
    telemetry_endpoint(run)
    print('Dedicated cluster credentials, RBAC, DNS and registry ready',flush=True)


def telemetry_endpoint(run):
    """Expose only the native TLS ingest ports to this independent lab cluster."""
    original=json.loads(execute(['kubectl','--context','docker-desktop','get','svc',
        'argus-telemetry-ingest','-n','argus-system','-o','json']))
    name='plan9-b09a-telemetry'
    desired={'apiVersion':'v1','kind':'Service','metadata':{'name':name,'namespace':'argus-system',
        'labels':{'argus.plan9/run':RUN}},'spec':{'type':'LoadBalancer','selector':original['spec']['selector'],
        'ports':[{'name':'grpc','port':4317,'targetPort':4317},{'name':'https','port':4318,'targetPort':4318}]}}
    current=execute(['kubectl','--context','docker-desktop','get','svc',name,'-n','argus-system',
        '--ignore-not-found','-o','json'])
    if current:
        assert json.loads(current)['metadata']['labels']['argus.plan9/run']==RUN
    execute(['kubectl','--context','docker-desktop','apply','-f','-'],data=json.dumps(desired))
    import time
    for _ in range(30):
        svc=json.loads(execute(['kubectl','--context','docker-desktop','get','svc',name,'-n','argus-system','-o','json']))
        hosts=svc.get('status',{}).get('loadBalancer',{}).get('ingress',[])
        if hosts and hosts[0].get('ip'):
            break
        time.sleep(1)
    else:
        raise RuntimeError('Cross-cluster telemetry IP unavailable')
    ip=hosts[0]['ip']
    dns=run.get('configmap','coredns','-n','kube-system')
    marker='# Plan9 cross-cluster telemetry'
    if marker not in dns['data']['Corefile']:
        dns['data']['Corefile']=dns['data']['Corefile'].replace('hosts {',
            'hosts {\n            '+marker+'\n            '+ip+
            ' argus-telemetry-ingest.argus-system.svc argus-telemetry-ingest.argus-system.svc.cluster.local',1)
        run.apply([dns])
        run.kubectl('rollout','restart','deployment/coredns','-n','kube-system')
        run.kubectl('rollout','status','deployment/coredns','-n','kube-system','--timeout=60s')
    mapping=json.loads(run.resources.read_text(encoding='utf-8'))
    mapping['telemetry_network_endpoint']={'namespace':'argus-system','name':name,'uid':svc['metadata']['uid'],'ip':ip}
    write_json(run.resources,mapping)
    write_json(run.out/'telemetry-network-endpoint.json',{'service':name,'namespace':'argus-system',
        'uid':svc['metadata']['uid'],'ip':ip,'ports':[4317,4318],'end_to_end_native_tls':True})
