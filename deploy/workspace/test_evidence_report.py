import hashlib
import json
from pathlib import Path
import re
import tempfile
import unittest
import subprocess
import sys
from unittest.mock import patch
from uuid import uuid4

import evidence_report
import report_receipt


class EvidenceReportTest(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.root = Path(temp.name)
        self.query = self.root / "query.json"
        self.query.write_text(json.dumps({"status": "success", "query_context": {"resources": [{"type": "host", "id": "host"}]}, "data": {"resultType": "matrix", "result": [{"metric": {"__name__": "system_network_io", "argus_resource_id": "host", "direction": "receive"}, "values": [[1793447566, str(2**30)], [1793447656, str(3 * 2**30)]]}]}}), encoding="utf-8")
        value = json.loads(self.query.read_text())
        value['argus_meta'] = {'input_metrics_origin': 'authorized_series_metadata', 'input_metrics_complete': True, 'result_is_original_metric': True, 'input_metrics': [{'name': 'system_network_io', 'type': 'counter', 'storage_type': 'sum', 'temporality': 'cumulative', 'monotonic': True, 'unit': 'By', 'source_key': 'source:2'}]}
        self.query.write_text(json.dumps(value), encoding='utf-8')
        self.job, self.attempt, self.file = [str(uuid4()) for _ in range(3)]
        self.panel = self.root / (self.file + ".json")
        self.panel.write_text(json.dumps([{"metric": {"argus_resource_id": "host"}, "values": [[1793447566, "75.8"], [1793447656, "74.4"]]}]), encoding="utf-8")
        self.manifest = self.root / "manifest.json"
        self.value = {"schema_version": "argus.dashboard_query_manifest/v1", "job_id": self.job, "attempt_id": self.attempt, "complete": True,
            "execution": {"from": "2026-10-31T11:49:16Z", "to": "2026-10-31T11:54:16Z", "resources": [{"type": "host", "id": "host"}], "panels": [{"id": "memory", "targets": [{"id": "main", "status": "success", "result_type": "matrix", "meta": {"partial": False}}]}]},
            "definition": {"panels": [{"id": "memory", "unit": "%"}]},
            "files": [{"id": self.file, "panel_id": "memory", "target_id": "main", "kind": "data", "bytes": self.panel.stat().st_size, "sha256": hashlib.sha256(self.panel.read_bytes()).hexdigest(), "source_ref": f"dashboard-query:{self.job}/{self.attempt}", "path": f"/workspace/queries/{self.job}/{self.attempt}/{self.file}.json"}]}
        self.save_manifest()

    def save_manifest(self):
        self.manifest.write_text(json.dumps(self.value), encoding="utf-8")

    def test_complete_program_tables_and_identical_report_json(self):
        digest = hashlib.sha256(self.query.read_bytes()).hexdigest()
        result, markdown, inputs = evidence_report.compose(self.query, "By", expected_sha256=digest, manifests=[self.manifest], metric_type="counter")
        self.assertEqual(result["metric_evidence"]["display_min"], 1)
        self.assertEqual(result["metric_evidence"]["series"][0]["display_last_point_value"], 3)
        self.assertTrue(result["metric_evidence"]["expected_sha256_verified"])
        self.assertEqual(result["metric_evidence"]["metric_type"], "counter")
        self.assertIn("cumulative readings", markdown)
        self.assertEqual(result["dashboard_evidence"][0]["panels"][0]["unit"], "%")
        self.assertEqual(result["dashboard_evidence"][0]["panels"][0]["value_max"], 75.8)
        blocks = re.findall(r'```json\s*(.*?)```', markdown, re.S)
        self.assertEqual(len(blocks), 1)
        self.assertEqual(json.loads(blocks[0]), result)
        self.assertIn("| First UTC | First raw | First display |", markdown)
        self.assertIn("| memory/main | numeric | 2 |", markdown)
        json_out, markdown_out = self.root / "out.json", self.root / "out.md"
        evidence_report.save_outputs(result, markdown, inputs, json_out, markdown_out)
        self.assertEqual(json.loads(json_out.read_text()), result)
        self.assertEqual(markdown_out.read_text(), markdown)

    def test_metric_only_keeps_flat_contract_and_notes_precede_final_facts(self):
        notes = self.root / "notes.md"
        notes.write_text("Observed facts do not establish a cause.", encoding="utf-8")
        result, markdown, inputs = evidence_report.compose(self.query, "By", notes=notes)
        self.assertEqual(result["metric"], "system_network_io")
        self.assertNotIn("metric_evidence", result)
        self.assertLess(markdown.index("Observed facts"), markdown.index("# Evidence summary"))
        self.assertIn(notes.resolve(), inputs)

    def test_tampered_metric_or_panel_cannot_be_recomposed_as_original(self):
        with self.assertRaisesRegex(ValueError, "input SHA256 differs"):
            evidence_report.compose(self.query, "By", expected_sha256="0" * 64)
        self.panel.write_bytes(self.panel.read_bytes() + b" ")
        with self.assertRaises(ValueError):
            evidence_report.compose(self.query, "By", manifests=[self.manifest])

    def test_duplicate_jobs_and_over_limit_manifests_are_rejected(self):
        with self.assertRaisesRegex(ValueError, "repeats a dashboard"):
            evidence_report.compose(self.query, "By", manifests=[self.manifest, self.manifest])
        with self.assertRaisesRegex(ValueError, "dashboard limit"):
            evidence_report.compose(self.root / "absent", "By", manifests=[self.manifest] * 9)

    def test_no_output_can_replace_any_input_or_the_other_output(self):
        result, markdown, inputs = evidence_report.compose(self.query, "By", manifests=[self.manifest])
        for original in (self.query, self.manifest, self.panel):
            before = original.read_bytes()
            with self.assertRaises(ValueError):
                evidence_report.save_outputs(result, markdown, inputs, original, self.root / "out.md")
            self.assertEqual(original.read_bytes(), before)
        with self.assertRaises(ValueError):
            evidence_report.save_outputs(result, markdown, inputs, self.root / "same", self.root / "same")

    def test_combined_byte_and_point_budgets_are_not_per_input_only(self):
        with patch.object(evidence_report, "MAX_SOURCE_BYTES", 1):
            with self.assertRaisesRegex(ValueError, "file/byte limit"):
                evidence_report.compose(self.query, "By", manifests=[self.manifest])
        with patch.object(evidence_report.metric_evidence, "MAX_POINTS", 3):
            with self.assertRaisesRegex(ValueError, "combined point limit"):
                evidence_report.compose(self.query, "By", manifests=[self.manifest])

    def test_no_data_and_partial_state_remain_explicit(self):
        self.panel.write_text("[]", encoding="utf-8")
        self.value["files"][0].update(bytes=2, sha256=hashlib.sha256(b"[]").hexdigest())
        self.value["execution"]["partial"] = True
        self.value["complete"] = False
        self.save_manifest()
        result, markdown, _ = evidence_report.compose(self.query, "By", manifests=[self.manifest])
        dashboard = result["dashboard_evidence"][0]
        self.assertTrue(dashboard["partial"])
        self.assertFalse(dashboard["complete"])
        self.assertEqual(dashboard["panels"][0]["status"], "no_data")
        self.assertNotIn("value_min", dashboard["panels"][0])
        self.assertIn("no_data", markdown)

    def test_bilingual_reports_keep_identical_machine_evidence_and_originals(self):
        original_hash = hashlib.sha256(self.query.read_bytes()).hexdigest()
        english, english_md, _ = evidence_report.compose(self.query, manifests=[self.manifest], expected_sha256=original_hash)
        chinese, chinese_md, _ = evidence_report.compose(self.query, manifests=[self.manifest], expected_sha256=original_hash, locale="zh-CN")
        self.assertEqual(chinese, english)
        self.assertEqual(hashlib.sha256(self.query.read_bytes()).hexdigest(), original_hash)
        self.assertIn("# 指标证据", chinese_md)
        self.assertIn("# 仪表盘面板证据", chinese_md)
        self.assertIn("| 序列 | 点数 | 原始最小值 |", chinese_md)
        self.assertIn("考虑计数器重置", chinese_md)
        self.assertIn("display = raw / 2**30", chinese_md)
        self.assertIn("| memory/main | 数值 |", chinese_md)
        for markdown in (chinese_md, english_md):
            blocks = re.findall(r'```json\s*(.*?)```', markdown, re.S)
            self.assertEqual(len(blocks), 1)
            self.assertEqual(json.loads(blocks[0]), english)
        with self.assertRaisesRegex(ValueError, "locale"):
            evidence_report.compose(self.query, locale="unrecognized")

    def test_chinese_combined_cli_writes_complete_report_without_manual_splicing(self):
        json_out, markdown_out = self.root / "combined.json", self.root / "combined.md"
        result = subprocess.run([sys.executable, str(Path(evidence_report.__file__)), str(self.query), "--dashboard-manifest", str(self.manifest), "--locale", "zh-CN", "--json-out", str(json_out), "--markdown-out", str(markdown_out)], capture_output=True, text=True, encoding="utf-8", check=False)
        self.assertEqual(result.returncode, 0, result.stderr)
        encoded = json.loads(json_out.read_text(encoding="utf-8"))
        receipt = json.loads(result.stdout)
        self.assertEqual(receipt["schema_version"], "argus.evidence_report_receipt/v1")
        self.assertEqual(receipt["status"], "saved_in_workspace")
        self.assertFalse(receipt["published"])
        self.assertEqual(receipt["dashboard_evidence_shape"], "array")
        self.assertEqual(receipt["dashboard_jobs"][0]["panel_target_count"], 1)
        self.assertEqual(receipt["markdown_file"]["sha256"], hashlib.sha256(markdown_out.read_bytes()).hexdigest())
        self.assertEqual(receipt["markdown_file"]["bytes"], len(markdown_out.read_bytes()))
        self.assertEqual(receipt["json_file"]["sha256"], hashlib.sha256(json_out.read_bytes()).hexdigest())
        self.assertLessEqual(len(result.stdout.encode("utf-8")), report_receipt.MAX_RECEIPT_BYTES + 2)
        markdown = markdown_out.read_text(encoding="utf-8")
        blocks = re.findall(r'```json\s*(.*?)```', markdown, re.S)
        self.assertEqual(len(blocks), 1)
        self.assertEqual(json.loads(blocks[0]), encoded)
        self.assertEqual(encoded["metric_evidence"]["display_unit"], "GiB")
        self.assertEqual(len(encoded["dashboard_evidence"][0]["panels"]), 1)
        self.assertIn("# 完整证据", markdown)

    def test_explicit_print_evidence_still_returns_the_complete_original_shape(self):
        json_out, markdown_out = self.root / "full.json", self.root / "full.md"
        result = subprocess.run([sys.executable, str(Path(evidence_report.__file__)), str(self.query), "--dashboard-manifest", str(self.manifest), "--print-evidence", "--json-out", str(json_out), "--markdown-out", str(markdown_out)], capture_output=True, text=True, encoding="utf-8", check=False)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout), json.loads(json_out.read_bytes()))

    def test_bounded_receipt_distinguishes_total_and_per_series_counts(self):
        query = json.loads(self.query.read_text(encoding="utf-8"))
        first = query["data"]["result"][0]
        query["data"]["result"] = [dict(first, metric=dict(first["metric"], device=f"device-{i}")) for i in range(500)]
        self.query.write_text(json.dumps(query), encoding="utf-8")
        conclusion, markdown, inputs = evidence_report.compose(self.query)
        json_out, markdown_out = self.root / "many.json", self.root / "many.md"
        evidence_report.save_outputs(conclusion, markdown, inputs, json_out, markdown_out)
        wire = report_receipt.create(conclusion, markdown, json_out, markdown_out, inputs)
        receipt = json.loads(wire)
        self.assertEqual(receipt["metric_summary"]["total_point_count"], 1000)
        self.assertEqual(receipt["metric_summary"]["series_count"], 500)
        self.assertEqual(receipt["metric_summary"]["min_points_per_series"], 2)
        self.assertEqual(receipt["metric_summary"]["max_points_per_series"], 2)
        self.assertLessEqual(len(wire.encode()), 8192)
        self.assertGreater(json_out.stat().st_size, len(wire.encode()) * 10)
        self.assertEqual(len(json.loads(json_out.read_bytes())["series"]), 500)

    def test_changed_output_cannot_receive_a_successful_receipt(self):
        conclusion, markdown, inputs = evidence_report.compose(self.query)
        json_out, markdown_out = self.root / "verify.json", self.root / "verify.md"
        evidence_report.save_outputs(conclusion, markdown, inputs, json_out, markdown_out)
        markdown_out.write_bytes(markdown_out.read_bytes() + b"changed")
        with self.assertRaisesRegex(ValueError, "differs from the generated bytes"):
            report_receipt.create(conclusion, markdown, json_out, markdown_out, inputs)

    def test_receipt_preserves_partial_and_unknown_instead_of_asserting_complete(self):
        value = json.loads(self.query.read_text(encoding="utf-8"))
        for partial in (True, False, None):
            if partial is None:
                value["argus_meta"].pop("partial", None)
            else:
                value["argus_meta"]["partial"] = partial
            self.query.write_text(json.dumps(value), encoding="utf-8")
            conclusion, markdown, inputs = evidence_report.compose(self.query)
            json_out, markdown_out = self.root / "partial.json", self.root / "partial.md"
            evidence_report.save_outputs(conclusion, markdown, inputs, json_out, markdown_out)
            receipt = json.loads(report_receipt.create(conclusion, markdown, json_out, markdown_out, inputs))
            self.assertIs(receipt["metric_summary"]["query_partial"], partial)
            self.assertTrue(receipt["metric_summary"]["limitations"])

    def test_extra_or_conflicting_json_in_notes_fails_before_output(self):
        notes = self.root / "notes.md"
        notes.write_text('```json\n{"not_the_evidence":true}\n```', encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "exactly one complete"):
            evidence_report.compose(self.query, notes=notes)


if __name__ == "__main__":
    unittest.main()
