"""Protect evidence/cleanup ownership when two acceptance stages share code."""
from pathlib import Path
import sys
import unittest
import tempfile
import json

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from B13a.manage import Run as Previous
from B13b.manage import Run as Current


class RunIsolation(unittest.TestCase):
    def test_stage_paths_and_manifests_are_disjoint(self):
        previous, current = Previous(), Current()
        self.assertNotEqual(previous.out, current.out)
        self.assertEqual(previous.out.name, previous.id)
        self.assertEqual(current.out.name, current.id)
        self.assertNotEqual(previous.private, current.private)
        self.assertNotEqual(previous.resources, current.resources)
        self.assertNotEqual(previous.envfile, current.envfile)
        self.assertNotEqual(previous.PACKAGE, current.PACKAGE)
        self.assertEqual(current.PREDECESSOR, previous.id)
        self.assertNotIn('/root/plan9-redis.json', (current.PACKAGE / 'compose.yaml').read_text(encoding='utf-8'))

    def test_prepare_rejects_foreign_owner_before_any_private_write(self):
        with tempfile.TemporaryDirectory() as temporary:
            current = Current()
            current.resources = Path(temporary) / 'resource-map.json'
            current.resources.write_text(json.dumps({'run_id': Previous().id}), encoding='utf-8')
            current.private = Path(temporary) / 'must-not-be-created'
            with self.assertRaises(AssertionError):
                current.prepare()
            self.assertFalse(current.private.exists())


if __name__ == '__main__':
    unittest.main()
