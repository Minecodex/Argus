"""Staged B10c entrypoint; each action records its actual result."""
import argparse
from infrastructure import Run

if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('action', choices=['prepare','preflight','transition','sources','cluster','bootstrap','foundation',
                                         'helm','extensions','nodes','monitoring','verify','final-checks','handoff','publish-web','publish-backend','retire'])
    action = parser.parse_args().action
    run = Run()
    if action in ('prepare','preflight','cluster','bootstrap','foundation'):
        getattr(run, action)()
    else:
        if action == 'transition':
            from transition import execute_stage
        elif action == 'sources':
            from sources import execute_stage
        elif action == 'helm':
            from helm import execute_stage
        elif action in ('extensions','nodes'):
            from exercises import execute_stage
        elif action in ('monitoring','verify','handoff'):
            from monitoring import execute_stage
        elif action == 'final-checks':
            from verification import execute_stage
        elif action == 'publish-web':
            from publish_web import execute_stage
        elif action == 'publish-backend':
            from publish_backend import execute_stage
        else:
            from retirement import execute_stage
        execute_stage(run, action)
