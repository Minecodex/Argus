"""Exact time comparisons for returned evaluation points, not raw coverage."""
from datetime import datetime, timezone
from decimal import Decimal
import re

from evidence_text import table_header, translator

RFC3339 = re.compile(r"(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.(\d{1,9}))?(Z|[+-]\d{2}:\d{2})")
EPOCH = datetime(1970, 1, 1, tzinfo=timezone.utc)


def instant(value):
    match = RFC3339.fullmatch(value) if isinstance(value, str) else None
    if match is None:
        raise ValueError("time comparison requires explicit RFC3339 timestamps with a timezone")
    base, fraction, zone = match.groups()
    delta = datetime.fromisoformat(base + zone.replace("Z", "+00:00")).astimezone(timezone.utc) - EPOCH
    return Decimal(delta.days * 86400 + delta.seconds) + Decimal("0." + (fraction or "0"))


def number(value):
    return int(value) if value == value.to_integral_value() else float(value)


def relation(offset):
    return "after" if offset > 0 else "before" if offset < 0 else "equal"


def compare(requested_from, requested_to, first=None, last=None):
    result = {
        "schema_version": "argus.evaluation_window/v1",
        "basis": "returned_evaluation_timestamps",
        "requested_bounds": "unavailable", "returned_bounds": "unavailable",
        "requested_duration_seconds": None, "returned_span_seconds": None,
        "first_vs_requested_from": None, "first_offset_seconds": None,
        "last_vs_requested_to": None, "last_offset_seconds": None,
        "raw_sampling_coverage": "not_established",
    }
    start = instant(requested_from) if requested_from is not None else None
    end = instant(requested_to) if requested_to is not None else None
    if start is not None and end is not None:
        if end < start:
            raise ValueError("requested time bounds are reversed")
        result.update(requested_bounds="available", requested_duration_seconds=number(end - start))
    if first is None and last is None:
        return result
    if first is None or last is None:
        raise ValueError("both returned endpoint timestamps are required")
    begin, finish = instant(first), instant(last)
    if finish < begin:
        raise ValueError("returned endpoint timestamps are reversed")
    result.update(returned_bounds="available", returned_span_seconds=number(finish - begin))
    if start is not None:
        offset = begin - start
        result.update(first_vs_requested_from=relation(offset), first_offset_seconds=number(offset))
    if end is not None:
        offset = finish - end
        result.update(last_vs_requested_to=relation(offset), last_offset_seconds=number(offset))
    return result


def render_markdown(value, locale="en-US"):
    t = translator(locale)
    fields = (
        ("Requested duration (seconds)", "requested_duration_seconds"),
        ("Returned evaluation span (seconds)", "returned_span_seconds"),
        ("First evaluation relative to requested start", "first_vs_requested_from"),
        ("First evaluation minus requested start (seconds)", "first_offset_seconds"),
        ("Last evaluation relative to requested end", "last_vs_requested_to"),
        ("Last evaluation minus requested end (seconds)", "last_offset_seconds"),
    )
    text = "## " + t("Evaluation time comparison") + "\n\n"
    text += table_header(t, ("Time fact", "Computed result"))
    for label, key in fields:
        cell = value[key]
        text += "| " + t(label) + " | " + (t("unavailable") if cell is None else t(cell) if isinstance(cell, str) else str(cell)) + " |\n"
    return text + "\n" + t("Offsets are signed: positive means after, negative means before. The span is elapsed time between returned endpoints; it does not prove continuous raw sampling or ingestion completeness.") + "\n\n"
