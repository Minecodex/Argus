"""Read-only oracle edge cases; no provider requests or platform fixtures."""
import unittest
from closeout_evidence import observation_window, utc_difference


class ObservationTimeTests(unittest.TestCase):
    def test_instant_stat_has_one_instant_not_a_five_minute_span(self):
        start, end = observation_window([[1794916345.004, "8181911552"]])
        self.assertEqual(start, end)

    def test_partial_series_retains_only_its_observed_span(self):
        start, end = observation_window([[1794916293.064, "0.5"], [1794916345.064, "0.5"]])
        self.assertEqual(utc_difference(end, start), 52)

    def test_empty_series_has_no_invented_observation_times(self):
        self.assertEqual(observation_window([]), (None, None))

    def test_timezone_alias_and_millisecond_display(self):
        self.assertEqual(utc_difference("2026-11-17T19:52:25.004+08:00", "2026-11-17T11:52:25.004Z"), 0)
        self.assertLess(utc_difference("2026-11-17T11:52:25.004Z", "2026-11-17T11:52:25.004759081Z"), .001)

    def test_wrong_second_and_missing_timezone_are_rejected(self):
        self.assertGreaterEqual(utc_difference("2026-11-17T11:52:26Z", "2026-11-17T11:52:25Z"), 1)
        with self.assertRaises(AssertionError):
            utc_difference("2026-11-17T11:52:25", "2026-11-17T11:52:25Z")


if __name__ == '__main__':
    unittest.main()
