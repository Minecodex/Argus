"""Actual bounded offline tools in a disposable, owner-checked Kubernetes Pod."""
import base64
import hashlib
import json
from pathlib import PurePosixPath
import subprocess

from prepare import OUT, save

NS = 'p9-b12-context-20261116'
OWNER = 'argus-b12-context-compare-20261116'
IMAGE = 'host.docker.internal:5001/argus/argus-workspace:local-20261116-b12-file-previews@sha256:b20c65b97143eecd873b7c1ee13d06b4b18750345a791a9ba588fced486c366c'


def kubectl(*args, data=None, timeout=90):
    value = subprocess.run(['kubectl', '--context', 'docker-desktop', *args], input=data,
                           capture_output=True, text=True, encoding='utf-8', timeout=timeout)
    if value.returncode:
        raise RuntimeError('Kubernetes operation failed: '+value.stderr[:1200])
    return value.stdout.strip()


def check_namespace():
    value = json.loads(kubectl('get', 'namespace', NS, '-o', 'json'))
    if value['metadata']['labels'].get('io.argus.plan9.run') != OWNER:
        raise RuntimeError('Namespace ownership mismatch')
    record = json.loads((OUT/'namespace.json').read_text(encoding='utf-8'))
    if value['metadata']['uid'] != record['uid']:
        raise RuntimeError('Namespace UID changed')
    return value


def provision():
    exists = kubectl('get', 'namespace', NS, '--ignore-not-found', '-o', 'json')
    if exists:
        check_namespace()
        return
    kubectl('create', '-f', '-', data=json.dumps({'apiVersion':'v1','kind':'Namespace',
            'metadata':{'name':NS,'labels':{'io.argus.plan9.run':OWNER,
                        'pod-security.kubernetes.io/enforce':'restricted'}}}))
    value = json.loads(kubectl('get','namespace',NS,'-o','json'))
    save(OUT/'namespace.json', {'name':NS,'uid':value['metadata']['uid'],'owner':OWNER})
    policies = {'apiVersion':'v1','kind':'List','items':[
        {'apiVersion':'networking.k8s.io/v1','kind':'NetworkPolicy','metadata':{'name':'offline','namespace':NS},
         'spec':{'podSelector':{},'policyTypes':['Ingress','Egress'],'ingress':[],'egress':[]}},
        {'apiVersion':'v1','kind':'ResourceQuota','metadata':{'name':'bounded','namespace':NS},
         'spec':{'hard':{'pods':'1','requests.cpu':'500m','requests.memory':'512Mi','limits.cpu':'1','limits.memory':'768Mi'}}}
    ]}
    kubectl('apply','-f','-',data=json.dumps(policies))


