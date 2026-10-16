"""Compare one frozen Plan6 visual run at original business-region dimensions."""
import argparse
import html
import json
import hashlib
import re
from pathlib import Path
from PIL import Image, ImageChops, ImageStat


def crop_region(image, box):
    left, top = round(box["x"]), round(box["y"])
    right = round(box["x"] + box["width"])
    bottom = round(box["y"] + box["height"])
    if left < 0 or top < 0 or right > image.width or bottom > image.height or right <= left or bottom <= top:
        raise ValueError("business region is clipped or outside the captured image")
    return image.crop((left, top, right, bottom))


def geometry_differences(row):
    """No shell coordinates are compared; geometry is relative to each region."""
    differences = []
    for selector, left in row.get("demoGeometry", {}).items():
        right = row.get("portalGeometry", {}).get(selector, [])
        if len(left) != len(right):
            differences.append({"selector": selector, "property": "count", "demo": len(left), "portal": len(right)})
            continue
        for index, (a, b) in enumerate(zip(left, right)):
            for name in ("x", "y", "width", "height"):
                av, bv = a[name], b[name]
                if name in ("x", "y"):
                    av -= row["demoBox"][name]
                    bv -= row["portalBox"][name]
                if abs(av - bv) > 1:  # CSS pixel rounding only, not layout tolerance.
                    differences.append({"selector": selector, "index": index, "property": name, "demo": av, "portal": bv})
            for name, value in a.get("styles", {}).items():
                if name == "height":
                    continue
                if b.get("styles", {}).get(name) != value:
                    differences.append({"selector": selector, "index": index, "property": name, "demo": value, "portal": b.get("styles", {}).get(name)})
    return differences


