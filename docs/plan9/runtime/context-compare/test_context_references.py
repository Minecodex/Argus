import json
import unittest
from context_references import FileReference, ReferenceCatalog, InvocationLedger, digest, verify_content


class ReferencesTest(unittest.TestCase):
    def setUp(self):
        import hashlib
        self.body = b'{"counter":900719925474099312345}'
        self.ref = FileReference('opaque-file-id', 'workspace-a',
                                 '/workspace/结果 741dc561-54a5-4550-b329-52f396548fe1.json',
                                 hashlib.sha256(self.body).hexdigest(), len(self.body), 'result-a')
        self.catalog = ReferenceCatalog('scope-a', 'workspace-a', 152)
        self.catalog.register(self.ref, authorization_scope='scope-a', source_sequence=150)

    def test_exact_reference_survives_repeated_lossy_summaries(self):
        expected = self.catalog.project(authorization_scope='scope-a', workspace_id='workspace-a')
        for _ in range(10):
            lossy_summary = 'The file is at /workspace/结果 741dc561….json'
            context = json.loads(json.dumps({'summary': lossy_summary, 'references': expected}, ensure_ascii=False))
            self.assertEqual(context['references']['files'][0]['path'], self.ref.path)
            self.assertEqual(context['references']['context_hash'], expected['context_hash'])

    def test_permission_and_workspace_change_remove_old_references(self):
        for scope, workspace in [('scope-b', 'workspace-a'), ('scope-a', 'workspace-b')]:
            value = self.catalog.project(authorization_scope=scope, workspace_id=workspace)
            self.assertFalse(value['available'])
            self.assertEqual(value['files'], [])

    def test_identity_conflict_is_not_silently_replaced(self):
        changed = FileReference(self.ref.file_id, self.ref.workspace_id, self.ref.path, '0'*64, 0, 'result-b')
        with self.assertRaisesRegex(ValueError, 'IDENTITY_CONFLICT'):
            self.catalog.register(changed, authorization_scope='scope-a', source_sequence=151)

    def test_concurrent_future_result_is_not_mixed_into_snapshot(self):
        with self.assertRaisesRegex(ValueError, 'CONTEXT_BOUNDARY'):
            self.catalog.register(self.ref, authorization_scope='scope-a', source_sequence=153)

    def test_budget_preserves_whole_references_and_marks_incomplete(self):
        value = self.catalog.project(authorization_scope='scope-a', workspace_id='workspace-a', max_items=0)
        self.assertFalse(value['complete'])
        self.assertEqual(value['omitted_count'], 1)
        self.assertEqual(value['files'], [])
        self.assertIn('recovery', value)

    def test_content_hash_checks_original_bytes(self):
        verify_content(self.ref, self.body)
        with self.assertRaisesRegex(ValueError, 'CONTENT_CHANGED'):
            verify_content(self.ref, self.body + b' ')

    def test_byte_budget_includes_hash_and_incomplete_notice(self):
        for limit in [600,800,1000,2000]:
            try:
                value = self.catalog.project(authorization_scope='scope-a', workspace_id='workspace-a', max_bytes=limit)
            except ValueError as error:
                self.assertIn('BUDGET',str(error))
            else:
                self.assertLessEqual(len(json.dumps(value).encode()),limit)
                if not value['complete']:
                    self.assertEqual(value['files'],[])

    def test_retries_reuse_identity_without_guessing_intent(self):
        ledger = InvocationLedger()
        args = {'path': self.ref.path}
        self.assertIsNone(ledger.begin('call-a', args, 'scope-a'))
        ledger.finish('call-a', {'bytes': 1})
        self.assertEqual(ledger.begin('call-a', args, 'scope-a')['result'], {'bytes': 1})
        self.assertIsNone(ledger.begin('call-b', args, 'scope-a'))
        with self.assertRaisesRegex(ValueError, 'IDENTITY_CONFLICT'):
            ledger.begin('call-a', args, 'scope-b')

    def test_unknown_results_cannot_be_automatically_replayed(self):
        ledger = InvocationLedger()
        ledger.begin('call-a', {'operation': 'mutate'}, 'scope-a')
        ledger.finish('call-a', None, 'unknown')
        self.assertEqual(ledger.begin('call-a', {'operation': 'mutate'}, 'scope-a')['status'], 'unknown')
        with self.assertRaisesRegex(ValueError, 'ALREADY_TERMINAL'):
            ledger.finish('call-a', {'done': True})

    def test_reference_validation_allows_ordinary_short_names(self):
        FileReference('id', 'ws', '/workspace/12345678.json', 'a'*64, 0, 'result')
        for path in ['/workspace/../secret', '/workspace/a/../../secret', '/elsewhere/file']:
            with self.assertRaises(ValueError):
                FileReference('id', 'ws', path, 'a'*64, 0, 'result')


if __name__ == '__main__':
    unittest.main()
