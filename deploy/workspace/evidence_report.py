#!/usr/bin/env python3
"""Assemble reproducible metric/panel reports from original offline evidence."""
import argparse
import json
from pathlib import Path
import sys

import dashboard_evidence
import metric_evidence
from evidence_text import LOCALES, translator
import report_receipt

MAX_DASHBOARDS = 8
MAX_SOURCE_BYTES = 64 * 1024 * 1024
MAX_SOURCE_FILES = 512


def compose(query, unit=None, transform="auto", expected_sha256=None, manifests=(), notes=None, metric_type="unknown", locale="en-US"):
    t = translator(locale)
    if len(manifests) > MAX_DASHBOARDS:
        raise ValueError("report exceeds dashboard limit")
    query = Path(query).resolve()
    if query.stat().st_size > metric_evidence.MAX_BYTES:
        raise ValueError("metric query exceeds evidence size limit")
    with query.open("rb") as stream:
        metric = metric_evidence.summarize(stream.read(metric_evidence.MAX_BYTES + 1), unit, transform, expected_sha256, metric_type)
    inputs, dashboards, seen = {query}, [], set()
    point_count = metric["point_count"]
    sections = [metric_evidence.render_markdown(metric, include_json=False, locale=locale)]
    for manifest in manifests:
        dashboard, files = dashboard_evidence.summarize_manifest(Path(manifest))
        identity = dashboard["job_id"], dashboard["attempt_id"]
        if identity in seen:
            raise ValueError("report repeats a dashboard job/attempt")
        seen.add(identity)
        inputs.update(files)
        point_count += sum(panel.get("point_count", 0) for panel in dashboard["panels"])
        dashboards.append(dashboard)
        sections.append(dashboard_evidence.render_markdown(dashboard, include_json=False, locale=locale))
        check_limits(inputs, point_count)
    if notes is not None:
        notes = Path(notes).resolve()
        if notes.stat().st_size > 64 * 1024:
            raise ValueError("qualitative notes exceed size limit")
        sections.append("# " + t("Interpretation") + "\n\n" + notes.read_text(encoding="utf-8") + "\n")
        inputs.add(notes)
    check_limits(inputs, point_count)
    conclusion = {"metric_evidence": metric, "dashboard_evidence": dashboards} if dashboards else metric
    encoded = json.dumps(conclusion, ensure_ascii=False, indent=2, allow_nan=False)
    # The last summary is identical in the JSON file and report. Neither
    # the model nor this composer invents a new set of panel/metric facts.
    sections.append("# " + t("Evidence summary") + "\n\n```json\n" + encoded.replace("`", "\\u0060") + "\n```\n")
    markdown = "\n".join(sections)
    report_receipt.validate_markdown(conclusion, markdown)
    return conclusion, markdown, inputs


def check_limits(inputs, point_count):
    if len(inputs) > MAX_SOURCE_FILES or sum(path.stat().st_size for path in inputs) > MAX_SOURCE_BYTES:
        raise ValueError("report exceeds source file/byte limit")
    if point_count > metric_evidence.MAX_POINTS:
        raise ValueError("report exceeds combined point limit")


def save_outputs(conclusion, markdown, inputs, json_out, markdown_out):
    outputs = [Path(json_out).resolve(), Path(markdown_out).resolve()]
    if outputs[0] == outputs[1] or any(output in inputs for output in outputs):
        raise ValueError("report outputs must be distinct and preserve every original input")
    encoded = json.dumps(conclusion, ensure_ascii=False, indent=2, allow_nan=False)
    for destination, content in zip(outputs, (encoded + "\n", markdown)):
        destination.parent.mkdir(parents=True, exist_ok=True)
        destination.write_text(content, encoding="utf-8", newline="\n")
    return encoded


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("query", type=Path)
    parser.add_argument("--unit", help="optional source-unit consistency hint; required for legacy inputs")
    parser.add_argument("--metric-type", choices=metric_evidence.METRIC_TYPES, default="unknown")
    parser.add_argument("--transform", choices=("auto", "identity", "percent", "invert_percent", "bytes_to_gib"), default="auto")
    parser.add_argument("--expected-sha256")
    parser.add_argument("--dashboard-manifest", action="append", type=Path, default=[])
    parser.add_argument("--notes", type=Path, help="path to an existing UTF-8 qualitative notes FILE; not inline text; numeric additions require separate evidence")
    parser.add_argument("--json-out", required=True, type=Path)
    parser.add_argument("--markdown-out", required=True, type=Path)
    parser.add_argument("--locale", choices=LOCALES, default="en-US", help="human report language; machine facts and source labels are unchanged")
    parser.add_argument("--print-evidence", action="store_true", help="explicitly print complete evidence JSON instead of the default bounded file/validation receipt")
    args = parser.parse_args()
    try:
        conclusion, markdown, inputs = compose(args.query, args.unit, args.transform, args.expected_sha256, args.dashboard_manifest, args.notes, args.metric_type, args.locale)
        encoded = save_outputs(conclusion, markdown, inputs, args.json_out, args.markdown_out)
        receipt = report_receipt.create(conclusion, markdown, args.json_out, args.markdown_out, inputs)
        print(encoded if args.print_evidence else receipt)
    except (OSError, ValueError, KeyError, TypeError, OverflowError) as error:
        print("evidence report unavailable: " + str(error), file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
