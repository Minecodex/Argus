"""Stitch original screenshot pixels; reject gaps or out-of-image strips."""
import json
import sys
from pathlib import Path
from PIL import Image

data = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
canvas = Image.new("RGB", (data["width"], data["height"]))
coverage = Image.new("L", canvas.size)
for piece in data["pieces"]:
    source = Image.open(piece["file"]).convert("RGB")
    b = piece["source"]
    bounds = (b["x"], b["y"], b["x"] + b["width"], b["y"] + b["height"])
    if min(bounds) < 0 or bounds[2] > source.width or bounds[3] > source.height:
        raise ValueError("capture strip is outside the original screenshot")
    x, y = piece.get("x", 0), piece["y"]
    if x < 0 or y < 0 or x + b["width"] > canvas.width or y + b["height"] > canvas.height:
        raise ValueError("capture tile is outside the business region")
    canvas.paste(source.crop(bounds), (x, y))
    coverage.paste(255, (x, y, x + b["width"], y + b["height"]))
if coverage.getextrema() != (255, 255):
    raise ValueError("business capture is incomplete")
canvas.save(data["output"])
