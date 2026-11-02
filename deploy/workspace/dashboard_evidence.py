#!/usr/bin/env python3
"""Compute panel facts from an authorized sealed dashboard manifest and files."""
import argparse
import hashlib
import json
import re
import sys
from datetime import datetime
from pathlib import Path, PurePosixPath
from uuid import UUID

from metric_evidence import MAX_BYTES, MAX_POINTS, markdown_cell, summarize_rows
from evidence_text import LOCALES, table_header, translator
import evaluation_window

MAX_TOTAL_BYTES = 64 * 1024 * 1024
MAX_FILES = 512


def read_bounded(path):
    with path.open("rb") as stream:
        raw = stream.read(MAX_BYTES + 1)
    if len(raw) > MAX_BYTES:
        raise ValueError("evidence file exceeds 16 MiB")
    return raw


def utc(value):
    parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    if parsed.tzinfo is None:
        raise ValueError("manifest time requires a timezone")
    return parsed


def identifier(value):
    return str(UUID(value))


def summarize_manifest(path, expected_sha256=None):
    path = Path(path).resolve(strict=True)
    raw = read_bounded(path)
    digest = hashlib.sha256(raw).hexdigest()
    if expected_sha256 is not None and (not re.fullmatch(r"[0-9a-fA-F]{64}", expected_sha256) or expected_sha256.lower() != digest):
        raise ValueError("manifest differs from its sealed SHA256")
    manifest = json.loads(raw)
    if manifest.get("schema_version") != "argus.dashboard_query_manifest/v1":
        raise ValueError("unsupported dashboard manifest")
    job, attempt = identifier(manifest["job_id"]), identifier(manifest["attempt_id"])
    execution = manifest["execution"]
    if (execution.get("job_id") and execution["job_id"] != job) or (execution.get("attempt_id") and execution["attempt_id"] != attempt):
        raise ValueError("manifest execution belongs to another job or attempt")
    start, end = utc(execution["from"]), utc(execution["to"])
    if not start < end:
        raise ValueError("invalid frozen dashboard window")
    resources = execution.get("resources", [])
    if not isinstance(resources, list):
        raise ValueError("invalid manifest resource scope")
    allowed_ids = {ref["id"] for ref in resources}
    definitions = {panel["id"]: panel for panel in manifest["definition"]["panels"]}
    targets = {(panel["id"], target["id"]): target for panel in execution["panels"] for target in panel.get("targets", [])}
    files = manifest.get("files", [])
    if not isinstance(files, list) or len(files) > MAX_FILES:
        raise ValueError("dashboard file count exceeds limit")
    seen, verified, inputs = set(), [], {path}
    total_bytes, total_points = len(raw), 0
    for file in files:
        if file.get("kind") != "data":
            continue
        pair = (file["panel_id"], file["target_id"])
        if pair not in targets or pair[0] not in definitions or pair in seen:
            raise ValueError("duplicate or unknown panel target file")
        seen.add(pair)
        file_id = identifier(file["id"])
        declared = PurePosixPath(file["path"])
        expected_path = PurePosixPath("/workspace/queries") / job / attempt / (file_id + ".json")
        if declared != expected_path or file.get("source_ref") != "dashboard-query:" + job + "/" + attempt:
            raise ValueError("panel file does not belong to this manifest job/attempt")
        local = (path.parent / declared.name).resolve(strict=True)
        if local.parent != path.parent or local == path:
            raise ValueError("panel path escaped the manifest directory")
        inputs.add(local)
        body = read_bounded(local)
        total_bytes += len(body)
        if total_bytes > MAX_TOTAL_BYTES:
            raise ValueError("dashboard evidence exceeds 64 MiB")
        if type(file.get("bytes")) is not int or len(body) != file["bytes"] or hashlib.sha256(body).hexdigest() != file.get("sha256"):
            raise ValueError("panel file differs from sealed size or SHA256")
        target, definition = targets[pair], definitions[pair[0]]
        meta = target.get("meta") or {}
        record = {"panel_id": pair[0], "target_id": pair[1], "file_id": file_id, "input_sha256": file["sha256"], "bytes": len(body), "unit": definition.get("unit"), "unit_origin": "published_panel_output", "query_hash": target.get("query_hash"), "partial": bool(meta.get("partial"))}
        kind = target.get("result_type")
        if target.get("status") != "success" or kind not in ("matrix", "vector"):
            record.update(status="not_numeric", result_type=kind, reason="Use the signal-specific original file; numeric panel statistics unavailable.")
        else:
            rows = json.loads(body)
            if not isinstance(rows, list):
                raise ValueError("numeric panel file must contain original series")
            for row in rows:
                resource = row.get("metric", {}).get("argus_resource_id")
                if resource and resource not in allowed_ids:
                    raise ValueError("panel data contains a resource outside the frozen scope")
            if not rows:
                record.update(status="no_data", point_count=0, reason="No returned numeric series; not proof of complete raw coverage.")
            elif any(row.get("histogram") or row.get("histograms") for row in rows):
                record.update(status="not_numeric", reason="Native histograms require histogram-aware analysis.")
            else:
                facts = summarize_rows(rows, kind, definition.get("unit"), "identity", meta)
                if utc(facts["first_point_at"]) < start or utc(facts["last_point_at"]) > end:
                    raise ValueError("returned evaluation points escaped the frozen window")
                total_points += facts["point_count"]
                if total_points > MAX_POINTS:
                    raise ValueError("dashboard point count exceeds limit")
                record.update(status="numeric", **facts)
                if len(facts["series"]) == 1:
                    only = facts["series"][0]
                    record.update(first_point_value=only["first_point_value"], last_point_value=only["last_point_value"], value_mean=only["value_mean"])
        verified.append(record)
    for pair, target in targets.items():
        if pair not in seen:
            verified.append({"panel_id": pair[0], "target_id": pair[1], "status": "unavailable", "result_type": target.get("result_type"), "reason": "No sealed data file for this target."})
    for panel in verified:
        panel["evaluation_window"] = evaluation_window.compare(execution["from"], execution["to"], panel.get("first_point_at"), panel.get("last_point_at"))
    result = {"schema_version": "argus.dashboard_evidence/v2", "manifest_sha256": digest, "job_id": job, "attempt_id": attempt, "dashboard_id": execution.get("dashboard_id"), "revision_id": execution.get("revision_id"), "resource_refs": resources, "requested_from": execution["from"], "requested_to": execution["to"], "complete": bool(manifest.get("complete")) and bool(targets) and all(p["status"] in ("numeric", "no_data") for p in verified), "partial": bool(execution.get("partial")) or any(p.get("partial") for p in verified), "panels": verified, "limitations": ["Evaluation timestamps do not establish continuous raw sampling coverage.", "Panel units apply to published expression outputs; source metric units may differ.", "Statistics cover all returned series; do not replace them with a filtered copy."]}
    result["manifest_checksum_status"] = "verified" if expected_sha256 is not None else "not_requested"
    result["manifest_checksum_explanation"] = ("Matched the supplied expected manifest SHA256." if expected_sha256 is not None else "No expected manifest SHA256 was supplied. This is not a checksum mismatch; panel files were separately checked against their sealed hashes.")
    return result, inputs


