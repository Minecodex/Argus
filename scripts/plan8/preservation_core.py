"""Deterministic typed ClickHouse hashes and bounded multiset comparison.

Map key order is not semantic. Array order, duplicate rows, types and values are.
Ambiguous duplicate Map keys fail closed rather than being silently discarded.
"""
import gzip
import hashlib
import re

ENCODING = "argus.typed_tuple.canonical_maps/v2"
_ID = re.compile(r"[a-zA-Z_][a-zA-Z0-9_]*\Z")
_HASH = re.compile(r"[a-f0-9]{64}\Z")
_ATOM = re.compile(r"(?:U?Int(?:8|16|32|64|128|256)|Float(?:32|64)|String|UUID|Bool|Date|Date32)\Z")


def identifier(value):
    if not isinstance(value, str) or not _ID.fullmatch(value):
        raise ValueError("invalid SQL identifier")
    return "`" + value + "`"


def column_type(value):
    """Accept only the actual fact-table type grammar; never interpolate arbitrary SQL."""
    if not isinstance(value, str) or len(value) > 256:
        raise ValueError("invalid column type")
    value = re.sub(r"\s+", "", value)
    if _ATOM.fullmatch(value) or re.fullmatch(r"DateTime64\([0-9],'UTC'\)|DateTime\('UTC'\)|FixedString\([1-9][0-9]*\)", value):
        return value
    for prefix in ("Nullable", "LowCardinality", "Array"):
        if value.startswith(prefix + "(") and value.endswith(")"):
            return prefix + "(" + column_type(value[len(prefix)+1:-1]) + ")"
    if value == "Map(String,String)":
        return value
    if value.startswith("Tuple(") and value.endswith(")"):
        inner = value[6:-1]
        depth, start, parts = 0, 0, []
        for i, char in enumerate(inner):
            if char == "(": depth += 1
            elif char == ")": depth -= 1
            elif char == "," and depth == 0:
                parts.append(column_type(inner[start:i])); start = i + 1
            if depth < 0: raise ValueError("invalid tuple type")
        if depth: raise ValueError("invalid tuple type")
        parts.append(column_type(inner[start:]))
        return "Tuple(" + ",".join(parts) + ")"
    raise ValueError("unsupported column type; explicit review required")


def hash_expression(columns):
    expressions, ambiguous, seen = [], [], set()
    if not columns:
        raise ValueError("empty column projection")
    for col in columns:
        name, typ = identifier(col["name"]), column_type(col["type"])
        if name in seen: raise ValueError("duplicate column")
        seen.add(name)
        value = name
        if typ == "Map(String,String)":
            keys = "arraySort(mapKeys(" + name + "))"
            value = "mapFromArrays(" + keys + ",arrayMap(k -> " + name + "[k]," + keys + "))"
            ambiguous.append("length(mapKeys(" + name + "))!=arrayUniq(mapKeys(" + name + "))")
        elif "Map(" in typ:
            raise ValueError("nested Map requires an explicit canonicalization rule")
        expressions.append("CAST(" + value + " AS " + typ + ")")
    digest = "lower(hex(SHA256(toJSONString(tuple(" + ",".join(expressions) + ")))))"
    if ambiguous:
        digest = "if(throwIf(" + " OR ".join(ambiguous) + ",'AMBIGUOUS_MAP_KEYS'),''," + digest + ")"
    return digest


def require_encoding(manifest):
    if manifest.get("logical_hash_encoding") != ENCODING:
        raise ValueError("legacy or unknown baseline encoding; cannot reinterpret old hashes")


def sorted_hashes(values):
    previous = ""
    for raw in values:
        value = raw.strip()
        if not _HASH.fullmatch(value) or value < previous:
            raise ValueError("invalid or unsorted hash stream")
        previous = value
        yield value


def compare_hashes(original, current):
    """Require every original occurrence; append-only rows are allowed."""
    before, after = iter(sorted_hashes(original)), iter(sorted_hashes(current))
    expected, observed = next(before, None), next(after, None)
    matched, missing, added, original_count, current_count = 0, 0, 0, 0, 0
    examples = []
    while expected is not None or observed is not None:
        if expected is None or (observed is not None and observed < expected):
            added += 1; current_count += 1; observed = next(after, None)
        elif observed is None or expected < observed:
            missing += 1; original_count += 1
            if len(examples) < 16: examples.append(expected)
            expected = next(before, None)
        else:
            matched += 1; original_count += 1; current_count += 1
            expected, observed = next(before, None), next(after, None)
    return {"original_rows": original_count, "observed_rows": current_count,
            "matched_rows": matched, "missing_rows": missing, "added_rows": added,
            "missing_hash_examples": examples, "passed": missing == 0}


def file_digest(path):
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1 << 20), b""): digest.update(chunk)
    return digest.hexdigest()


def compare_archives(original, current):
    with gzip.open(original, "rt", encoding="ascii") as before, gzip.open(current, "rt", encoding="ascii") as after:
        return compare_hashes(before, after)
