import argparse
import json
from pathlib import Path
import sys
from manage import Run,ENGINES,execute,write_json,now
sys.path.insert(0,str(Path(__file__).resolve().parents[1]/'common'))
from api import Api
from retirement import retire

if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('--session',required=True);p.add_argument('--ca',required=True);a=p.parse_args()
    run=Run();api=Api(run,a.session,a.ca);previous=run.out/'cleanup.json'
    if not previous.exists() or json.loads(previous.read_text(encoding='utf-8'))['status']!='destroyed':
        for key in ENGINES:
            run.shell('collector',f'systemctl stop plan9-cache-{key}.service || true\n');run.client('recover',key)
    retire(run,api,execute,write_json,now)
