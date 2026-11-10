"""Regression checks for the lab-only static-Pod preparation boundary."""
import copy
import unittest

import yaml

from environment import updated_manifest, digest


class EndpointPreparationTests(unittest.TestCase):
    def manifest(self, name):
        command = [name, '--bind-address=127.0.0.1', '--authentication-kubeconfig=/original/auth.conf',
                   '--authorization-kubeconfig=/original/auth.conf', '--leader-elect=true']
        if name == 'etcd':
            command = [name, '--listen-metrics-urls=http://127.0.0.1:2381', '--client-cert-auth=true',
                       '--peer-client-cert-auth=true', '--data-dir=/var/lib/etcd']
        return {'spec': {'containers': [{'name': name, 'command': command,
                'volumeMounts': [{'name': 'existing', 'mountPath': '/original'}]}],
                'volumes': [{'name': 'existing', 'hostPath': {'path': '/original'}}]}}

    def test_keeps_component_authentication_and_existing_mounts(self):
        for name in ('kube-scheduler', 'kube-controller-manager'):
            with self.subTest(name=name):
                before = self.manifest(name)
                original = copy.deepcopy(before)
                after = yaml.safe_load(updated_manifest(yaml.safe_dump(before), name))
                command = after['spec']['containers'][0]['command']
                for flag in original['spec']['containers'][0]['command'][2:]:
                    self.assertIn(flag, command)
                self.assertFalse(any('anonymous' in c or 'always-allow' in c for c in command))
                self.assertIn('--bind-address=0.0.0.0', command)
                self.assertEqual(after['spec']['volumes'][0], original['spec']['volumes'][0])
                self.assertTrue(after['spec']['containers'][0]['volumeMounts'][-1]['readOnly'])

    def test_etcd_changes_metrics_listener_without_client_or_peer_auth(self):
        before = self.manifest('etcd')
        after = yaml.safe_load(updated_manifest(yaml.safe_dump(before), 'etcd'))
        before['spec']['containers'][0]['command'][1] = '--listen-metrics-urls=http://0.0.0.0:2381'
        self.assertEqual(after, before)

    def test_rejects_unknown_listener_and_existing_tls_configuration(self):
        for flag in ('--bind-address=192.0.2.1', '--bind-address=0.0.0.0'):
            before = self.manifest('kube-scheduler')
            before['spec']['containers'][0]['command'][1] = flag
            with self.assertRaises(AssertionError):
                updated_manifest(yaml.safe_dump(before), 'kube-scheduler')
        before = self.manifest('kube-scheduler')
        before['spec']['containers'][0]['command'].append('--tls-cert-file=/another.crt')
        with self.assertRaises(AssertionError):
            updated_manifest(yaml.safe_dump(before), 'kube-scheduler')

    def test_windows_line_endings_do_not_mask_content_changes(self):
        self.assertEqual(digest('name: test\n'), digest('name: test\r\n'))
        self.assertNotEqual(digest('name: test\n'), digest('name: other\r\n'))


if __name__ == '__main__':
    unittest.main()
