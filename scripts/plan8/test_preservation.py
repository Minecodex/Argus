import argparse
import gzip
import json
import os
import pathlib
import tempfile
import unittest
from unittest.mock import patch

import preservation_core as core
import telemetry_preservation as tool


class PreservationTests(unittest.TestCase):
    def test_duplicate_rows_are_not_a_set(self):
        a, b, c = [x * 64 for x in "123"]
        result = core.compare_hashes([a, a, b], [a, b, c])
        self.assertFalse(result["passed"])
        self.assertEqual((result["matched_rows"], result["missing_rows"]), (2, 1))

    def test_appends_allowed_but_middle_and_tail_missing_fail(self):
        a, b, c, d = [x * 64 for x in "1234"]
        self.assertTrue(core.compare_hashes([a, c], [a, b, c, d])["passed"])
        self.assertEqual(core.compare_hashes([a, b, d], [a, c])["missing_rows"], 2)
        self.assertFalse(core.compare_hashes([a], [])["passed"])

    def test_invalid_order_and_bad_hash_in_remaining_tail_fail(self):
        a, b = "a" * 64, "b" * 64
        for original, current in [([b, a], [a]), ([a], [a, "bad"]), ([], [b, a])]:
            with self.assertRaises(ValueError): core.compare_hashes(original, current)

    def test_untrusted_schema_and_nested_maps_rejected(self):
        for name, typ in [("x);SELECT", "String"), ("x", "String) UNION SELECT 1"), ("x", "Array(Map(String,String))")]:
            with self.assertRaises(ValueError): core.hash_expression([{"name": name, "type": typ}])
        with self.assertRaises(ValueError): core.hash_expression([{"name": "x", "type": "String"}] * 2)
        self.assertEqual(core.column_type("Array(Tuple(Float64, Float64))"), "Array(Tuple(Float64,Float64))")

    def test_legacy_encoding_rejected_before_cluster_access(self):
        with tempfile.TemporaryDirectory(prefix="argus-preservation-unit-") as folder:
            root = pathlib.Path(folder); baseline = root / "legacy.json"
            baseline.write_text(json.dumps({"passed": True, "tables": {}}))
            args = argparse.Namespace(mode="verify", baseline=baseline, output=root / "out")
            with patch.object(tool, "Cluster") as cluster:
                with self.assertRaisesRegex(ValueError, "legacy"): tool.run(args)
                cluster.assert_not_called()
            self.assertFalse(args.output.exists())

    def test_archive_hash_and_path_are_verified(self):
        with tempfile.TemporaryDirectory(prefix="argus-preservation-unit-") as folder:
            root = pathlib.Path(folder); archive = root / "rows.gz"
            with gzip.open(archive, "wt") as stream: stream.write("a" * 64 + "\n")
            record = {"archive": "rows.gz", "archive_sha256": core.file_digest(archive)}
            self.assertEqual(tool.archived_path(root / "manifest.json", record), archive)
            record["archive_sha256"] = "0" * 64
            with self.assertRaises(ValueError): tool.archived_path(root / "manifest.json", record)
            record.update(archive="../outside.gz")
            with self.assertRaises((ValueError, FileNotFoundError)): tool.archived_path(root / "manifest.json", record)

    def test_writer_pause_uses_live_state(self):
        cluster = tool.Cluster.__new__(tool.Cluster)
        deployment = {"metadata": {"generation": 3}, "spec": {"replicas": 1, "selector": {"matchLabels": {"app": "writer"}}}, "status": {"replicas": 1, "observedGeneration": 3}}
        cluster.json = lambda args: deployment
        with self.assertRaisesRegex(ValueError, "actually paused"): cluster.require_writer_paused("writer")
        deployment["spec"]["replicas"] = deployment["status"]["replicas"] = 0
        cluster.json = lambda args: {"items": [{"status": {"phase": "Running"}}]} if args[1] == "pods" else deployment
        with self.assertRaisesRegex(ValueError, "not finished"): cluster.require_writer_paused("writer")
        cluster.json = lambda args: {"items": []} if args[1] == "pods" else deployment
        cluster.require_writer_paused("writer")

    def test_manifest_roundtrip_preserves_duplicates_and_examines_all_tables(self):
        table = "metric_series_" + "1" * 32
        other = "metric_series_" + "2" * 32
        class Fixture:
            namespace = {"metadata": {"uid": "fixed-namespace"}}
            pause_checks = 0
            values = ["a" * 64, "a" * 64]
            def require_writer_paused(self, name): self.pause_checks += 1
            def rows(self, sql):
                if "system.tables" in sql:
                    return [{"name": name, "uuid": name, "engine": "ReplacingMergeTree"} for name in [table, other]]
                return [{"name": "id", "type": "UInt64"}]
            def query_lines(self, sql):
                if "now64" in sql: return iter(["1792966828890225939"])
                return iter(self.values if table in sql else [])
        fixture = Fixture()
        with tempfile.TemporaryDirectory(prefix="argus-preservation-unit-") as folder, patch.object(tool, "Cluster", return_value=fixture):
            root = pathlib.Path(folder)
            args = argparse.Namespace(mode="capture", baseline=None, output=root / "capture", database="argus_telemetry", context="test", namespace="test", writer_deployment="writer")
            captured = tool.run(args)
            self.assertTrue(captured["passed"])
            self.assertEqual(fixture.pause_checks, 2)
            args.mode, args.baseline = "verify", args.output / "manifest.json"
            args.output = root / "verify-added"; fixture.values += ["b" * 64]
            self.assertTrue(tool.run(args)["passed"])
            args.output = root / "verify-missing"; fixture.values = ["a" * 64, "b" * 64]
            result = tool.run(args)
            self.assertFalse(result["passed"])
            self.assertTrue(result["all_tables_examined"])
            self.assertEqual(result["tables"][table]["comparison"]["missing_rows"], 1)
            self.assertTrue(result["tables"][other]["passed"])

    def test_manifest_target_mismatch_rejected_before_output(self):
        with tempfile.TemporaryDirectory(prefix="argus-preservation-unit-") as folder:
            root = pathlib.Path(folder); baseline = root / "baseline.json"
            baseline.write_text(json.dumps({"logical_hash_encoding": core.ENCODING, "mode": "capture", "passed": True, "tables": {}, "target": {"namespace_uid": "foreign"}}))
            fake = unittest.mock.Mock(namespace={"metadata": {"uid": "current"}})
            args = argparse.Namespace(mode="verify", baseline=baseline, output=root / "out", database="argus_telemetry", context="test", namespace="test")
            with patch.object(tool, "Cluster", return_value=fake):
                with self.assertRaisesRegex(ValueError, "target identity mismatch"): tool.run(args)
            self.assertFalse(args.output.exists())


