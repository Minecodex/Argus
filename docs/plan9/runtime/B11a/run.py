"""Explicit stages for the B11a review package; never advances the next batch."""
import argparse
from manage import Run


if __name__=='__main__':
    parser=argparse.ArgumentParser()
    parser.add_argument('action',choices=['prepare','build','up','network','enroll','workload','remote',
        'dashboards','configure-alert','exercise-alert','verify','retire'])
    action=parser.parse_args().action;run=Run()
    if action in ['prepare','build','up','workload']:getattr(run,action)()
    elif action=='network':
        from network import configure
        configure(run)
    else:
        from integration import Batch
        api=run.api();api.login();batch=Batch(run,api)
        if action=='enroll':batch.enroll()
        elif action=='remote':
            from remote import configure
            configure(batch)
        elif action in ['dashboards','configure-alert','exercise-alert']:
            import monitoring
            getattr(monitoring,action.replace('-','_'))(batch)
        elif action=='verify':
            from verification import verify
            verify(batch)
        else:
            from retirement import retire
            retire(batch)
