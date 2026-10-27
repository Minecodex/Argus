"""Read-only telemetry preservation capture/verify; never grants release approval.

The existing writer must be fully stopped for capture. This tool never changes
replicas, records, credentials or retention. Passwords remain in memory/stdin.
"""
import argparse
import base64
import datetime
import gzip
import html
import json
import pathlib
import re
import subprocess
import threading

from preservation_core import ENCODING, column_type, compare_archives, file_digest, hash_expression, identifier, require_encoding, sorted_hashes

TABLE = re.compile(r"(?:metric_series|metric_samples|logs|traces|trace_summary|trace_span_edges)_[a-f0-9]{32}\Z")
BOUNDS = " SETTINGS max_threads=1,max_final_threads=1,max_block_size=128,preferred_block_size_bytes=262144,max_memory_usage=536870912,max_bytes_before_external_sort=8388608,max_execution_time=180"


class Cluster:
    def __init__(self, args):
        self.prefix = ["kubectl", "--context", args.context, "-n", args.namespace, "--request-timeout=30s"]
        self.namespace = self.json(["get", "namespace", args.namespace, "-o", "json"])
        pods = self.json(["get", "pods", "-l", args.clickhouse_selector, "-o", "json"])["items"]
        pods = [p for p in pods if not p["metadata"].get("deletionTimestamp") and p["status"].get("phase") == "Running"]
        if len(pods) != 1 or not all(s.get("ready") for s in pods[0]["status"].get("containerStatuses", [])):
            raise ValueError("exactly one ready ClickHouse Pod required")
        self.pod = pods[0]["metadata"]["name"]
        secret = self.json(["get", "secret", args.secret, "-o", "json"])
        password = base64.b64decode(secret["data"]["migration-password"], validate=True).decode()
        self.private_config = ("<config><host>127.0.0.1</host><port>9000</port><user>argus_telemetry_migration</user><password>" + html.escape(password) + "</password></config>").encode()

    def json(self, args):
        result = subprocess.run(self.prefix + args, capture_output=True, timeout=45)
        if result.returncode: raise ValueError("cluster metadata operation failed; private output suppressed")
        return json.loads(result.stdout)

    def query_lines(self, sql):
        command = self.prefix + ["exec", "-i", self.pod, "--", "clickhouse-client", "--config-file=/dev/stdin", "--query", sql]
        process = subprocess.Popen(command, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        # Drain stderr without publishing potentially private database details.
        errors = bytearray()
        def drain():
            for chunk in iter(lambda: process.stderr.read(4096), b""):
                if len(errors) < 4096: errors.extend(chunk[:4096-len(errors)])
        reader = threading.Thread(target=drain, daemon=True); reader.start()
        timeout = threading.Timer(210, process.kill); timeout.daemon = True; timeout.start()
        try:
            process.stdin.write(self.private_config); process.stdin.close()
            for line in process.stdout: yield line.decode("utf-8").strip()
            code = process.wait(); reader.join(timeout=5)
            if code:
                codes = re.findall(r"Code:\s*(\d+)", errors.decode(errors="replace"))
                raise ValueError("bounded query failed; exception codes=" + ",".join(codes))
        finally:
            timeout.cancel()
            if process.poll() is None: process.kill(); process.wait()
            reader.join(timeout=5)
            process.stdout.close(); process.stderr.close()

    def rows(self, sql):
        return [json.loads(x) for x in self.query_lines(sql + " FORMAT JSONEachRow") if x]

    def require_writer_paused(self, name):
        deployment = self.json(["get", "deployment", name, "-o", "json"])
        spec, status = deployment["spec"], deployment.get("status", {})
        if spec.get("replicas", 1) != 0 or status.get("replicas", 0) != 0 or status.get("observedGeneration", 0) < deployment["metadata"]["generation"]:
            raise ValueError("capture requires the writer actually paused; this tool never scales it")
        selector = spec["selector"]
        if selector.get("matchExpressions") or not selector.get("matchLabels"):
            raise ValueError("unsupported writer selector")
        labels = ",".join(k + "=" + v for k, v in selector["matchLabels"].items())
        pods = self.json(["get", "pods", "-l", labels, "-o", "json"])["items"]
        if any(p["status"].get("phase") not in ("Failed", "Succeeded") for p in pods):
            raise ValueError("writer Pods have not finished stopping")


def archived_path(baseline_path, record):
    root = baseline_path.parent.resolve()
    path = (root / record["archive"]).resolve(strict=True)
    if not path.is_relative_to(root) or not path.is_file() or file_digest(path) != record["archive_sha256"]:
        raise ValueError("baseline archive escaped its directory or changed")
    return path


def run(args):
    baseline = json.loads(args.baseline.read_text(encoding="utf-8-sig")) if args.baseline else None
    if args.mode == "verify":
        if not baseline: raise ValueError("verify requires a baseline")
        require_encoding(baseline)
        if baseline.get("mode") != "capture" or baseline.get("passed") is not True:
            raise ValueError("only a completed capture can be verified")
        for record in baseline["tables"].values(): archived_path(args.baseline, record)
    elif baseline:
        raise ValueError("capture cannot take a baseline")
    if args.output.exists(): raise ValueError("preserve previous evidence; use a fresh output directory")
    identifier(args.database)
    cluster = Cluster(args)
    target = {"context": args.context, "namespace": args.namespace, "namespace_uid": cluster.namespace["metadata"]["uid"], "database": args.database}
    if baseline and baseline["target"] != target: raise ValueError("baseline target identity mismatch")
    if args.mode == "capture": cluster.require_writer_paused(args.writer_deployment)
    inventory = cluster.rows("SELECT name,toString(uuid) AS uuid,engine FROM system.tables WHERE database='" + args.database + "'")
    tables = {t["name"]: t for t in inventory if TABLE.fullmatch(t["name"])}
    names = sorted(baseline["tables"] if baseline else tables)
    if not names: raise ValueError("no managed telemetry tables found")
    if baseline:
        cutoff = baseline["cutoff_unix_nano"]
    else:
        observed_cutoff = list(cluster.query_lines("SELECT toString(toUnixTimestamp64Nano(now64(9,'UTC'))) FORMAT TabSeparated"))
        if len(observed_cutoff) != 1: raise ValueError("capture clock query incomplete")
        cutoff = observed_cutoff[0]
    if not re.fullmatch(r"\d{18,20}", cutoff): raise ValueError("invalid capture cutoff")
    args.output.mkdir(parents=True)
    hashes = args.output / "hashes"; hashes.mkdir()
    result = {"schema_version": "argus.telemetry_preservation/v2", "logical_hash_encoding": ENCODING, "mode": args.mode, "target": target, "cutoff_unix_nano": cutoff,
              "observed_at": datetime.datetime.now(datetime.timezone.utc).isoformat(), "tables": {}, "passed": False,
              "scope": "logical row preservation only; not a disaster recovery backup or release approval", "observer_sha256": file_digest(pathlib.Path(__file__)), "core_sha256": file_digest(pathlib.Path(__file__).with_name("preservation_core.py"))}
    if baseline: result["baseline_sha256"] = file_digest(args.baseline)
    manifest = args.output / "manifest.json"
    def save(): manifest.write_text(json.dumps(result, indent=2) + "\n", encoding="utf-8")
    save()
    for name in names:
        if name not in tables: raise ValueError("original table missing")
        prior = baseline["tables"][name] if baseline else None
        if prior and prior["table_uuid"] != tables[name]["uuid"]: raise ValueError("original table identity changed")
        current = cluster.rows("SELECT name,type FROM system.columns WHERE database='" + args.database + "' AND table='" + name + "' ORDER BY position")
        original = prior["columns"] if prior else current
        if not prior and name.startswith("metric_series_"):
            original = [c for c in original if c["name"] not in {"kafka_topic", "kafka_partition", "kafka_offset", "record_sequence", "expires_at"}]
        actual = {c["name"]: column_type(c["type"]) for c in current}
        if any(actual.get(c["name"]) != column_type(c["type"]) for c in original): raise ValueError("original column or type changed; explicit migration review required")
        expression = hash_expression(original)
        untimed = name.startswith(("metric_series_", "trace_span_edges_"))
        timestamp = next((c["name"] for c in original if c["name"] in {"timestamp", "start_time", "start_timestamp", "root_start_time"}), None)
        if not untimed and timestamp is None: raise ValueError("unreviewed original table time boundary")
        where = "" if untimed else " WHERE toUnixTimestamp64Nano(CAST(" + identifier(timestamp) + " AS DateTime64(9,'UTC'))) < " + cutoff
        sql = "SELECT " + expression + " AS row_sha FROM " + identifier(args.database) + "." + identifier(name) + " FINAL" + where + " ORDER BY row_sha" + BOUNDS + " FORMAT TabSeparated"
        archive = hashes / (name + ".sha256.gz"); count = 0
        with gzip.open(archive, "xt", encoding="ascii", newline="\n") as stream:
            for digest in sorted_hashes(cluster.query_lines(sql)): stream.write(digest + "\n"); count += 1
        record = {"table_uuid": tables[name]["uuid"], "columns": original, "archive": archive.relative_to(args.output).as_posix(), "archive_sha256": file_digest(archive), "rows": count, "passed": True}
        if prior:
            comparison = compare_archives(archived_path(args.baseline, prior), archive)
            if comparison["original_rows"] != prior["rows"]: raise ValueError("baseline row count inconsistent")
            record.update(comparison=comparison, passed=comparison["passed"])
        result["tables"][name] = record; save()
    if args.mode == "capture": cluster.require_writer_paused(args.writer_deployment)
    result.update(passed=all(t["passed"] for t in result["tables"].values()), all_tables_examined=True, finished_at=datetime.datetime.now(datetime.timezone.utc).isoformat()); save()
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("mode", choices=["capture", "verify"])
    parser.add_argument("--context", required=True)
    parser.add_argument("--namespace", required=True)
    parser.add_argument("--database", default="argus_telemetry")
    parser.add_argument("--secret", default="argus-telemetry-clickhouse")
    parser.add_argument("--clickhouse-selector", default="clickhouse.altinity.com/chi=argus-clickhouse")
    parser.add_argument("--writer-deployment", default="argus-telemetry-writer")
    parser.add_argument("--output", type=pathlib.Path, required=True)
    parser.add_argument("--baseline", type=pathlib.Path)
    args = parser.parse_args()
    try:
        result = run(args)
        print(json.dumps({"passed": result["passed"], "tables": len(result["tables"]), "manifest": str(args.output / "manifest.json")}))
        return 0 if result["passed"] else 1
    except (ValueError, OSError, KeyError, subprocess.TimeoutExpired) as error:
        print(json.dumps({"passed": False, "error": str(error)}))
        return 1


if __name__ == "__main__": raise SystemExit(main())
