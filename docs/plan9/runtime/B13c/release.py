"""Publish the target lifecycle entry with the existing retained-data release."""
import os
from manage import Run

os.environ.setdefault('ARGUS_PLAN9_RELEASE_TAG', 'local-20261118-b13c-target-lifecycle-v1')
from B13a.release import deploy
from publish_portal import publish

if __name__ == '__main__':
    deploy(web_publisher=publish)
