import argparse
import json
from pathlib import Path
import sys
from manage import Run,execute,write_json,now
sys.path.insert(0,str(Path(__file__).resolve().parents[1]/'common'))
from api import Api
from retirement import retire

if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('--session',required=True);p.add_argument('--ca',required=True);a=p.parse_args()
    run=Run();api=Api(run,a.session,a.ca);previous=run.out/'cleanup.json'
    if not previous.exists() or json.loads(previous.read_text(encoding='utf-8'))['status']!='destroyed':
        run.shell('collector','systemctl stop plan9-mq-recovery.timer 2>/dev/null || true\nsystemctl stop plan9-mq-producer@work plan9-mq-producer@control plan9-mq-consumer@work plan9-mq-consumer@control\nrm -f /var/lib/plan9/hold-work\n')
        for side in ['work','control']:run.client('drain',side)
    retire(run,api,execute,write_json,now)
