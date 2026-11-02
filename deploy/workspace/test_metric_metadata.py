import copy
import json
import unittest

import metric_evidence


class SourceMetricMetadataTest(unittest.TestCase):
    def payload(self):
        return {"status": "success", "query_context": {"resources": [{"type": "host", "id": "host"}], "source_keys": ["source:2"]},
            "argus_meta": {"input_metrics_origin": "authorized_series_metadata", "input_metrics_complete": True, "result_is_original_metric": True,
                "input_metrics": [{"name": "network_bytes", "type": "counter", "storage_type": "sum", "temporality": "cumulative", "monotonic": True, "unit": "By", "source_key": "source:2"}]},
            "data": {"resultType": "matrix", "result": [{"metric": {"__name__": "network_bytes", "argus_resource_id": "host"}, "values": [[1, "258999299"], [61, "259031615"]]}]}}

    def test_original_metadata_supplies_type_unit_and_rejects_wrong_model_hint(self):
        raw = json.dumps(self.payload()).encode()
        result = metric_evidence.summarize(raw)
        self.assertEqual(result["unit"], "By")
        self.assertEqual(result["unit_origin"], "query_metadata")
        self.assertEqual(result["metric_type"], "counter")
        self.assertEqual(result["metric_type_origin"], "query_metadata")
        self.assertEqual(result["display_transform"], "bytes_to_gib")
        self.assertEqual(result["interpretation_limits"]["interval_activity"], "not_established_by_this_input")
        self.assertIn("activity level and steadiness remain unknown", metric_evidence.render_markdown(result))
        self.assertIn("区间活动强弱与稳定性仍未知", metric_evidence.render_markdown(result, locale="zh-CN"))
        self.assertTrue(any("authorized series metadata" in item for item in result["limitations"]))
        self.assertFalse(any("supplied from the metric catalog" in item for item in result["limitations"]))
        self.assertTrue(any("returned observation range" in item for item in result["limitations"]))
        with self.assertRaisesRegex(ValueError, "type hint conflicts"):
            metric_evidence.summarize(raw, "By", metric_type="gauge")
        with self.assertRaisesRegex(ValueError, "unit hint conflicts"):
            metric_evidence.summarize(raw, "1", metric_type="counter")

    def test_source_bound_ratio_definition_enables_auto_percent_without_guessing_unit_one(self):
        data = self.payload()
        data["data"]["result"][0].update(metric={"__name__": "system_memory_utilization", "argus_resource_id": "host"}, values=[[1, "0.7"], [61, "0.8"]])
        descriptor = data["argus_meta"]["input_metrics"][0]
        descriptor.update(name="system_memory_utilization", type="gauge", unit="1", storage_type="gauge", monotonic=False)
        raw = json.dumps(data).encode()
        self.assertEqual(metric_evidence.summarize(raw)["display_transform"], "identity")
        meaning = {"name": descriptor["name"], "type": "gauge", "unit": "1", "source_key": "source:2", "semantics": {"quantity_kind": "ratio", "recommended_display_transform": "percent", "definition_id": "host_metrics/system_memory_utilization", "definition_version": "argus.builtin_metric_semantics/v1", "origin": "builtin_collection_definition"}}
        data["argus_meta"]["input_metric_semantics"] = [meaning]
        result = metric_evidence.summarize(json.dumps(data).encode())
        self.assertEqual(result["unit"], "1")
        self.assertEqual(result["display_min"], 70)
        self.assertEqual(result["display_transform"], "percent")
        self.assertEqual(result["quantity_kind"], "ratio")
        self.assertEqual(metric_evidence.summarize(json.dumps(data).encode(), transform="identity")["display_transform"], "identity")
        meaning["source_key"] = "foreign:2"
        with self.assertRaisesRegex(ValueError, "quantity definition conflicts"):
            metric_evidence.summarize(json.dumps(data).encode())

    def test_conflicts_incompleteness_and_foreign_source_are_not_first_item_choices(self):
        for mutation in (
            lambda data: data["argus_meta"].update(input_metrics_complete=False),
            lambda data: data["argus_meta"].update(result_is_original_metric=False),
            lambda data: data["argus_meta"]["input_metrics"][0].update(source_key="other:2"),
            lambda data: data["argus_meta"]["input_metrics"].append({**data["argus_meta"]["input_metrics"][0], "unit": "s"}),
            lambda data: data["argus_meta"]["input_metrics"].append({**data["argus_meta"]["input_metrics"][0], "type": "gauge", "storage_type": "gauge"}),
        ):
            data = copy.deepcopy(self.payload())
            mutation(data)
            with self.assertRaises(ValueError):
                metric_evidence.summarize(json.dumps(data).encode(), "By")

    def test_old_input_does_not_certify_caller_type_and_derived_data_has_no_source_unit(self):
        data = self.payload()
        del data["argus_meta"]
        result = metric_evidence.summarize(json.dumps(data).encode(), "By", metric_type="gauge")
        self.assertEqual(result["metric_type"], "unknown")
        self.assertEqual(result["metric_type_origin"], "unavailable")
        self.assertEqual(result["metric_type_hint"], "gauge")
        self.assertEqual(result["unit_origin"], "provided_argument_unverified")
        self.assertTrue(any("supplied as an argument" in item for item in result["limitations"]))
        self.assertTrue(any("caller hint does not certify" in item for item in result["limitations"]))
        self.assertNotIn("catalog type was not supplied", metric_evidence.render_markdown(result))
        with self.assertRaisesRegex(ValueError, "explicit catalog unit"):
            metric_evidence.summarize(json.dumps(data).encode())
        derived = self.payload()
        del derived["data"]["result"][0]["metric"]["__name__"]
        with self.assertRaisesRegex(ValueError, "transformed or mixed"):
            metric_evidence.summarize(json.dumps(derived).encode())


if __name__ == "__main__":
    unittest.main()
