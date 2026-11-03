#!/usr/bin/env python3
"""Derive reproducible evidence from an already authorized Workspace query file."""
import argparse
import hashlib
import json
import math
from datetime import datetime, timedelta, timezone
from decimal import Decimal, ROUND_HALF_UP
from pathlib import Path
import statistics
import sys

from metric_metadata import resolve as resolve_metric_metadata
from evidence_text import LOCALES, table_header, translator
import evaluation_window

MAX_BYTES = 16 * 1024 * 1024
MAX_POINTS = 1_000_000
METRIC_TYPES = ("unknown", "gauge", "counter", "histogram")


def timestamp(value):
    milliseconds = int((Decimal(str(value)) * 1000).quantize(Decimal(1), rounding=ROUND_HALF_UP))
    return (datetime(1970, 1, 1, tzinfo=timezone.utc) + timedelta(milliseconds=milliseconds)).isoformat(timespec="milliseconds").replace("+00:00", "Z")


def convert(unit, transform, low, high, all_idle):
    if transform == "auto":
        # Dimensionless is not synonymous with a ratio (status/response codes
        # also use unit 1). Percentage conversion requires an explicit choice.
        transform = "bytes_to_gib" if unit == "By" else "identity"
    if transform == "invert_percent" and not 0 <= low <= high <= 1:
        raise ValueError("idle fraction conversion requires observations in 0..1")
    if transform == "bytes_to_gib" and unit == "By":
        return transform, "GiB", low / 2**30, high / 2**30
    if transform == "percent" and unit == "1":
        return transform, "%", low * 100, high * 100
    if transform == "invert_percent" and unit == "1" and all_idle:
        return transform, "%", (1 - high) * 100, (1 - low) * 100
    if transform == "identity":
        return transform, unit, low, high
    raise ValueError("conversion does not match source unit or exclusively idle series")


def returned_metadata_checks(meta, row_count, point_count, first, last):
    if not isinstance(meta, dict):
        raise ValueError("invalid query metadata")
    checked = []
    if "returned_rows" in meta:
        if type(meta["returned_rows"]) is not int or meta["returned_rows"] != row_count:
            raise ValueError("returned_rows differs from the file; use the original query or query the narrower selector")
        checked.append("returned_rows")
    if "returned_points" in meta:
        points = meta["returned_points"]
        if not isinstance(points, dict) or type(points.get("count")) is not int or points["count"] != point_count:
            raise ValueError("returned_points differs from the file; a derived subset is not the original query result")
        for key, expected in (("from", first), ("to", last)):
            text = points.get(key)
            if not isinstance(text, str):
                raise ValueError("returned point timestamps are missing")
            at = datetime.fromisoformat(text.replace("Z", "+00:00"))
            if at.tzinfo is None or at.astimezone(timezone.utc).isoformat(timespec="milliseconds").replace("+00:00", "Z") != expected:
                raise ValueError("returned point timestamps differ from the file")
        checked.append("returned_points")
    return checked


