"""Publish scoped notification trust using the verified preserved-data release path."""
import os
from manage import Run

os.environ.setdefault('ARGUS_PLAN9_RELEASE_TAG', 'local-20261118-b13b-notification-ca-v1')
from B13a.release import deploy

if __name__ == '__main__':
    deploy()