def render_markdown(evidence, include_json=True, locale="en-US"):
    t = translator(locale)
    table = table_header(t, ("Panel / target", "State", "Points", "First UTC", "Last UTC", "Minimum", "Maximum", "Output unit"))
    for panel in evidence["panels"]:
        cells = [panel["panel_id"] + "/" + panel["target_id"], t(panel["status"])] + [panel.get(key, t("unavailable")) for key in ("point_count", "first_point_at", "last_point_at", "value_min", "value_max", "unit")]
        table += "| " + " | ".join(markdown_cell(cell) for cell in cells) + " |\n"
    endpoints = table_header(t, ("Panel / target", "Series", "First UTC", "First value", "Last UTC", "Last value", "Output unit"))
    has_endpoints = False
    for panel in evidence["panels"]:
        for index, series in enumerate(panel.get("series", []), start=1):
            has_endpoints = True
            label = str(index) + " " + json.dumps(series.get("labels", {}), ensure_ascii=False, sort_keys=True)
            cells = [panel["panel_id"] + "/" + panel["target_id"], label, series["first_point_at"], series["first_point_value"], series["last_point_at"], series["last_point_value"], panel["unit"]]
            endpoints += "| " + " | ".join(markdown_cell(cell) for cell in cells) + " |\n"
    if has_endpoints:
        table += "\n" + t("Endpoint values retain their actual timestamps; a minimum or maximum is not automatically an endpoint.") + "\n\n" + endpoints
    for panel in evidence["panels"]:
        if "evaluation_window" in panel:
            table += "\n### " + markdown_cell(panel["panel_id"] + "/" + panel["target_id"]) + "\n\n"
            table += evaluation_window.render_markdown(panel["evaluation_window"], locale)
    encoded = json.dumps(evidence, ensure_ascii=False, indent=2, allow_nan=False)
    status = evidence.get("manifest_checksum_status", "unknown")
    checksum = t("Manifest checksum verification") + ": " + t(status) + ". " + t(evidence.get("manifest_checksum_explanation", ""))
    return "# " + t("Dashboard panel evidence") + "\n\n" + t("Statistics cover returned values from the published queries; units apply to those expression outputs.") + "\n\n" + checksum + "\n\n" + table + ("\n```json\n" + encoded.replace("`", "\\u0060") + "\n```\n" if include_json else "")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("manifest", type=Path)
    parser.add_argument("--expected-sha256")
    parser.add_argument("--json-out", type=Path)
    parser.add_argument("--markdown-out", type=Path)
    parser.add_argument("--locale", choices=LOCALES, default="en-US")
    args = parser.parse_args()
    try:
        evidence, inputs = summarize_manifest(args.manifest, args.expected_sha256)
        outputs = [value.resolve() for value in (args.json_out, args.markdown_out) if value is not None]
        if len(set(outputs)) != len(outputs) or any(output in inputs for output in outputs):
            raise ValueError("output paths must be distinct and preserve all original evidence")
        encoded = json.dumps(evidence, ensure_ascii=False, indent=2, allow_nan=False)
        markdown = render_markdown(evidence, locale=args.locale)
        for destination, content in ((args.json_out, encoded + "\n"), (args.markdown_out, markdown)):
            if destination is not None:
                destination.parent.mkdir(parents=True, exist_ok=True)
                destination.write_text(content, encoding="utf-8")
        print(encoded)
    except (OSError, ValueError, KeyError, TypeError, OverflowError) as error:
        print("dashboard evidence unavailable: " + str(error), file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
