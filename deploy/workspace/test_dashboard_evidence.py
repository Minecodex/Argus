import hashlib
import json
from pathlib import Path
import tempfile
import unittest
from uuid import uuid4
from unittest.mock import patch

from dashboard_evidence import main, summarize_manifest


class DashboardEvidenceTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.job, self.attempt, self.file_id = [str(uuid4()) for _ in range(3)]
        self.rows = [{"metric": {"argus_resource_id": "host"}, "values": [[1793447566, "75.823569709"], [1793447656, "74.459892779"]]}]
        self.body = json.dumps(self.rows).encode()
        self.manifest = {"schema_version": "argus.dashboard_query_manifest/v1", "job_id": self.job, "attempt_id": self.attempt, "complete": True,
            "execution": {"from": "2026-10-31T11:49:16Z", "to": "2026-10-31T11:54:16Z", "resources": [{"type": "host", "id": "host"}], "panels": [{"id": "memory", "targets": [{"id": "main", "status": "success", "result_type": "matrix", "meta": {"returned_rows": 1, "partial": False}}]}]},
            "definition": {"panels": [{"id": "memory", "unit": "%"}]},
            "files": [{"id": self.file_id, "panel_id": "memory", "target_id": "main", "kind": "data", "bytes": len(self.body), "sha256": hashlib.sha256(self.body).hexdigest(), "source_ref": f"dashboard-query:{self.job}/{self.attempt}", "path": f"/workspace/queries/{self.job}/{self.attempt}/{self.file_id}.json"}]}

    def write(self):
        (self.root / (self.file_id + ".json")).write_bytes(self.body)
        path = self.root / "manifest.json"
        path.write_text(json.dumps(self.manifest), encoding="utf-8")
        return path

    def test_real_epoch_conversion_and_panel_unit_are_preserved(self):
        path = self.write()
        result, inputs = summarize_manifest(path, hashlib.sha256(path.read_bytes()).hexdigest())
        panel = result["panels"][0]
        self.assertEqual(panel["first_point_at"], "2026-10-31T11:52:46.000Z")
        self.assertEqual(panel["last_point_at"], "2026-10-31T11:54:16.000Z")
        self.assertEqual(panel["point_count"], 2)
        self.assertEqual(panel["value_min"], 74.459892779)
        self.assertEqual(panel["first_point_value"], 75.823569709)
        self.assertNotEqual(panel["first_point_value"], panel["value_min"])
        self.assertEqual(panel["last_point_value"], 74.459892779)
        self.assertEqual(panel["display_min"], panel["value_min"])
        self.assertEqual(panel["unit"], "%")
        self.assertEqual(panel["evaluation_window"]["requested_duration_seconds"], 300)
        self.assertEqual(panel["evaluation_window"]["returned_span_seconds"], 90)
        self.assertEqual(panel["evaluation_window"]["first_offset_seconds"], 210)
        self.assertEqual(panel["evaluation_window"]["first_vs_requested_from"], "after")
        self.assertNotIn("expected_manifest_sha256_verified", result)
        self.assertEqual(result["manifest_checksum_status"], "verified")
        self.assertEqual(len(inputs), 2)
        from dashboard_evidence import render_markdown
        self.assertIn("First value", render_markdown(result))
        self.assertIn("首点值", render_markdown(result, locale="zh-CN"))

    def test_aggregation_does_not_invent_an_original_metric_or_resource_label(self):
        self.rows[0]["metric"] = {"state": "used"}
        self.body = json.dumps(self.rows).encode()
        self.manifest["files"][0].update(bytes=len(self.body), sha256=hashlib.sha256(self.body).hexdigest())
        result, _ = summarize_manifest(self.write())
        self.assertEqual(result["panels"][0]["series"][0]["labels"], {"state": "used"})
        self.assertNotIn("metric", result["panels"][0])
        self.assertEqual(result["resource_refs"][0]["id"], "host")

    def test_multiple_series_keep_their_own_endpoints(self):
        self.rows.append({"metric": {"argus_resource_id": "host", "state": "other"}, "values": [[1793447566, "10"], [1793447656, "20"]]})
        self.body = json.dumps(self.rows).encode()
        self.manifest["files"][0].update(bytes=len(self.body), sha256=hashlib.sha256(self.body).hexdigest())
        self.manifest["execution"]["panels"][0]["targets"][0]["meta"]["returned_rows"] = 2
        result, _ = summarize_manifest(self.write())
        panel = result["panels"][0]
        self.assertNotIn("first_point_value", panel)
        self.assertEqual(panel["series"][0]["first_point_value"], 75.823569709)
        self.assertEqual(panel["series"][1]["first_point_value"], 10)
        self.assertEqual(panel["series"][1]["last_point_value"], 20)
        from dashboard_evidence import render_markdown
        text = render_markdown(result)
        self.assertIn("75.823569709", text)
        self.assertIn('state', text)

    def test_missing_expected_manifest_checksum_is_not_a_mismatch(self):
        result, _ = summarize_manifest(self.write())
        self.assertNotIn("expected_manifest_sha256_verified", result)
        self.assertEqual(result["manifest_checksum_status"], "not_requested")
        self.assertIn("not a checksum mismatch", result["manifest_checksum_explanation"])
        from dashboard_evidence import render_markdown
        self.assertIn("未请求", render_markdown(result, locale="zh-CN"))
        self.assertIn("not_requested", render_markdown(result, locale="en-US"))

    def test_changed_original_size_hash_and_manifest_hash_are_rejected(self):
        for key, bad in (("bytes", len(self.body) + 1), ("sha256", "0" * 64)):
            with self.subTest(key=key):
                old = self.manifest["files"][0][key]
                self.manifest["files"][0][key] = bad
                with self.assertRaises(ValueError): summarize_manifest(self.write())
                self.manifest["files"][0][key] = old
        with self.assertRaises(ValueError): summarize_manifest(self.write(), "0" * 64)

    def test_cross_attempt_and_traversal_are_rejected(self):
        for field, bad in (("source_ref", f"dashboard-query:{self.job}/{uuid4()}"), ("path", f"/workspace/queries/{self.job}/{self.attempt}/../{self.file_id}.json")):
            with self.subTest(field=field):
                old = self.manifest["files"][0][field]
                self.manifest["files"][0][field] = bad
                with self.assertRaises(ValueError): summarize_manifest(self.write())
                self.manifest["files"][0][field] = old

    def test_foreign_resource_and_outside_time_are_rejected(self):
        self.rows[0]["metric"]["argus_resource_id"] = "foreign"
        self.body = json.dumps(self.rows).encode()
        self.manifest["files"][0].update(bytes=len(self.body), sha256=hashlib.sha256(self.body).hexdigest())
        with self.assertRaises(ValueError): summarize_manifest(self.write())
        self.rows[0]["metric"]["argus_resource_id"] = "host"
        self.rows[0]["values"][0][0] = 1
        self.body = json.dumps(self.rows).encode()
        self.manifest["files"][0].update(bytes=len(self.body), sha256=hashlib.sha256(self.body).hexdigest())
        with self.assertRaises(ValueError): summarize_manifest(self.write())

    def test_missing_and_non_numeric_files_are_explicit(self):
        self.manifest["files"] = []
        result, _ = summarize_manifest(self.write())
        self.assertFalse(result["complete"])
        self.assertEqual(result["panels"][0]["status"], "unavailable")
        self.manifest["execution"]["panels"][0]["targets"][0]["result_type"] = "log_entries"
        result, _ = summarize_manifest(self.write())
        self.assertNotIn("value_min", result["panels"][0])

    def test_vector_and_partial_output_are_preserved(self):
        self.rows[0]["value"] = self.rows[0].pop("values")[0]
        self.body = json.dumps(self.rows).encode()
        self.manifest["files"][0].update(bytes=len(self.body), sha256=hashlib.sha256(self.body).hexdigest())
        target = self.manifest["execution"]["panels"][0]["targets"][0]
        target.update(result_type="vector", meta={"partial": True})
        result, _ = summarize_manifest(self.write())
        self.assertTrue(result["partial"])
        self.assertEqual(result["panels"][0]["point_count"], 1)

    def test_empty_numeric_data_is_not_a_zero_value(self):
        self.body = b"[]"
        self.manifest["files"][0].update(bytes=2, sha256=hashlib.sha256(self.body).hexdigest())
        result, _ = summarize_manifest(self.write())
        self.assertEqual(result["panels"][0]["status"], "no_data")
        self.assertNotIn("value_min", result["panels"][0])

    def test_cli_cannot_replace_manifest_or_panel_original(self):
        path = self.write()
        for output in (path, self.root / (self.file_id + ".json")):
            before = output.read_bytes()
            with self.subTest(output=output.name), patch("sys.argv", ["dashboard-evidence", str(path), "--json-out", str(output)]), patch("sys.stderr"):
                self.assertEqual(main(), 2)
            self.assertEqual(output.read_bytes(), before)

    def test_cli_generates_exact_utc_table_and_evidence_block(self):
        path = self.write()
        output = self.root / "report.md"
        with patch("sys.argv", ["dashboard-evidence", str(path), "--markdown-out", str(output)]), patch("sys.stdout"):
            self.assertEqual(main(), 0)
        text = output.read_text(encoding="utf-8")
        self.assertIn("2026-10-31T11:52:46.000Z", text)
        self.assertIn("argus.dashboard_evidence/v2", text)
        self.assertIn("| memory/main | numeric | 2 |", text)


if __name__ == "__main__":
    unittest.main()
