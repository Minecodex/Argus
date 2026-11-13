"""Build only the bounded business workload and load it into the owned node."""
import subprocess
from run import Run,ROOT,IMAGE,execute
r=Run();source=ROOT/'docs/plan9/runtime/B10b'
for tag,directory in [('argus-plan9-host:b01-20261026','B01'),('argus-plan9-k8s:b09a-20261108','B09a'),('argus-plan9-workbench:b10a-20261111','B10a')]:
 if subprocess.run(['docker','image','inspect',tag],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL).returncode:
  subprocess.run(['docker','build','-t',tag,str(ROOT/'docs/plan9/runtime'/directory)],check=True)
subprocess.run(['docker','build','-t',IMAGE,str(source)],check=True)
archive=r.out/'business-image.tar';execute(['docker','image','save','--platform','linux/amd64','-o',str(archive),IMAGE],timeout=120)
target='/var/tmp/'+r.id+'-business.tar';execute(['docker','cp',str(archive),r.id+'-control-plane:'+target])
execute(['docker','exec',r.id+'-control-plane','ctr','-n','k8s.io','images','import','--platform','linux/amd64',target],timeout=120)
r.save('business-image',{'tag':IMAGE,'id':execute(['docker','image','inspect',IMAGE,'--format','{{.Id}}'])})
print('Owned business image loaded',flush=True)
