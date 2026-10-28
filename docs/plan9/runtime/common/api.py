"""Normal Argus API/session/confirmation client for owned Plan9 runs."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import time

import requests


class Api:
    def __init__(self, run, session_path, ca_path, credentials=None):
        self.run = run
        self.session_path = Path(session_path)
        stored = json.loads(self.session_path.read_text(encoding='utf-8'))
        self.origin = stored['origin']
        assert self.origin == 'https://argus.dev'
        self.http = requests.Session()
        self.http.trust_env = False
        self.http.verify = str(ca_path)
        self.http.headers['Origin'] = self.origin
        for c in stored['cookies']:
            self.http.cookies.set(c['name'], c['value'], domain=c['domain'], path=c['path'])
        self.csrf = stored['csrf']
        self.user = stored['user']
        self.credentials = credentials

    def save(self):
        self.session_path.write_text(json.dumps({
            'origin': self.origin, 'csrf': self.csrf, 'user': self.user,
            'cookies': [{'name': c.name, 'value': c.value, 'domain': c.domain,
                         'path': c.path, 'secure': c.secure} for c in self.http.cookies]},
            ensure_ascii=False, indent=2) + '\n', encoding='utf-8')

    def login(self):
        if not self.credentials:
            raise RuntimeError('Session expired; normal enterprise login required')
        self.http.cookies.clear()
        r = self.http.post(self.origin + '/api/v1/enterprise/auth/login', json={
            'username': self.credentials['enterprise_username'],
            'password': self.credentials['password']}, timeout=30)
        if r.status_code != 200 or r.json().get('status') != 'authenticated':
            raise RuntimeError('Normal enterprise reauthentication did not complete')
        session = r.json()['authenticated_session']
        self.csrf, self.user = session['csrf_token'], session['user']
        self.save()

    def call(self, method, path, body=None, *, key=None, expected=200, retry=True):
        headers = {'X-CSRF-Token': self.csrf}
        if method != 'GET':
            frozen = self.run.id + '\0' + (key or path) + '\0' + json.dumps(body, sort_keys=True)
            headers['Idempotency-Key'] = 'p9-' + hashlib.sha256(frozen.encode()).hexdigest()
        response = self.http.request(method, self.origin + '/api/v1' + path,
                                     json=body, headers=headers, timeout=45)
        try:
            value = response.json() if response.content else None
        except ValueError:
            value = None
        code = value.get('code') if isinstance(value, dict) else None
        with (self.run.out / 'api-events.jsonl').open('a', encoding='utf-8') as stream:
            stream.write(json.dumps({'time': datetime.now(timezone.utc).isoformat(), 'method': method,
                                     'path': path, 'status': response.status_code, 'code': code}) + '\n')
        if response.status_code == 401 and retry and self.credentials:
            self.login()
            return self.call(method, path, body, key=key, expected=expected, retry=False)
        allowed = expected if isinstance(expected, (list, tuple)) else [expected]
        if response.status_code not in allowed:
            params = value.get('params', {}) if isinstance(value, dict) else {}
            detail = {k: params[k] for k in ('field', 'rule', 'min', 'max') if k in params}
            raise RuntimeError(f'{method} {path}: HTTP {response.status_code}, code={code}, validation={detail}')
        return value

    def wait(self, path, predicate, *, timeout=300, label='API state'):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            value = self.call('GET', path)
            if predicate(value):
                return value
            time.sleep(2)
        raise RuntimeError(label + ' did not converge within the bounded wait')

    def confirm(self, preview):
        ref = preview['action_ref']
        result = self.call('POST', '/enterprise/pending-actions/' + ref + '/confirm', key='confirm-' + ref)
        execution = result.get('execution', {})
        if execution.get('status') not in ('succeeded', None):
            eid = execution['execution_id']
            def finished(value):
                if value['status'] in ('failed', 'cancelled'):
                    raise RuntimeError('Execution ' + eid + ': ' + value['status'] + ', ' + str(value.get('error_code')))
                return value['status'] == 'succeeded'
            result['execution'] = self.wait('/enterprise/executions/' + eid, finished, label='Execution')
            if result['execution'].get('resource_ref'):
                result['resource_ref'] = result['execution']['resource_ref']
        self.run.event('action_confirmed', action_ref=ref,
                       execution_id=result.get('execution', {}).get('execution_id'),
                       resource_ref=result.get('resource_ref'))
        return result
