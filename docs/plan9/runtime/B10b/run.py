"""B10b normal API client and staged executable entrypoint."""
import argparse
from infrastructure import Infrastructure,ROOT,NAMESPACE,IMAGE,execute,write_json
from common.kubernetes_workbench import Workbench
class Run(Infrastructure,Workbench):pass
if __name__=='__main__':
 p=argparse.ArgumentParser();p.add_argument('action');args=p.parse_args();r=Run()
 if args.action in ('prepare','cluster','bootstrap','foundation'):getattr(r,args.action)()
 elif args.action=='network':
  from network import install
  install(r)
 else:
  from workflow import actions
  actions[args.action](r,r.api())