def summarize_rows(rows, kind, unit, transform="identity", meta=None):
    """Shared numerical facts; dashboard expressions need no invented metric name."""
    if kind not in ("matrix", "vector"):
        raise ValueError("this helper requires direct numeric metric series; inspect other result kinds separately")
    if not isinstance(rows, list) or not rows or len(rows) > 10_000:
        raise ValueError("metric series are empty or exceed the evidence limit")
    values, times, series = [], [], []
    all_idle = all(row.get("metric", {}).get("state") == "idle" for row in rows)
    for row in rows:
        if row.get("histogram") or row.get("histograms"):
            raise ValueError("histograms need histogram-aware analysis")
        points = row.get("values", []) if kind == "matrix" else [row.get("value")]
        if not points:
            continue
        row_values, row_times = [], []
        for point in points:
            if not isinstance(point, list) or len(point) != 2:
                raise ValueError("invalid query point")
            number = float(point[1])
            if not math.isfinite(number):
                raise ValueError("non-finite observations need explicit missing/stale-data analysis")
            row_values.append(number)
            row_times.append(Decimal(str(point[0])))
        if len(set(row_times)) != len(row_times):
            raise ValueError("duplicate timestamps in one series require explicit analysis")
        values.extend(row_values)
        times.extend(row_times)
        if len(values) > MAX_POINTS:
            raise ValueError("point count exceeds the evidence limit")
        first_index, last_index = row_times.index(min(row_times)), row_times.index(max(row_times))
        series.append({"labels": row.get("metric", {}), "point_count": len(row_values), "value_min": min(row_values), "value_max": max(row_values), "value_mean": statistics.fmean(row_values), "first_point_at": timestamp(row_times[first_index]), "first_point_value": row_values[first_index], "last_point_at": timestamp(row_times[last_index]), "last_point_value": row_values[last_index]})
    if not values:
        raise ValueError("no numeric points returned")
    low, high = min(values), max(values)
    transform, display_unit, display_low, display_high = convert(unit, transform, low, high, all_idle)
    for row in series:
        _, _, row["display_min"], row["display_max"] = convert(unit, transform, row["value_min"], row["value_max"], all_idle)
        for field in ("first_point_value", "last_point_value", "value_mean"):
            row["display_" + field] = convert(unit, transform, row[field], row[field], all_idle)[2]
    first, last = timestamp(min(times)), timestamp(max(times))
    checked = returned_metadata_checks(meta if meta is not None else {}, len(rows), len(values), first, last)
    return {"checked_returned_metadata": checked, "unit": unit, "point_count": len(values), "point_count_basis": "all_returned_series", "first_point_at": first, "last_point_at": last, "value_min": low, "value_max": high, "display_transform": transform, "display_unit": display_unit, "display_min": display_low, "display_max": display_high, "series_count": len(series), "series": series}


