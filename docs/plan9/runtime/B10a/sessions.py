"""Real product CLI TCP forwarding; terminal and log UI use the same live cluster."""
import json,os,socket,subprocess,time,uuid
import requests
from infrastructure import NAMESPACE
def verify(r,a):
 svc=r.get('service','b10a-api','-n',NAMESPACE)
 path='/enterprise/kubernetes-clusters/'+r.cluster_id
 ticket=a.call('POST',path+'/stream-sessions',{'namespace_id':r.namespace(a),'type':'service','name':'b10a-api','uid':svc['metadata']['uid'],'mode':'port_forward','port':8080,'reason':'B10a real loopback HTTP forwarding validation'},key='forward-session-'+uuid.uuid4().hex,expected=201)
 session=ticket['session'];r.save('forward-session-created',session)
 sock=socket.socket();sock.bind(('127.0.0.1',0));port=sock.getsockname()[1];sock.close()
 env=os.environ.copy()
 for key in list(env):
  if key.lower().endswith('_proxy'):env.pop(key)
 log=r.out/'forward-cli.log'
 with log.open('w') as stream:
  process=subprocess.Popen([str(r.private/'argusctl.exe'),'kubernetes','port-forward','--url',ticket['websocket_url'],'--ticket-stdin','--listen','127.0.0.1:'+str(port)],stdin=subprocess.PIPE,stdout=stream,stderr=subprocess.STDOUT,text=True,env=env)
  process.stdin.write(ticket['ticket']+'\n');process.stdin.close();ticket.pop('ticket')
  receipt=None
  try:
   http=requests.Session();http.trust_env=False;deadline=time.monotonic()+35
   while time.monotonic()<deadline:
    if process.poll() is not None:raise RuntimeError('Product port-forward CLI ended: '+log.read_text(encoding="utf-8")[-500:])
    try:
     response=http.get('http://127.0.0.1:'+str(port)+'/?label=b10a-forwarded-http',timeout=3)
     if response.status_code==200:receipt=response.json();break
    except requests.RequestException:pass
    time.sleep(.5)
   assert receipt and receipt['run']==r.id and receipt['pod_uid']==session['pod_uid']
   r.save('forward-http-receipt',receipt)
  finally:
   a.call('POST',path+'/stream-sessions/'+session['id']+'/close',key='close-'+session['id'])
   try:process.wait(12)
   except subprocess.TimeoutExpired:process.terminate();process.wait(5)
 current=a.wait(path+'/stream-sessions/'+session['id'],lambda x:x['status'] in ('closed','invalidated','connection_lost'),timeout=20,label='Port forwarding closed')
 assert current['bytes_in']>0 and current['bytes_out']>0
 r.save('forward-session-closed',current)
 print('Real product CLI forwarding returned the pinned Pod HTTP response and closed with byte counts',flush=True)
