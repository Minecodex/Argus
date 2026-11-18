"""Rotate owned SSH and Redis monitoring credentials, checking native authentication."""
import argparse
import hashlib
import json
import secrets
from manage import Run, execute, now, write_json
from integration import Batch


def stored_credential(api, ident):
    return next(row for row in api.call('GET', '/enterprise/credentials')['items'] if row['id'] == ident)


def native_ssh(batch, passwords):
    run = batch.run
    host = batch.host_specs['orders']['address']
    key = run.shell('orders', 'cat /etc/ssh/ssh_host_ed25519_key.pub\n').split()[:2]
    settings = {'address': host, 'pubkey': ' '.join(key), 'passwords': passwords, 'hostname': run.id + '-orders'}
    # Read the password from a private temporary file via OpenSSH's prompt
    # mechanism. The pinned target host key comes from the owned container.
    script = f"python3 - <<'PY'\nsettings = {settings!r}\n" + r'''
import json, os, pathlib, subprocess, tempfile
checks = {}
with tempfile.TemporaryDirectory(prefix='b13c-ssh-', dir='/var/lib/plan9') as temporary:
    directory = pathlib.Path(temporary)
    known = directory / 'known_hosts'
    known.write_text(settings['address'] + ' ' + settings['pubkey'] + '\n')
    password = directory / 'password'
    askpass = directory / 'askpass'
    askpass.write_text('#!/bin/sh\ncat "$PLAN9_ASKPASS_FILE"\n')
    askpass.chmod(0o700)
    env = dict(os.environ, SSH_ASKPASS=str(askpass), SSH_ASKPASS_REQUIRE='force', PLAN9_ASKPASS_FILE=str(password))
    for name, value in settings['passwords'].items():
        password.touch(mode=0o600)
        password.write_text(value + '\n')
        result = subprocess.run(['ssh', '-F', '/dev/null', '-T', '-o', 'UserKnownHostsFile=' + str(known), '-o', 'StrictHostKeyChecking=yes',
                                 '-o', 'HostKeyAlgorithms=ssh-ed25519', '-o', 'PreferredAuthentications=password', '-o', 'NumberOfPasswordPrompts=1',
                                 '-o', 'ConnectTimeout=5', 'root@' + settings['address'], 'hostname'], env=env, stdin=subprocess.DEVNULL,
                                capture_output=True, timeout=12, text=True)
        if result.returncode == 0 and result.stdout.strip() == settings['hostname']:
            checks[name] = 'authenticated'
        elif result.returncode == 255 and 'Permission denied' in result.stderr and 'ssh_askpass' not in result.stderr:
            checks[name] = 'password_denied'
        else:
            raise RuntimeError('SSH identity or transport error during owned credential check')
print(json.dumps(checks))
PY
'''
    return json.loads(run.shell('reports', script))


def ssh_rotation(batch):
    run, api = batch.run, batch.api
    if (run.out / 'ssh-rotation.json').exists():
        return
    owner = batch.map['hosts']['orders']
    path = '/enterprise/hosts/' + owner['host_id']
    private = run.private / 'host-credentials.json'
    journal = run.private / 'ssh-rotation.json'
    stored = json.loads(private.read_text(encoding='utf-8'))
    if not journal.exists():
        host = api.call('GET', path)
        assert host['labels']['plan9_run'] == run.id and host['connection_status'] == 'online'
        secret = api.call('GET', '/enterprise/secrets/' + owner['secret_id'])
        credential = stored_credential(api, owner['credential_id'])
        assert native_ssh(batch, {'old_before': stored['orders']['password']}) == {'old_before': 'authenticated'}
        preview = api.call('POST', path + '/actions/preview-reconnect', {'expected_version': host['resource_version']}, key='ssh-old-preview', expected=201)
        write_json(journal, {'at': now(), 'old': stored['orders']['password'], 'new': secrets.token_urlsafe(32), 'old_preview': preview['action_ref'],
                             'secret_version': secret['version'], 'credential_version': credential['version'], 'host': host, 'stage': 'prepared'})
    state = json.loads(journal.read_text(encoding='utf-8'))
    if state['stage'] == 'prepared':
        execute(['docker', 'exec', '-i', run.container('orders')['Id'], 'chpasswd'], data='root:' + state['new'] + '\n')
        api.call('POST', '/enterprise/secrets/' + owner['secret_id'] + '/rotate', {'value': state['new'], 'expected_version': state['secret_version']}, key='ssh-secret-cutover')
        state['stage'] = 'rotated'
        write_json(journal, state)
    stored['orders']['password'] = state['new']
    write_json(private, stored)
    credential = stored_credential(api, owner['credential_id'])
    secret = api.call('GET', '/enterprise/secrets/' + owner['secret_id'])
    assert credential['version'] == state['credential_version'] + 1 and secret['version'] == state['secret_version'] + 1
    denied = api.call('POST', '/enterprise/pending-actions/' + state['old_preview'] + '/confirm', key='ssh-old-preview-refused', expected=[409, 410])
    assert denied['code'] == 'PENDING_ACTION_INVALIDATED'
    probes = native_ssh(batch, {'old': state['old'], 'new': state['new']})
    assert probes == {'old': 'password_denied', 'new': 'authenticated'}, probes
    if 'reconnect' not in state:
        host = api.call('GET', path)
        result = api.confirm(api.call('POST', path + '/actions/preview-reconnect', {'expected_version': host['resource_version']}, key='ssh-new-reconnect', expected=201))
        state['reconnect'] = result
        write_json(journal, state)
    host = api.wait(path, lambda row: row['connection_status'] == 'online', label='Native new-version connection', timeout=100)
    collector = batch.converge('orders')
    assert host['id'] == state['host']['id'] and host['connector_id'] == state['host']['connector_id']
    assert host['pinned_host_key'] == state['host']['pinned_host_key'] and collector['id'] == owner['collector_id']
    run.save('ssh-rotation', {'passed': True, 'at': now(), 'host_id': host['id'], 'secret_versions': [state['secret_version'], secret['version']],
             'credential_versions': [state['credential_version'], credential['version']], 'native_probes': probes, 'old_preview_error': denied['code'],
             'current_version_reconnect': state['reconnect'], 'same_host_connector_collector_and_ssh_key': True})
    write_json(journal, {'stage': 'verified', 'at': now()})
    print('Actual SSH password and platform Secret rotated; old SSH and old preview refused; new control connection verified', flush=True)


