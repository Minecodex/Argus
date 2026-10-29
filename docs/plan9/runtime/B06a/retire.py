import argparse
import json
from pathlib import Path
import sys
from manage import Run,execute,write_json,now
sys.path.insert(0,str(Path(__file__).resolve().parents[1]/'common'))
from api import Api
from retirement import retire
if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('--session',required=True);p.add_argument('--ca',required=True);a=p.parse_args();run=Run();api=Api(run,a.session,a.ca);old=run.out/'cleanup.json'
    if not old.exists() or json.loads(old.read_text(encoding='utf-8'))['status']!='destroyed':
        for kind in ['connections','operations']:run.fault(kind,True)
        run.shell('collector','systemctl stop plan9-mongo-pressure-connections plan9-mongo-pressure-operations 2>/dev/null || true\nsystemctl stop plan9-mongo-workload@a plan9-mongo-workload@b\n')
    retire(run,api,execute,write_json,now)
