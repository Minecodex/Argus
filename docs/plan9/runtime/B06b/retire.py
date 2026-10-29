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
        for kind in ['health','documents']:run.fault(kind,True)
        run.shell('collector','systemctl stop plan9-elastic-pressure-health plan9-elastic-pressure-documents 2>/dev/null || true\nsystemctl stop plan9-elastic-workload@a plan9-elastic-workload@b\n')
        for side in ['a','b']:run.client('recover',side)
    retire(run,api,execute,write_json,now)