class Sandbox:
    def __init__(self, name, dataset):
        self.name = name
        self.dataset = dataset
        self.reads = []
        check_namespace()
        pod = {'apiVersion':'v1','kind':'Pod','metadata':{'name':name,'namespace':NS,'labels':{'io.argus.plan9.run':OWNER}},
               'spec':{'restartPolicy':'Never','terminationGracePeriodSeconds':1,'activeDeadlineSeconds':1200,'automountServiceAccountToken':False,
                       'securityContext':{'runAsNonRoot':True,'runAsUser':10001,'runAsGroup':10001,'fsGroup':10001,
                                          'seccompProfile':{'type':'RuntimeDefault'}},
                       'containers':[{'name':'workspace','image':IMAGE,'imagePullPolicy':'Never',
                         'command':['python3','-c','import time;time.sleep(1200)'],
                         'resources':{'requests':{'cpu':'250m','memory':'256Mi'},'limits':{'cpu':'1','memory':'512Mi'}},
                         'securityContext':{'allowPrivilegeEscalation':False,'readOnlyRootFilesystem':True,'capabilities':{'drop':['ALL']}},
                         'workingDir':'/workspace','volumeMounts':[{'name':'data','mountPath':'/workspace'},{'name':'temp','mountPath':'/tmp'}]}],
                       'volumes':[{'name':'data','emptyDir':{'sizeLimit':'16Mi'}},{'name':'temp','emptyDir':{'sizeLimit':'8Mi'}}]}}
        kubectl('create','-f','-',data=json.dumps(pod))
        value = json.loads(kubectl('get','pod',name,'-n',NS,'-o','json'))
        self.uid = value['metadata']['uid']
        kubectl('wait','pod/'+name,'-n',NS,'--for=condition=Ready','--timeout=90s',timeout=100)
        seed = {x['path']:base64.b64encode((OUT/'private'/(x['file_id']+'.json')).read_bytes()).decode()
                for x in dataset['files']}
        script = "import sys,json,base64,pathlib\nfor name,value in json.load(sys.stdin).items():\n p=pathlib.Path(name);assert p.is_relative_to('/workspace');p.parent.mkdir(parents=True,exist_ok=True);p.write_bytes(base64.b64decode(value))"
        self.exec('python3','-c',script,data=json.dumps(seed))

    def exec(self, *args, data=None, timeout=90):
        return kubectl('exec','-i','-n',NS,self.name,'--',*args,data=data,timeout=timeout)

    def read(self, args):
        path = args.get('path', '')
        if not isinstance(path,str) or not path.startswith('/workspace/') or '..' in PurePosixPath(path).parts:
            return {'error_code':'TOOL_INPUT_INVALID','message':'path must stay within /workspace'}
        code = '''import sys,json,pathlib,hashlib
p=pathlib.Path(sys.argv[1]);root=pathlib.Path('/workspace')
if not p.resolve().is_relative_to(root) or not p.is_file():
 print(json.dumps({'error_code':'WORKSPACE_FILE_NOT_FOUND'}));sys.exit(0)
b=p.read_bytes();lines=b.decode('utf-8').splitlines(keepends=True)
start=int(sys.argv[2]);end=int(sys.argv[3]);content=''.join(lines[start-1:end])
print(json.dumps({'path':str(p),'content':content[:32768],'byte_size':len(b),'sha256':hashlib.sha256(b).hexdigest(),
 'start_line':start,'end_line':min(end,len(lines)),'partial':len(content)>32768 or end<len(lines)}))'''
        value = json.loads(self.exec('python3','-c',code,path,str(args.get('start_line',1)),str(args.get('end_line',2147483647))))
        self.reads.append({'path':path,'success':'error_code' not in value,'sha256':value.get('sha256')})
        return value

    def bash(self, args):
        # The model command is an argv value inside the offline Pod, never a host shell command.
        code = '''import subprocess,json,sys
try:
 p=subprocess.run(['/bin/bash','-lc',sys.argv[1]],cwd='/workspace',capture_output=True,text=True,timeout=int(sys.argv[2]))
 print(json.dumps({'stdout':p.stdout[:32768],'stderr':p.stderr[:32768],'exit_code':p.returncode,
                  'partial':len(p.stdout)>32768 or len(p.stderr)>32768}))
except subprocess.TimeoutExpired:
 print(json.dumps({'error_code':'WORKSPACE_COMMAND_TIMEOUT'}))'''
        return json.loads(self.exec('python3','-c',code,args['command'],str(min(args.get('timeout_seconds',60),120)),timeout=140))

    def verify_originals(self):
        code = "import json,pathlib,hashlib;print(json.dumps({str(p):hashlib.sha256(p.read_bytes()).hexdigest() for p in pathlib.Path('/workspace/queries').rglob('*.json')}))"
        actual = json.loads(self.exec('python3','-c',code))
        return all(actual.get(x['path']) == x['sha256'] for x in self.dataset['files'])

    def close(self):
        check_namespace()
        value = json.loads(kubectl('get','pod',self.name,'-n',NS,'-o','json'))
        assert value['metadata']['uid'] == self.uid and value['metadata']['labels']['io.argus.plan9.run'] == OWNER
        kubectl('delete','pod',self.name,'-n',NS,'--wait=true','--timeout=60s')


def cleanup():
    value = check_namespace()
    assert not json.loads(kubectl('get','pods','-n',NS,'-o','json'))['items']
    kubectl('delete','namespace',NS,'--wait=true','--timeout=90s',timeout=100)
    save(OUT/'cleanup.json',{'namespace':NS,'uid':value['metadata']['uid'],'deleted':True,'persistent_volumes_created':False})