@unittest.skipUnless(os.getenv("ARGUS_PLAN8_PRESERVATION_LIVE") == "1", "explicit actual ClickHouse probe required")
class LiveClickHouseTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.cluster = tool.Cluster(argparse.Namespace(context="docker-desktop", namespace="argus-system", clickhouse_selector="clickhouse.altinity.com/chi=argus-clickhouse", secret="argus-telemetry-clickhouse"))
        cls.columns = [{"name": "id", "type": "UInt64"}, {"name": "attrs", "type": "Map(String,String)"}, {"name": "items", "type": "Array(UInt64)"}, {"name": "unit", "type": "String"}]

    def digest(self, attrs="map('b','2','a','1')", items="[toUInt64(1),toUInt64(2)]", unit="'ms'", identity="1", columns=None):
        sql = "SELECT " + core.hash_expression(columns or self.columns) + " FROM (SELECT toUInt64(" + identity + ") AS id," + attrs + " AS attrs," + items + " AS items," + unit + " AS unit) FORMAT TabSeparated"
        return list(self.cluster.query_lines(sql))[0]

    def test_actual_map_order_and_empty_map(self):
        self.assertEqual(self.digest(), self.digest(attrs="map('a','1','b','2')"))
        self.assertEqual(len(self.digest(attrs="CAST(map(), 'Map(String,String)')")), 64)

    def test_actual_values_identity_types_and_array_order_remain_distinct(self):
        original = self.digest()
        changes = [self.digest(attrs="map('a','9','b','2')"), self.digest(unit="'s'"), self.digest(identity="2"), self.digest(items="[toUInt64(2),toUInt64(1)]")]
        self.assertTrue(all(x != original for x in changes))
        # Changed types are rejected by the schema contract before querying;
        # actual value hashing must not collapse any of these changes.
        self.assertFalse(core.compare_hashes([original], [changes[0]])["passed"])

    def test_actual_duplicate_map_keys_rejected(self):
        with self.assertRaisesRegex(ValueError, "bounded query failed"):
            self.digest(attrs="map('a','1','a','2')")


if __name__ == "__main__": unittest.main(verbosity=2)