def native_redis(batch, passwords):
    script = f"python3 - <<'PY'\npasswords = {passwords!r}\n" + r'''
import json, socket
result = {}
for label, value in passwords.items():
    with socket.create_connection(('redis', 6379), timeout=5) as conn:
        args = [b'AUTH', b'monitor', value.encode()]
        conn.sendall(b'*3\r\n' + b''.join(b'$' + str(len(arg)).encode() + b'\r\n' + arg + b'\r\n' for arg in args))
        stream = conn.makefile('rb')
        response = stream.readline(200)
        if response == b'+OK\r\n':
            conn.sendall(b'*1\r\n$4\r\nPING\r\n')
            assert stream.readline(200) == b'+PONG\r\n'
            result[label] = 'authenticated'
        elif response.startswith(b'-WRONGPASS '):
            result[label] = 'password_denied'
        else:
            raise RuntimeError('Unexpected native auth or Redis transport error')
print(json.dumps(result))
PY
'''
    return json.loads(batch.run.shell('orders', script))


def redis_rotation(batch):
    run, api = batch.run, batch.api
    if (run.out / 'redis-rotation.json').exists():
        return
    target = batch.map['targets']['redis']
    path = '/monitoring/collections/' + target['collection_id']
    journal = run.private / 'redis-rotation.json'
    private = run.private / 'redis-credentials.json'
    saved = json.loads(private.read_text(encoding='utf-8'))
    if not journal.exists():
        current = api.call('GET', path)
        assert current['health'] == 'healthy' and current['target']['id'] == target['target_id']
        assert native_redis(batch, {'old_before': saved['monitor']}) == {'old_before': 'authenticated'}
        write_json(journal, {'old': saved['monitor'], 'new': secrets.token_urlsafe(32), 'at': now(), 'collection': current, 'stage': 'prepared'})
    state = json.loads(journal.read_text(encoding='utf-8'))
    if state['stage'] == 'prepared':
        acl = run.private / 'users.acl'
        old_hash, new_hash = [hashlib.sha256(state[key].encode()).hexdigest() for key in ['old', 'new']]
        prior = acl.read_text(encoding='utf-8')
        assert 'user monitor on #' + old_hash in prior or 'user monitor on #' + new_hash in prior
        acl.write_text(prior.replace('user monitor on #' + old_hash, 'user monitor on #' + new_hash), encoding='utf-8')
        run.container('redis')
        run.compose('restart', '-t', '5', 'redis', timeout=30)
        run.container('redis')
        state['stage'] = 'native-rotated'
        write_json(journal, state)
    probes = native_redis(batch, {'old': state['old'], 'new': state['new']})
    assert probes == {'old': 'password_denied', 'new': 'authenticated'}, probes
    if state['stage'] == 'native-rotated':
        row = state['collection']
        body = {'kind': 'collection', 'operation': 'update', 'id': row['id'], 'expected_version': row['version'],
                'collection': {key: row[key] for key in ['name', 'integration', 'target', 'collector_id', 'config']}}
        body['collection'].update(enabled=True, credential_update='replace')
        body['credentials'] = {'username': 'monitor', 'password': state['new']}
        preview = api.call('POST', '/monitoring/commands/preview', body, key='redis-private-credential-rotation', expected=201)
        assert state['new'] not in json.dumps(preview) and state['old'] not in json.dumps(preview)
        result = api.confirm(preview)
        state.update(stage='platform-updated', activated_at=now(), execution=result['execution']['execution_id'])
        write_json(journal, state)
        saved['monitor'] = state['new']
        write_json(private, saved)
    batch.converge('orders')
    row = api.wait(path, lambda x: x['health'] == 'healthy' and x['freshness'] == 'fresh' and x.get('last_sample_at', '') > state['activated_at'],
                   label='Fresh actual Redis samples using the replacement credential', timeout=100)
    assert row['id'] == state['collection']['id'] and row['target']['id'] == target['target_id']
    assert row['version'] == state['collection']['version'] + 1
    for key in ['orders', 'reports']:
        assert json.loads(run.shell(key, 'curl -fsS http://127.0.0.1:18085/health\n'))['run'] == run.id
    run.save('redis-rotation', {'passed': True, 'at': now(), 'native_probes': probes, 'versions': [state['collection']['version'], row['version']],
              'collection_id': row['id'], 'target_id': target['target_id'], 'execution': state['execution'], 'last_sample_at': row['last_sample_at'],
              'preview_contains_no_password': True, 'actual_restart_used_for_acl_cutover': True, 'both_real_apps_healthy_after': True})
    write_json(journal, {'stage': 'verified', 'at': now()})
    print('Native Redis old monitoring authentication rejected, new accepted, same Argus collection received fresh samples', flush=True)


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('step', choices=['ssh', 'redis'])
    run = Run()
    api = run.api()
    api.login()
    {'ssh': ssh_rotation, 'redis': redis_rotation}[parser.parse_args().step](Batch(run, api))
