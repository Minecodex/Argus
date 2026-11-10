"""Owned namespace and bounded activity to exercise actual component histograms."""
import json
import time

from B09a.manage import APP_IMAGE
from environment import assert_owner
from run import HERE

NAMESPACE = 'b09b-components'


def deploy(run):
    assert_owner(run)
    labels = {'argus.plan9/run': run.id}
    deadline_file = run.out / 'activity-deadline.json'
    if deadline_file.exists():
        deadline = json.loads(deadline_file.read_text())['expires_at_epoch']
    else:
        deadline = int(time.time()) + 8 * 3600
        run.save('activity-deadline', {'expires_at_epoch': deadline})
    def metadata(name):
        return {'name': name, 'namespace': NAMESPACE, 'labels': labels}
    objects = [
        {'apiVersion': 'v1', 'kind': 'Namespace', 'metadata': {'name': NAMESPACE, 'labels': labels}},
        {'apiVersion': 'v1', 'kind': 'ServiceAccount', 'metadata': metadata('component-activity')},
        {'apiVersion': 'rbac.authorization.k8s.io/v1', 'kind': 'Role', 'metadata': metadata('component-activity'),
         'rules': [{'apiGroups': ['apps'], 'resources': ['deployments/scale'],
                    'resourceNames': ['metric-workload'], 'verbs': ['get', 'patch']}]},
        {'apiVersion': 'rbac.authorization.k8s.io/v1', 'kind': 'RoleBinding', 'metadata': metadata('component-activity'),
         'roleRef': {'apiGroup': 'rbac.authorization.k8s.io', 'kind': 'Role', 'name': 'component-activity'},
         'subjects': [{'kind': 'ServiceAccount', 'name': 'component-activity', 'namespace': NAMESPACE}]},
        {'apiVersion': 'v1', 'kind': 'ConfigMap', 'metadata': metadata('component-activity'),
         'data': {'activity.py': (HERE / 'activity.py').read_text(encoding='utf-8')}},
    ]
    for name in ('metric-workload', 'component-activity'):
        active = name == 'component-activity'
        container = {'name': 'activity' if active else 'idle', 'image': APP_IMAGE, 'imagePullPolicy': 'Never',
            'command': ['python3', '-u', '/scenario/activity.py'] if active else ['python3', '-c', 'import time; time.sleep(28800)'],
            'resources': {'requests': {'cpu': '5m', 'memory': '16Mi'}, 'limits': {'cpu': '50m', 'memory': '64Mi'}},
            'securityContext': {'allowPrivilegeEscalation': False, 'capabilities': {'drop': ['ALL']}}}
        spec = {'automountServiceAccountToken': active,
                'securityContext': {'runAsNonRoot': True, 'runAsUser': 10001, 'seccompProfile': {'type': 'RuntimeDefault'}},
                'terminationGracePeriodSeconds': 3, 'containers': [container]}
        if active:
            container['env'] = [{'name': 'B09B_ACTIVITY_UNTIL', 'value': str(deadline)}]
            spec['serviceAccountName'] = 'component-activity'
            spec['volumes'] = [{'name': 'scenario', 'configMap': {'name': 'component-activity'}}]
            container['volumeMounts'] = [{'name': 'scenario', 'mountPath': '/scenario', 'readOnly': True}]
        objects.append({'apiVersion': 'apps/v1', 'kind': 'Deployment', 'metadata': metadata(name),
            'spec': {'replicas': 1, 'selector': {'matchLabels': {'app': name}},
                     'template': {'metadata': {'labels': {**labels, 'app': name}}, 'spec': spec}}})
    for value in objects:
        existing = run.infra.kubectl('get', value['kind'], value['metadata']['name'],
            *([] if value['kind'] == 'Namespace' else ['-n', NAMESPACE]), '--ignore-not-found', '-o', 'json')
        if existing:
            assert json.loads(existing)['metadata']['labels']['argus.plan9/run'] == run.id
    run.infra.apply(objects)
    for name in ('metric-workload', 'component-activity'):
        run.infra.kubectl('rollout', 'status', 'deployment/' + name, '-n', NAMESPACE, '--timeout=90s')
    run.save('activity-resources', run.infra.get('namespace', NAMESPACE))
    run.save('activity-workloads', run.infra.get('deploy,pods,sa,role,rolebinding,configmap', '-n', NAMESPACE))
    run.event('real_component_activity_started', namespace=NAMESPACE, interval_seconds=10, max_hours=8)
    print('Bounded real scheduling, DNS and API activity is running', flush=True)