def summarize(raw, unit=None, transform="auto", expected_sha256=None, metric_type="unknown"):
    if metric_type not in METRIC_TYPES:
        raise ValueError("metric type hint must be unknown, gauge, counter or histogram; original query metadata determines the effective type")
    if len(raw) > MAX_BYTES:
        raise ValueError("query file exceeds 16 MiB evidence limit")
    digest = hashlib.sha256(raw).hexdigest()
    if expected_sha256 is not None:
        expected_sha256 = expected_sha256.lower()
        if len(expected_sha256) != 64 or any(c not in "0123456789abcdef" for c in expected_sha256):
            raise ValueError("expected SHA256 must be the sealed result's 64-digit hexadecimal hash")
        if digest != expected_sha256:
            raise ValueError("input SHA256 differs from the supplied expected checksum; use the original query workspace_file.sha256, not a later read/bash result_hash, and verify the original source")
    value = json.loads(raw, parse_float=Decimal)
    if not isinstance(value, dict):
        raise ValueError("expected an original metric query object; dashboard panel arrays require argus-dashboard-evidence MANIFEST or argus-evidence-report ORIGINAL_METRIC --dashboard-manifest MANIFEST")
    if value.get("status") != "success":
        raise ValueError("query did not succeed")
    data = value.get("data", {})
    if not isinstance(data, dict):
        raise ValueError("original metric query data must be a PromQL result object")
    rows = data.get("result", [])
    names = {row.get("metric", {}).get("__name__") for row in rows}
    if len(names) != 1 or None in names or "" in names:
        raise ValueError("source unit cannot be assigned to transformed or mixed metric names")
    context, meta = value.get("query_context", {}), value.get("argus_meta", {})
    semantics = resolve_metric_metadata(meta, context, next(iter(names)), unit, metric_type)
    unit, metric_type = semantics["unit"], semantics["metric_type"]
    if transform == "auto" and semantics.get("quantity_kind") == "ratio":
        transform = semantics["recommended_display_transform"]
    facts = summarize_rows(rows, data.get("resultType"), unit, transform, meta)
    resources = context.get("resources", [])
    label_ids = {row.get("metric", {}).get("argus_resource_id") for row in rows}
    label_ids.discard(None)
    label_ids.discard("")
    resource_id = resources[0].get("id") if len(resources) == 1 else next(iter(label_ids)) if not resources and len(label_ids) == 1 else None
    limitations = ["Returned PromQL evaluation timestamps do not prove continuous raw sampling coverage.", "Statistics cover returned values; labels and series populations must be preserved when interpreting them.", "Requested time bounds do not establish observations throughout that window; describe only the returned observation range."]
    if semantics["unit_origin"] == "query_metadata":
        limitations.append("The source unit is recorded in the original query's authorized series metadata; transformed expressions require separate unit analysis.")
    else:
        limitations.append("The source unit was supplied as an argument and is not verified by original query metadata; transformed expressions require separate unit analysis.")
    if meta.get("partial"):
        limitations.append("The query reported a partial result.")
    if meta.get("ingestion_status") == "unknown":
        limitations.append("End-to-end ingestion completeness is unknown.")
    if metric_type == "counter":
        limitations.append("These are cumulative counter readings. Their magnitude does not establish volume, throughput, traffic share or dominant activity in the requested window. No reset-aware increase/rate was computed.")
    elif metric_type == "unknown":
        limitations.append("Metric type is unknown in the original query evidence. A caller hint does not certify it. If cumulative, readings do not establish interval volume or rate; verify type before interpreting activity.")
    result = {"schema_version": "argus.metric_evidence/v6", "input_sha256": digest, "expected_sha256_verified": expected_sha256 is not None, "resource_id": resource_id, "resource_identity_origin": "query_context" if resources else "retained_metric_labels" if resource_id else "unavailable", "resource_refs": resources, "source_keys": context.get("source_keys", []), "metric": next(iter(names)), **semantics, **facts, "requested_from": context.get("from"), "requested_to": context.get("to"), "latest_sample_at": meta.get("latest_sample_at"), "limitations": limitations}
    result["evaluation_window"] = evaluation_window.compare(context.get("from"), context.get("to"), facts["first_point_at"], facts["last_point_at"])
    result["query_partial"] = meta.get("partial") if type(meta.get("partial")) is bool else None
    if metric_type == "counter":
        result["interpretation_limits"] = {"input_basis": "original_cumulative_readings", "interval_activity": "not_established_by_this_input", "verification_needed": "reset_aware_query_or_verified_counter_boundary_evidence"}
    return result


