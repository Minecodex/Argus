"""Explicit B11b stages; retirement only after a later user instruction."""
import argparse
import runpy
from manage import Run,HERE

if __name__=='__main__':
    parser=argparse.ArgumentParser();parser.add_argument('action',choices=['prepare','build','up','workload','network','isolation',
        'bastion','member','retry_member','collectors','builtins','dashboards','remote','configure_alert','exercise_alert',
        'connection','configure_data_alert','exercise_data_pause','browser','delivery_watch','verify','retire']);action=parser.parse_args().action
    run=Run()
    if action in ['prepare','build','up','workload']:getattr(run,action)()
    elif action in ['browser','delivery_watch']:runpy.run_path(str(HERE/(action+'.py')),run_name='__main__')
    elif action in ['network','isolation']:
        import network
        getattr(network,'configure' if action=='network' else action)(run)
    else:
        from integration import Batch
        api=run.api();api.login();batch=Batch(run,api)
        if action in ['bastion','member','retry_member','collectors','builtins']:getattr(batch,action)()
        elif action in ['dashboards','configure_alert','exercise_alert','configure_data_alert','exercise_data_pause']:
            import monitoring
            getattr(monitoring,action)(batch)
        elif action=='remote':
            from remote import configure
            configure(batch)
        elif action=='connection':
            from connection_exercise import exercise
            exercise(batch)
        elif action=='verify':
            from verification import verify
            verify(batch)
        else:
            from retirement import retire
            retire(batch)
