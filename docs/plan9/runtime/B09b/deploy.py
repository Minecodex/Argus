"""Create a fresh B09b cluster only after B09a retirement completes."""
import argparse
from pathlib import Path
import shutil

from run import ROOT
from infrastructure import Infrastructure,RUN


if __name__=='__main__':
    parser=argparse.ArgumentParser()
    parser.add_argument('action',choices=['prepare','cluster','bootstrap','foundation'])
    parser.add_argument('--run',default=RUN)
    parser.add_argument('--session',default='artifacts/plan9/p9-b09b-20261109-01/private/enterprise-session.json')
    args=parser.parse_args()
    run=Infrastructure(args.run)
    if args.action=='foundation':
        run.foundation(args.session)
        target=run.private/'enterprise-session.json'
        if Path(args.session).resolve()!=target.resolve():
            shutil.copy2(args.session,target)
    else:
        getattr(run,args.action)()