def markdown_cell(value):
    return str(value).replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;").replace("|", "\\|").replace("\n", " ").replace("\r", " ").replace("`", "&#96;")


def render_markdown(evidence, include_json=True, locale="en-US"):
    t = translator(locale)
    formulas = {"identity": "display = raw", "percent": "display = raw * 100", "invert_percent": "display = (1 - raw) * 100", "bytes_to_gib": "display = raw / 2**30"}
    text = "# " + t("Metric evidence") + "\n\n" + t("Statistics cover the returned observations. Endpoint values retain their corresponding UTC timestamps; extrema describe the value range.") + "\n\n"
    text += t("Metric") + ": " + markdown_cell(evidence.get("metric", t("unavailable"))) + ".\n\n"
    text += t("Requested UTC window") + ": " + markdown_cell(evidence.get("requested_from") or t("unavailable")) + " " + t("to") + " " + markdown_cell(evidence.get("requested_to") or t("unavailable")) + ".\n\n"
    text += t("Returned observations") + ": " + markdown_cell(evidence["point_count"]) + "; " + t("UTC range") + " " + markdown_cell(evidence["first_point_at"]) + " " + t("to") + " " + markdown_cell(evidence["last_point_at"]) + ".\n\n"
    if "evaluation_window" in evidence:
        text += evaluation_window.render_markdown(evidence["evaluation_window"], locale)
    text += t("Source unit") + ": " + markdown_cell(evidence["unit"]) + "; " + t("display unit") + ": " + markdown_cell(evidence["display_unit"]) + "; " + t("conversion") + ": " + formulas[evidence["display_transform"]] + ".\n\n"
    metric_type = evidence.get("metric_type", "unknown")
    text += t("Metric type") + ": " + markdown_cell(metric_type) + ".\n\n"
    if metric_type == "counter":
        text += t("These tables describe cumulative readings. Interval volume, throughput and activity direction require reset-aware queries; counter magnitude alone cannot establish them. No such computation was performed here.") + "\n\n"
        text += t("Interval activity level and steadiness remain unknown from this input. Increasing cumulative readings alone do not establish quiet, busy or steady network activity; separate reset-aware evidence is needed.") + "\n\n"
    elif metric_type == "unknown":
        text += t("Metric type is unknown in the original query evidence; a caller hint does not certify it. Verify type before interpreting interval activity or rates.") + "\n\n"
    text += table_header(t, ("Series", "Points", "Raw minimum", "Raw maximum", "Raw mean", "Display minimum", "Display maximum", "Display mean"))
    labels = []
    for index, row in enumerate(evidence["series"], start=1):
        dimensions = {key: value for key, value in row["labels"].items() if key != "__name__" and not key.startswith(("argus_", "otel_scope_"))}
        # Keep full identity/provenance in JSON, while the human table shows
        # business dimensions and a stable index into that complete population.
        label = str(index) + " " + json.dumps(dimensions, ensure_ascii=False, sort_keys=True, separators=(",", ":"))
        labels.append(label)
        cells = [label] + [row[key] for key in ("point_count", "value_min", "value_max", "value_mean", "display_min", "display_max", "display_value_mean")]
        text += "| " + " | ".join(markdown_cell(cell) for cell in cells) + " |\n"
    text += "\n" + table_header(t, ("Series", "First UTC", "First raw", "First display", "Last UTC", "Last raw", "Last display"))
    for label, row in zip(labels, evidence["series"]):
        cells = [label] + [row[key] for key in ("first_point_at", "first_point_value", "display_first_point_value", "last_point_at", "last_point_value", "display_last_point_value")]
        text += "| " + " | ".join(markdown_cell(cell) for cell in cells) + " |\n"
    encoded = json.dumps(evidence, ensure_ascii=False, indent=2, allow_nan=False)
    return text + ("\n```json\n" + encoded.replace("`", "\\u0060") + "\n```\n" if include_json else "")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("input", type=Path)
    parser.add_argument("--unit", help="optional consistency hint; required only for legacy query files without source unit metadata")
    parser.add_argument("--metric-type", choices=METRIC_TYPES, default="unknown", help="optional consistency hint; original query metadata determines type and conflicting hints fail")
    parser.add_argument("--transform", choices=("auto", "identity", "percent", "invert_percent", "bytes_to_gib"), default="auto")
    parser.add_argument("--expected-sha256", help="original query workspace_file.sha256; a later read/bash result_hash is not the original file checksum")
    parser.add_argument("--json-out", type=Path)
    parser.add_argument("--markdown-out", type=Path)
    parser.add_argument("--locale", choices=LOCALES, default="en-US", help="human report language; machine JSON and original labels stay unchanged")
    args = parser.parse_args()
    outputs = [path.resolve() for path in (args.json_out, args.markdown_out) if path is not None]
    if args.input.resolve() in outputs or len(set(outputs)) != len(outputs):
        parser.error("output paths must be distinct and must not replace the query file")
    if args.input.stat().st_size > MAX_BYTES:
        parser.error("query file exceeds 16 MiB evidence limit")
    try:
        with args.input.open("rb") as query_file:
            raw = query_file.read(MAX_BYTES + 1)
        evidence = summarize(raw, args.unit, args.transform, args.expected_sha256, args.metric_type)
        encoded = json.dumps(evidence, ensure_ascii=False, indent=2, allow_nan=False)
        markdown = render_markdown(evidence, locale=args.locale)
        for path, content in ((args.json_out, encoded + "\n"), (args.markdown_out, markdown)):
            if path is not None:
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text(content, encoding="utf-8")
        print(encoded)
    except (ValueError, KeyError, TypeError, OverflowError) as error:
        print("metric evidence unavailable: " + str(error), file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