def build(output):
    metadata = json.loads((output / "run.json").read_text(encoding="utf-8-sig"))
    records, seen = [], set()
    reviews_path = output / "reviewed-differences.json"
    reviews = json.loads(reviews_path.read_text(encoding="utf-8-sig")) if reviews_path.exists() else {}
    allowed_reasons = {"credential_reference", "advanced_promql_and_matchers", "required_field_markers", "actual_iam_roles", "demo_only_copy", "actual_metric_observations"}
    for manifest in sorted(output.glob("*-*.json")):
        if not re.fullmatch(r"(?:zh|en)-(?:light|dark)-(?:1280|1440|1920)(?:-scenarios)?\.json", manifest.name):
            continue
        rows = json.loads(manifest.read_text(encoding="utf-8-sig"))
        if not isinstance(rows, list):
            continue
        for original in rows:
            row = dict(original)
            key = f"{row['locale']}-{row['theme']}-{row['viewport']['width']}-{row['tab']}"
            if key in seen or row.get("run_id") != metadata["run_id"]:
                raise ValueError(f"duplicate or foreign visual record: {key}")
            seen.add(key)
            demo = crop_region(Image.open(output / f"{key}-demo-full.png").convert("RGB"), row.get("demoImageBox", row["demoBox"]))
            portal = crop_region(Image.open(output / f"{key}-portal-full.png").convert("RGB"), row.get("portalImageBox", row["portalBox"]))
            size = max(demo.width, portal.width), max(demo.height, portal.height)

            def canvas(image):
                result = Image.new("RGB", size, (255, 0, 255))
                result.paste(image, (0, 0))
                return result

            a, b = canvas(demo), canvas(portal)
            difference = ImageChops.difference(a, b)
            bands = difference.split()
            mask = ImageChops.lighter(ImageChops.lighter(bands[0], bands[1]), bands[2]).point(lambda value: 255 if value > 25 else 0)
            diff_pixels = mask.histogram()[255]
            contracts = geometry_differences(row)
            extra_evidence = b""
            extra_views = []
            extra_changed = False
            for view in row.get("extra_views", []):
                images = []
                for side in ("demo", "portal"):
                    image_file = (output / view[f"{side}File"]).resolve()
                    image_file.relative_to(output)
                    images.append(crop_region(Image.open(image_file).convert("RGB"), view[f"{side}ImageBox"]))
                extra_size = max(i.width for i in images), max(i.height for i in images)
                extra_canvases = []
                for i in images:
                    c = Image.new("RGB", extra_size, (255, 0, 255))
                    c.paste(i, (0, 0))
                    extra_canvases.append(c)
                extra_evidence += b"".join(i.tobytes() for i in extra_canvases)
                combined_extra = Image.new("RGB", (extra_size[0] * 2, extra_size[1]))
                combined_extra.paste(extra_canvases[0], (0, 0))
                combined_extra.paste(extra_canvases[1], (extra_size[0], 0))
                extra_key = f"{key}-{view['name']}"
                combined_extra.save(output / f"{extra_key}-side-by-side.png")
                Image.blend(*extra_canvases, .5).save(output / f"{extra_key}-overlay.png")
                ImageChops.difference(*extra_canvases).save(output / f"{extra_key}-difference.png")
                extra_diff = ImageChops.difference(*extra_canvases).split()
                changed = ImageChops.lighter(ImageChops.lighter(extra_diff[0], extra_diff[1]), extra_diff[2]).point(lambda value: 255 if value > 25 else 0).histogram()[255]
                extra_changed = extra_changed or bool(changed) or images[0].size != images[1].size
                extra_views.append({"name": view["name"], "key": extra_key, "region_sizes": {"demo": images[0].size, "portal": images[1].size}, "difference_ratio": changed / (extra_size[0] * extra_size[1])})
            if row["tab"] == "conversation-preview" and not extra_views:
                raise ValueError(f"pending card is missing full scroll-content evidence: {key}")
            if row["tab"] == "detail" and not any(view["name"] == "detail-body" for view in extra_views):
                raise ValueError(f"detail drawer is missing full scroll-content evidence: {key}")
            if row["tab"] in {"editor", "create"} and not any(view["name"] == "editor-fields" for view in extra_views):
                raise ValueError(f"policy editor is missing full scroll-content evidence: {key}")
            evidence_hash = hashlib.sha256(a.tobytes() + b.tobytes() + json.dumps(contracts, sort_keys=True).encode() + extra_evidence).hexdigest()
            combined = Image.new("RGB", (size[0] * 2, size[1]), (255, 255, 255))
            combined.paste(a, (0, 0))
            combined.paste(b, (size[0], 0))
            combined.save(output / f"{key}-side-by-side.png")
            Image.blend(a, b, .5).save(output / f"{key}-overlay.png")
            difference.save(output / f"{key}-difference.png")
            row.update(key=key, region_sizes={"demo": demo.size, "portal": portal.size},
                       difference_ratio=diff_pixels / (size[0] * size[1]),
                       mean_absolute_difference=sum(ImageStat.Stat(difference).mean) / 3,
                       contract_differences=contracts,
                       full_content_views=extra_views,
                       evidence_hash=evidence_hash,
                       status="needs_review" if diff_pixels or contracts or demo.size != portal.size or extra_changed else "matched")
            review = reviews.get(key)
            if review:
                if review.get("evidence_hash") != evidence_hash or review.get("source_hashes") != metadata["source_hashes"]:
                    raise ValueError(f"stale visual review: {key}")
                if review.get("allowed_contract_differences") != contracts:
                    raise ValueError(f"unreviewed control geometry/style difference: {key}")
                if not review.get("explanation") or not review.get("reviewed_regions"):
                    raise ValueError(f"visual review lacks named regions and rationale: {key}")
                if review.get("decision") == "matched":
                    # A reviewer may distinguish rasterization from a business
                    # difference only when measured controls are identical.
                    if contracts:
                        raise ValueError(f"matched review has different controls: {key}")
                    if any(abs(a - b) > 1 for a, b in zip(demo.size, portal.size)):
                        raise ValueError(f"matched review has different business bounds: {key}")
                    if any(any(abs(a - b) > 1 for a, b in zip(view["region_sizes"]["demo"], view["region_sizes"]["portal"])) for view in extra_views):
                        raise ValueError(f"matched review has different full-content bounds: {key}")
                    row.update(status="matched", review=review)
                elif review.get("decision") == "failed":
                    row.update(status="failed", review=review)
                else:
                    reasons = review.get("required_differences", [])
                    if not reasons or not set(reasons).issubset(allowed_reasons) or not review.get("fields"):
                        raise ValueError(f"unapproved or unscoped visual difference: {key}")
                    row.update(status="approved_difference", review=review)
            records.append(row)
    if not records:
        raise ValueError("visual run has no records")
    (output / "comparison-results.json").write_text(json.dumps(records, ensure_ascii=False, indent=2), encoding="utf-8")
    cards = []
    for row in records:
        key = html.escape(row["key"])
        contracts = html.escape(json.dumps(row["contract_differences"], ensure_ascii=False, indent=2))
        full_views = "".join(f'<details><summary>Complete original-pixel {html.escape(view["name"])} scroll exposures</summary><div class="images"><img src="{html.escape(view["key"])}-side-by-side.png" alt="Complete business fields"><img src="{html.escape(view["key"])}-overlay.png" alt="Overlay"><img src="{html.escape(view["key"])}-difference.png" alt="Difference"></div></details>' for view in row["full_content_views"])
        cards.append(f'<article data-status="{row["status"]}"><h2>{key} — {row["difference_ratio"]:.2%} changed pixels</h2>'
                     f'<p>Demo left, portal right. Status: {row["status"]}. Geometry/style differences: {len(row["contract_differences"])}.</p>'
                     f'<div class="images"><img src="{key}-side-by-side.png" alt="Side by side at original dimensions"></div>'
                     f'<details><summary>Overlay, difference and measured contracts</summary><div class="images"><img src="{key}-overlay.png" alt="Overlay">'
                     f'<img src="{key}-difference.png" alt="Difference"></div><pre>{contracts}</pre></details>{full_views}</article>')
    page = '<!doctype html><meta charset="utf-8"><title>Plan6 UI comparison</title><style>body{font-family:system-ui;margin:24px;background:#f6f3ed}article{margin:24px 0;border:1px solid #bbb;padding:16px}.images{overflow:auto}img{max-width:none;display:block}h2{font-size:18px}pre{white-space:pre-wrap}</style><h1>Plan6 UI comparison evidence</h1><p>Original pixel dimensions, actual region bounds; no scaling or business-control masks. Capturing screenshots does not pass visual acceptance.</p>' + "".join(cards)
    (output / "index.html").write_text(page, encoding="utf-8")
    required = {"active", "history", "deliveries", "policies", "noise", "channels", "editor", "preview", "detail", "silence-editor", "inhibition-editor", "channel-editor", "route-editor", "conversation-query", "conversation-preview", "organization-permissions", "organization-audit", "create", "create-preview"}
    expected = {f"{locale}-{theme}-{width}-{case}" for locale in ("zh", "en") for theme in ("light", "dark") for width in (1280, 1440, 1920) for case in required}
    missing = sorted(expected - seen)
    summary = {"run_id": metadata["run_id"], "records": len(records), "base_records": sum(r["tab"] in {"active", "history", "deliveries", "policies", "noise", "channels"} for r in records), "needs_review": sum(r["status"] == "needs_review" for r in records), "approved_differences": sum(r["status"] == "approved_difference" for r in records), "missing_cases": missing, "complete": not missing, "passed": not missing and all(r["status"] in {"matched", "approved_difference"} for r in records)}
    (output / "summary.json").write_text(json.dumps(summary, indent=2), encoding="utf-8")
    print(json.dumps({key: value for key, value in summary.items() if key != "missing_cases"} | {"missing_count": len(missing)}))


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--input-dir", required=True, type=Path)
    build(parser.parse_args().input_dir.resolve())
