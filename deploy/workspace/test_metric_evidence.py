import importlib.util
import hashlib
import json
from pathlib import Path
import unittest
import subprocess
import sys
import tempfile

spec = importlib.util.spec_from_file_location("metric_evidence", Path(__file__).with_name("metric_evidence.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class MetricEvidenceTest(unittest.TestCase):
    def payload(self, points, state="used", name="system_memory_usage"):
        return {"status": "success", "query_context": {"resources": [{"type": "host", "id": "host"}]}, "data": {"resultType": "matrix", "result": [{"metric": {"__name__": name, "state": state}, "values": points}]}}

    def test_dashboard_array_misuse_returns_actionable_cli_error_without_partial_output(self):
        for value in ([], [{"metric": {}, "values": [[1, "2"]]}], None, {"status": "success", "data": []}):
            with self.subTest(value=value):
                with self.assertRaises(ValueError):
                    module.summarize(json.dumps(value).encode(), "By")
        with tempfile.TemporaryDirectory() as temporary:
            source, destination = Path(temporary) / "panel.json", Path(temporary) / "wrong-report.md"
            source.write_text('[{"metric":{},"values":[[1,"2"]]}]', encoding="utf-8")
            result = subprocess.run([sys.executable, str(Path(module.__file__)), str(source), "--markdown-out", str(destination)], capture_output=True, text=True, encoding="utf-8", check=False)
            self.assertEqual(result.returncode, 2)
            self.assertIn("argus-dashboard-evidence MANIFEST", result.stderr)
            self.assertNotIn("Traceback", result.stderr)
            self.assertFalse(destination.exists())

    def test_real_extrema_and_binary_conversion(self):
        data = self.payload([[1793337323, "13010493440"], [1793337383, "13388550144"], [1793337443, "12972355584"]])
        result = module.summarize(json.dumps(data).encode(), "By")
        self.assertEqual(result["value_min"], 12972355584)
        self.assertEqual(result["display_min"], 12972355584 / 2**30)
        self.assertNotEqual(result["display_min"], 12972355584 / 1e9)
        self.assertEqual(result["first_point_at"], "2026-10-30T05:15:23.000Z")

    def test_multi_state_cpu_is_not_busy(self):
        data = self.payload([[1, "0.9"]], "idle", "system_cpu_utilization")
        result = module.summarize(json.dumps(data).encode(), "1", "invert_percent")
        self.assertAlmostEqual(result["display_min"], 10)
        data["data"]["result"].append({"metric": {"__name__": "system_cpu_utilization", "state": "user"}, "values": [[1, "0.1"]]})
        with self.assertRaises(ValueError):
            module.summarize(json.dumps(data).encode(), "1", "invert_percent")
        result = module.summarize(json.dumps(data).encode(), "1", "percent")
        self.assertEqual(result["display_transform"], "percent")
        self.assertEqual(result["series_count"], 2)

    def test_dimensionless_status_code_is_not_automatically_a_percentage(self):
        data = self.payload([[1, "200"], [2, "503"]], name="probe_http_status_code")
        result = module.summarize(json.dumps(data).encode(), "1")
        self.assertEqual(result["display_transform"], "identity")
        self.assertEqual(result["display_max"], 503)

    def test_explicit_unbounded_ratio_may_exceed_one_hundred_percent(self):
        data = self.payload([[1, "2"]], name="service_load_ratio")
        result = module.summarize(json.dumps(data).encode(), "1", "percent")
        self.assertEqual(result["display_max"], 200)

    def test_missing_and_transformed_results_do_not_become_values(self):
        for value in ("NaN", "+Inf"):
            with self.assertRaises(ValueError):
                module.summarize(json.dumps(self.payload([[1, value]])).encode(), "By")
        data = self.payload([[1, "2"]])
        del data["data"]["result"][0]["metric"]["__name__"]
        with self.assertRaises(ValueError):
            module.summarize(json.dumps(data).encode(), "By")

    def test_instant_timestamp_is_derived_from_the_file(self):
        data = self.payload([])
        data["data"]["resultType"] = "vector"
        data["data"]["result"][0]["value"] = [1793344511, "223526912"]
        result = module.summarize(json.dumps(data).encode(), "By")
        self.assertEqual(result["point_count"], 1)
        self.assertEqual(result["first_point_at"], "2026-10-30T07:15:11.000Z")
        self.assertEqual(result["first_point_at"], result["last_point_at"])

    def test_legacy_identity_is_observed_not_invented_as_a_typed_scope(self):
        data = self.payload([[1, "2"]])
        del data["query_context"]
        data["data"]["result"][0]["metric"]["argus_resource_id"] = "host"
        result = module.summarize(json.dumps(data).encode(), "By")
        self.assertEqual(result["resource_id"], "host")
        self.assertEqual(result["resource_identity_origin"], "retained_metric_labels")
        self.assertEqual(result["resource_refs"], [])

    def returned_payload(self):
        data = self.payload([[1, "10"], [2, "20"]])
        data["data"]["result"].append({"metric": {"__name__": "system_memory_usage", "state": "free"}, "values": [[1, "30"], [2, "40"]]})
        data["argus_meta"] = {"loaded_samples": 500, "returned_rows": 2, "returned_points": {"count": 4, "from": "1970-01-01T00:00:01Z", "to": "1970-01-01T00:00:02Z"}}
        return data

    def test_original_hash_and_returned_metadata_are_verified(self):
        raw = json.dumps(self.returned_payload()).encode()
        digest = hashlib.sha256(raw).hexdigest()
        result = module.summarize(raw, "By", expected_sha256=digest.upper())
        self.assertEqual(result["input_sha256"], digest)
        self.assertTrue(result["expected_sha256_verified"])
        self.assertEqual(result["checked_returned_metadata"], ["returned_rows", "returned_points"])
        self.assertEqual(result["point_count"], 4)
        # Loaded raw samples are a different quantity from returned evaluations.
        self.assertNotEqual(result["point_count"], 500)

    def test_filtered_copy_cannot_reuse_original_result_metadata(self):
        data = self.returned_payload()
        data["data"]["result"] = data["data"]["result"][:1]
        with self.assertRaisesRegex(ValueError, "returned_rows differs"):
            module.summarize(json.dumps(data).encode(), "By")
        data["argus_meta"]["returned_rows"] = 1
        with self.assertRaisesRegex(ValueError, "returned_points differs"):
            module.summarize(json.dumps(data).encode(), "By")

    def test_changed_copy_without_metadata_still_fails_the_sealed_hash(self):
        data = self.returned_payload()
        original_hash = hashlib.sha256(json.dumps(data).encode()).hexdigest()
        data["data"]["result"] = data["data"]["result"][:1]
        del data["argus_meta"]
        with self.assertRaisesRegex(ValueError, "input SHA256 differs"):
            module.summarize(json.dumps(data).encode(), "By", expected_sha256=original_hash)

    def test_changed_returned_time_is_rejected(self):
        for value in ("1970-01-01T00:00:03Z", "1970-01-01T00:00:02", None):
            with self.subTest(value=value):
                data = self.returned_payload()
                data["argus_meta"]["returned_points"]["to"] = value
                with self.assertRaises(ValueError):
                    module.summarize(json.dumps(data).encode(), "By")

    def test_malformed_metadata_and_hash_are_not_treated_as_absence(self):
        for key, value in (("returned_rows", True), ("returned_rows", "2"), ("returned_points", None), ("returned_points", {"count": True})):
            with self.subTest(key=key, value=value):
                data = self.returned_payload()
                data["argus_meta"][key] = value
                with self.assertRaises(ValueError):
                    module.summarize(json.dumps(data).encode(), "By")
        for digest in ("", "0" * 63, "x" * 64):
            with self.subTest(digest=digest):
                with self.assertRaises(ValueError):
                    module.summarize(json.dumps(self.returned_payload()).encode(), "By", expected_sha256=digest)

    def test_unattested_legacy_input_does_not_claim_hash_verification(self):
        result = module.summarize(json.dumps(self.payload([[1, "2"]])).encode(), "By")
        self.assertFalse(result["expected_sha256_verified"])
        self.assertEqual(result["checked_returned_metadata"], [])


class EndpointEvidenceTests(unittest.TestCase):
    def evidence(self, points, unit='1', transform='percent', state='used'):
        raw = json.dumps({'status': 'success', 'data': {'resultType': 'matrix', 'result': [{'metric': {'__name__': 'metric', 'state': state}, 'values': points}]}}).encode()
        return module.summarize(raw, unit, transform)

    def test_endpoint_values_are_paired_by_time_not_extrema_or_input_order(self):
        result = self.evidence([[30, '0.4'], [10, '0.8'], [20, '0.1']])
        row = result['series'][0]
        self.assertEqual((row['first_point_at'], row['first_point_value'], row['display_first_point_value']), ('1970-01-01T00:00:10.000Z', 0.8, 80))
        self.assertEqual((row['last_point_at'], row['last_point_value'], row['display_last_point_value']), ('1970-01-01T00:00:30.000Z', 0.4, 40))
        self.assertEqual((row['value_min'], row['value_max']), (0.1, 0.8))

    def test_inversion_preserves_endpoint_and_range_directions(self):
        row = self.evidence([[1, '0.9'], [2, '0.2']], transform='invert_percent', state='idle')['series'][0]
        self.assertAlmostEqual(row['display_first_point_value'], 10)
        self.assertAlmostEqual(row['display_last_point_value'], 80)
        self.assertAlmostEqual(row['display_min'], 10)
        self.assertAlmostEqual(row['display_max'], 80)

    def test_byte_series_and_markdown_share_program_values_and_formula(self):
        result = self.evidence([[1, str(2**30)], [2, str(3 * 2**30)]], 'By', 'bytes_to_gib')
        row = result['series'][0]
        self.assertEqual((row['display_min'], row['display_max'], row['display_value_mean']), (1, 3, 2))
        text = module.render_markdown(result)
        self.assertIn('display = raw / 2**30', text)
        self.assertIn('| First UTC | First raw | First display |', text)
        self.assertIn('| 1.0 | 3.0 | 2.0 |', text)

    def test_duplicate_timestamps_do_not_create_ambiguous_endpoint_claims(self):
        with self.assertRaisesRegex(ValueError, 'duplicate timestamps'):
            self.evidence([[1, '0.1'], [1, '0.8']])

    def test_catalog_type_preserves_cumulative_semantics_without_inventing_rates(self):
        data = {'status': 'success', 'argus_meta': {'input_metrics_origin': 'authorized_series_metadata', 'input_metrics_complete': True, 'result_is_original_metric': True, 'input_metrics': [{'name': 'network', 'type': 'counter', 'storage_type': 'sum', 'temporality': 'cumulative', 'monotonic': True, 'unit': 'By', 'source_key': 'source:2'}]}, 'data': {'resultType': 'matrix', 'result': [{'metric': {'__name__': 'network'}, 'values': [[1, '258999299'], [61, '259031615']]}]}}
        raw = json.dumps(data).encode()
        result = module.summarize(raw, 'By', metric_type='counter')
        self.assertEqual(result['metric_type'], 'counter')
        self.assertEqual(result['value_min'], 258999299)
        self.assertNotIn('rate', result)
        self.assertNotIn('increase', result)
        self.assertIn('cumulative readings', module.render_markdown(result))
        self.assertTrue(any('traffic share' in value for value in result['limitations']))
        del data['argus_meta']
        unknown = module.summarize(json.dumps(data).encode(), 'By')
        self.assertEqual(unknown['metric_type'], 'unknown')
        self.assertEqual(unknown['metric_type_origin'], 'unavailable')

    def test_declared_histogram_requires_its_own_analysis(self):
        raw = json.dumps({'status': 'success', 'data': {'resultType': 'matrix', 'result': [{'metric': {'__name__': 'histogram'}, 'values': [[1, '5']]}]}}).encode()
        with self.assertRaisesRegex(ValueError, 'histogram-aware'):
            module.summarize(raw, 's', metric_type='histogram')

    def test_untrusted_labels_cannot_close_fence_add_rows_or_inject_html(self):
        result = self.evidence([[1, '0.1']])
        result['series'][0]['labels']['state'] = '<img src=x>|\n```\r'
        text = module.render_markdown(result)
        self.assertNotIn('<img', text.split('```json')[0])
        self.assertEqual(text.count('```'), 2)
        self.assertIn('&lt;img', text)
        self.assertIn('\\|', text)



if __name__ == "__main__":
    unittest.main()
