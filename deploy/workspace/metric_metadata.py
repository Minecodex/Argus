"""Resolve original metric semantics from source-bound query metadata."""

ORIGIN = "authorized_series_metadata"
MAX_DESCRIPTORS = 128


def resolve(meta, context, name, unit_hint, type_hint):
    """Metadata provenance still depends on the original-file hash/import.

    This function is not a signature verifier. Caller hints are consistency
    checks and cannot authenticate a metric type when original metadata is absent.
    """
    origin = meta.get("input_metrics_origin")
    if not origin:
        if unit_hint is None:
            raise ValueError("legacy query file has no unit metadata; an explicit catalog unit is required")
        if type_hint == "histogram":
            raise ValueError("histograms need histogram-aware queries and analysis")
        return {"unit": unit_hint, "unit_origin": "provided_argument_unverified", "metric_type": "unknown", "metric_type_origin": "unavailable", "metric_type_hint": type_hint, "source_metric_metadata": [], "source_metric_metadata_complete": False}
    if origin != ORIGIN or meta.get("input_metrics_complete") is not True:
        raise ValueError("input metric metadata is unavailable/incomplete; narrow the original query/source before interpreting it")
    if meta.get("result_is_original_metric") is not True:
        raise ValueError("input metric metadata cannot supply derived expression output type/unit; use expression-aware analysis")
    values = meta.get("input_metrics")
    if not isinstance(values, list) or len(values) > MAX_DESCRIPTORS:
        raise ValueError("invalid input metric metadata")
    matches = [value for value in values if isinstance(value, dict) and value.get("name") == name]
    if not matches:
        raise ValueError("original metric has no matching input metadata")
    signatures = set()
    allowed = context.get("source_keys", [])
    for value in matches:
        for key in ("type", "unit", "storage_type", "temporality", "source_key"):
            if not isinstance(value.get(key), str):
                raise ValueError("invalid original metric descriptor")
        if not isinstance(value.get("monotonic"), bool):
            raise ValueError("invalid original monotonic metadata")
        if allowed and value["source_key"] not in allowed:
            raise ValueError("metric metadata refers to a source outside the query context")
        signatures.add((value["type"], value["unit"], value["storage_type"], value["temporality"], value["monotonic"]))
    if len(signatures) != 1:
        raise ValueError("selected metric has conflicting type/unit/temporality metadata; select an explicit homogeneous source")
    kind, unit, _, _, _ = next(iter(signatures))
    if unit_hint is not None and unit_hint != unit:
        raise ValueError("unit hint conflicts with original query metadata")
    if type_hint != "unknown" and type_hint != kind:
        raise ValueError("metric type hint conflicts with original query metadata")
    if kind == "histogram":
        raise ValueError("histograms need histogram-aware queries and analysis")
    effective = kind if kind in ("gauge", "counter") else "unknown"
    result = {"unit": unit, "unit_origin": "query_metadata", "metric_type": effective, "metric_type_origin": "query_metadata", "metric_type_hint": type_hint, "source_metric_metadata": matches, "source_metric_metadata_complete": True}
    meanings = meta.get("input_metric_semantics", [])
    if not isinstance(meanings, list) or len(meanings) > MAX_DESCRIPTORS:
        raise ValueError("invalid input quantity definitions")
    resolved = {}
    for entry in meanings:
        if not isinstance(entry, dict) or entry.get("name") != name:
            continue
        source = entry.get("source_key")
        if source not in {value["source_key"] for value in matches} or entry.get("type") != kind or entry.get("unit") != unit:
            raise ValueError("quantity definition conflicts with the original input identity/type/unit")
        meaning = entry.get("semantics", {})
        if (meaning.get("origin") == "builtin_collection_definition"
                and meaning.get("definition_version") == "argus.builtin_metric_semantics/v1"
                and meaning.get("quantity_kind") == "ratio"
                and meaning.get("recommended_display_transform") == "percent"
                and meaning.get("definition_id") == "host_metrics/" + name
                and kind == "gauge" and unit == "1"):
            resolved[source] = meaning
    if resolved and set(resolved) == {value["source_key"] for value in matches}:
        result.update(quantity_kind="ratio", quantity_origin="builtin_collection_definition", recommended_display_transform="percent", quantity_definition_refs=sorted({value["definition_id"] for value in resolved.values()}))
    return result
