import json
import unittest
from evaluation_window import compare, render_markdown
import metric_evidence


class EvaluationWindowTest(unittest.TestCase):
    def test_returned_endpoints_are_later_and_only_span_two_minutes(self):
        value = compare("2026-11-02T05:34:23Z", "2026-11-02T05:39:23Z", "2026-11-02T05:37:23.000Z", "2026-11-02T05:39:23.000Z")
        self.assertEqual(value["requested_duration_seconds"], 300)
        self.assertEqual(value["returned_span_seconds"], 120)
        self.assertEqual(value["first_offset_seconds"], 180)
        self.assertEqual(value["first_vs_requested_from"], "after")
        self.assertEqual(value["last_vs_requested_to"], "equal")
        self.assertEqual(value["raw_sampling_coverage"], "not_established")
        for locale, expected in [("en-US", "after"), ("zh-CN", "晚于")]:
            report = render_markdown(value, locale)
            self.assertIn(expected, report)
            self.assertIn("| 180 |", report)

    def test_timezone_offsets_and_nanoseconds_are_compared_exactly(self):
        value = compare("2026-11-02T13:34:23.000000001+08:00", "2026-11-02T05:39:23.000000001Z", "2026-11-02T05:34:23.000000002Z", "2026-11-02T05:39:23Z")
        self.assertEqual(value["requested_duration_seconds"], 300)
        self.assertEqual(value["first_offset_seconds"], 0.000000001)
        self.assertEqual(value["last_offset_seconds"], -0.000000001)
        self.assertEqual(value["last_vs_requested_to"], "before")

    def test_single_point_span_is_zero_without_claiming_zero_data(self):
        value = compare("2026-11-02T05:34:23Z", "2026-11-02T05:39:23Z", "2026-11-02T05:39:23Z", "2026-11-02T05:39:23Z")
        self.assertEqual(value["returned_span_seconds"], 0)
        self.assertEqual(value["first_offset_seconds"], 300)

    def test_missing_bounds_and_no_points_remain_unknown(self):
        value = compare(None, None, "2026-11-02T05:39:23Z", "2026-11-02T05:39:23Z")
        self.assertIsNone(value["requested_duration_seconds"])
        self.assertIsNone(value["first_offset_seconds"])
        empty = compare("2026-11-02T05:34:23Z", "2026-11-02T05:39:23Z")
        self.assertEqual(empty["requested_duration_seconds"], 300)
        self.assertIsNone(empty["returned_span_seconds"])
        self.assertEqual(empty["returned_bounds"], "unavailable")

    def test_points_before_or_after_bounds_are_not_silently_relabelled(self):
        value = compare("2026-11-02T05:34:23Z", "2026-11-02T05:39:23Z", "2026-11-02T05:34:22.5Z", "2026-11-02T05:39:24Z")
        self.assertEqual(value["first_vs_requested_from"], "before")
        self.assertEqual(value["first_offset_seconds"], -0.5)
        self.assertEqual(value["last_vs_requested_to"], "after")

    def test_invalid_or_naive_timestamps_are_not_interpreted_in_host_timezone(self):
        for start, end, first, last in [
            ("2026-11-02T05:34:23", None, None, None),
            ("2026-11-02T05:39:23Z", "2026-11-02T05:34:23Z", None, None),
            (None, None, "2026-11-02T05:39:23Z", None),
            (None, None, "2026-11-02T05:39:23Z", "2026-11-02T05:34:23Z"),
        ]:
            with self.subTest(start=start, first=first):
                with self.assertRaises(ValueError):
                    compare(start, end, first, last)

    def test_metric_time_facts_use_the_full_returned_population(self):
        value = {"status": "success", "query_context": {"from": "2026-11-02T05:34:23Z", "to": "2026-11-02T05:39:23Z"},
                 "data": {"resultType": "matrix", "result": [
                     {"metric": {"__name__": "fixture"}, "values": [[1793597903, "2"]]},
                     {"metric": {"__name__": "fixture"}, "values": [[1793597843, "1"], [1793597963, "3"]]},
                 ]}}
        evidence = metric_evidence.summarize(json.dumps(value).encode(), "1")
        self.assertEqual(evidence["point_count"], 3)
        self.assertEqual(evidence["evaluation_window"]["first_offset_seconds"], 180)
        self.assertEqual(evidence["evaluation_window"]["returned_span_seconds"], 120)
        self.assertIn("| 180 |", metric_evidence.render_markdown(evidence))


if __name__ == "__main__":
    unittest.main()
