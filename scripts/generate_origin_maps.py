#!/usr/bin/env python3
"""Generate stylized origin maps for Bow Tie Duck landing pages.

Data source: https://github.com/johan/world.geo.json (Open Data Commons)
"""

from __future__ import annotations

import json
import math
import pathlib
import urllib.request
from dataclasses import dataclass
from html import escape
from typing import Iterable, List, Tuple

WORLD_GEOJSON_URL = "https://raw.githubusercontent.com/johan/world.geo.json/master/countries.geo.json"
WIDTH = 1600
HEIGHT = 920
PAD_X = 44
PAD_Y = 34

LAND_FILL = "#edeadf"
LAND_STROKE = "#d8d4c5"
HIGHLIGHT_FILL = "#e5e0ba"
HIGHLIGHT_STROKE = "#b9ad7c"
PH_FILL = "#f2d382"
PH_STROKE = "#9d8353"
GRID_COLOR = "#d6d2c2"
ROUTE_COLOR = "#463e1a"
ROUTE_GLOW = "#ff9418"
TEXT = "#1c232d"
TEXT_SOFT = "#463e1a"

DEST = ("Manila", 120.9842, 14.5995)


@dataclass(frozen=True)
class Point:
    label: str
    lon: float
    lat: float


@dataclass(frozen=True)
class MapSpec:
    file_stem: str
    title: str
    subtitle: str
    sources: Tuple[Point, ...]
    highlight_countries: Tuple[str, ...]


SPECS: Tuple[MapSpec, ...] = (
    MapSpec(
        file_stem="bordier-origin-map",
        title="Maison Bordier Route",
        subtitle="Saint-Malo, Brittany to Manila",
        sources=(Point("Saint-Malo", -2.0257, 48.6493),),
        highlight_countries=("France", "Philippines"),
    ),
    MapSpec(
        file_stem="perrotte-origin-map",
        title="Maison Perrotte Route",
        subtitle="Angers, Loire Valley to Manila",
        sources=(Point("Angers", -0.5564, 47.4784),),
        highlight_countries=("France", "Philippines"),
    ),
    MapSpec(
        file_stem="aberdeen-origin-map",
        title="Aberdeen Angus IGP Route",
        subtitle="Aberdeen, Scotland to Manila",
        sources=(Point("Aberdeen", -2.0943, 57.1497),),
        highlight_countries=("United Kingdom", "Philippines"),
    ),
    MapSpec(
        file_stem="mavrommatis-origin-map",
        title="Maison Mavrommatis Origins",
        subtitle="Cyprus and Mediterranean table to Manila",
        sources=(
            Point("Nicosia", 33.3823, 35.1856),
            Point("Athens", 23.7275, 37.9838),
            Point("Paris", 2.3522, 48.8566),
        ),
        highlight_countries=("Cyprus", "Greece", "France", "Philippines"),
    ),
    MapSpec(
        file_stem="rodel-origin-map",
        title="Rodel and Fils Freres Route",
        subtitle="French Atlantic canning tradition to Manila",
        sources=(Point("French Atlantic", -1.5534, 47.2184),),
        highlight_countries=("France", "Philippines"),
    ),
    MapSpec(
        file_stem="italian-spirit-origin-map",
        title="Italian Spirit Origin Map",
        subtitle="Tuscany, Parma, South Tyrol to Manila",
        sources=(
            Point("Tuscany", 11.2558, 43.7696),
            Point("Parma", 10.3279, 44.8015),
            Point("South Tyrol", 11.3548, 46.4983),
        ),
        highlight_countries=("Italy", "Philippines"),
    ),
    MapSpec(
        file_stem="english-tea-time-origin-map",
        title="English Tea Time Origins",
        subtitle="Devon and France to Manila",
        sources=(
            Point("Devon", -3.7160, 50.7184),
            Point("Angers", -0.5564, 47.4784),
        ),
        highlight_countries=("United Kingdom", "France", "Philippines"),
    ),
)


def mercator_y(lat: float) -> float:
    lat = max(min(lat, 85.0), -85.0)
    rad = math.radians(lat)
    return math.log(math.tan(math.pi / 4 + rad / 2))


def project(lon: float, lat: float) -> Tuple[float, float]:
    x = (lon + 180.0) / 360.0
    y = (mercator_y(lat) - mercator_y(-85.0)) / (mercator_y(85.0) - mercator_y(-85.0))
    sx = PAD_X + x * (WIDTH - 2 * PAD_X)
    sy = HEIGHT - (PAD_Y + y * (HEIGHT - 2 * PAD_Y))
    return sx, sy


def geometry_to_paths(geom: dict) -> List[str]:
    gtype = geom.get("type")
    coords = geom.get("coordinates", [])
    out: List[str] = []

    def ring_to_path(ring: Iterable[Iterable[float]]) -> str:
        points = list(ring)
        if not points:
            return ""
        d_parts = []
        for idx, pair in enumerate(points):
            lon, lat = float(pair[0]), float(pair[1])
            x, y = project(lon, lat)
            cmd = "M" if idx == 0 else "L"
            d_parts.append(f"{cmd}{x:.2f},{y:.2f}")
        d_parts.append("Z")
        return " ".join(d_parts)

    if gtype == "Polygon":
        for ring in coords:
            path = ring_to_path(ring)
            if path:
                out.append(path)
    elif gtype == "MultiPolygon":
        for polygon in coords:
            for ring in polygon:
                path = ring_to_path(ring)
                if path:
                    out.append(path)
    return out


def arc_path(src: Point, dst: Point) -> str:
    sx, sy = project(src.lon, src.lat)
    dx, dy = project(dst.lon, dst.lat)
    mx, my = (sx + dx) / 2, (sy + dy) / 2
    dist = math.hypot(dx - sx, dy - sy)
    lift = max(56.0, dist * 0.22)
    cx, cy = mx, min(sy, dy) - lift
    return f"M{sx:.2f},{sy:.2f} Q{cx:.2f},{cy:.2f} {dx:.2f},{dy:.2f}"


def render_svg(spec: MapSpec, features: list[dict]) -> str:
    def country_name(feature: dict) -> str:
        props = feature.get("properties", {})
        return str(props.get("name", "")).strip()

    highlight = set(spec.highlight_countries)

    lines = []
    lines.append(
        f'<svg xmlns="http://www.w3.org/2000/svg" width="{WIDTH}" height="{HEIGHT}" viewBox="0 0 {WIDTH} {HEIGHT}" role="img" aria-label="{escape(spec.title)}">'
    )
    lines.append("<defs>")
    lines.append('  <linearGradient id="paper" x1="0" y1="0" x2="0" y2="1">')
    lines.append('    <stop offset="0%" stop-color="#faf8ef"/>')
    lines.append('    <stop offset="100%" stop-color="#f1eee1"/>')
    lines.append("  </linearGradient>")
    lines.append('  <radialGradient id="routeGlow" cx="50%" cy="30%" r="70%">')
    lines.append('    <stop offset="0%" stop-color="#ff9418" stop-opacity="0.35"/>')
    lines.append('    <stop offset="100%" stop-color="#ff9418" stop-opacity="0"/>')
    lines.append("  </radialGradient>")
    lines.append("</defs>")

    lines.append(f'<rect x="0" y="0" width="{WIDTH}" height="{HEIGHT}" fill="url(#paper)"/>')

    for lon in range(-150, 181, 30):
        x1, y1 = project(float(lon), -83.0)
        x2, y2 = project(float(lon), 83.0)
        lines.append(
            f'<path d="M{x1:.2f},{y1:.2f} L{x2:.2f},{y2:.2f}" stroke="{GRID_COLOR}" stroke-width="1" opacity="0.36"/>'
        )
    for lat in (-60, -30, 0, 30, 60):
        x1, y1 = project(-179.0, float(lat))
        x2, y2 = project(179.0, float(lat))
        lines.append(
            f'<path d="M{x1:.2f},{y1:.2f} L{x2:.2f},{y2:.2f}" stroke="{GRID_COLOR}" stroke-width="1" opacity="0.36"/>'
        )

    lines.append('<g id="land" fill-rule="evenodd">')
    for feature in features:
        name = country_name(feature)
        paths = geometry_to_paths(feature.get("geometry", {}))
        if not paths:
            continue
        fill = LAND_FILL
        stroke = LAND_STROKE
        if name in highlight:
            fill = HIGHLIGHT_FILL
            stroke = HIGHLIGHT_STROKE
        if name == "Philippines":
            fill = PH_FILL
            stroke = PH_STROKE
        for d in paths:
            lines.append(
                f'  <path d="{d}" fill="{fill}" stroke="{stroke}" stroke-width="0.9"/>'
            )
    lines.append("</g>")

    lines.append(
        '<ellipse cx="800" cy="318" rx="510" ry="220" fill="url(#routeGlow)" opacity="0.26"/>'
    )

    dst = Point(*DEST)
    for src in spec.sources:
        d = arc_path(src, dst)
        lines.append(f'<path d="{d}" fill="none" stroke="{ROUTE_GLOW}" stroke-opacity="0.26" stroke-width="8"/>')
        lines.append(f'<path d="{d}" fill="none" stroke="{ROUTE_COLOR}" stroke-opacity="0.9" stroke-width="2.8"/>')

    def marker(point: Point, is_destination: bool = False) -> None:
        x, y = project(point.lon, point.lat)
        dot_fill = "#ff9418" if is_destination else "#463e1a"
        dot_stroke = "#6d5320" if is_destination else "#f7f1dd"
        lines.append(f'<circle cx="{x:.2f}" cy="{y:.2f}" r="7.8" fill="{dot_fill}" stroke="{dot_stroke}" stroke-width="2"/>')
        lines.append(f'<circle cx="{x:.2f}" cy="{y:.2f}" r="16" fill="none" stroke="{dot_fill}" stroke-opacity="0.25" stroke-width="1.5"/>')
        label_dx = 14
        label_dy = -12 if is_destination else -10
        lines.append(
            f'<text x="{x + label_dx:.2f}" y="{y + label_dy:.2f}" fill="{TEXT}" font-family="GothamPro, Centra, Arial, sans-serif" font-size="21" font-weight="700" letter-spacing="0.5">{escape(point.label)}</text>'
        )

    for src in spec.sources:
        marker(src)
    marker(dst, is_destination=True)

    lines.append(
        '<rect x="48" y="48" width="640" height="114" rx="14" fill="#ffffff" fill-opacity="0.82" stroke="#d8d4c5"/>'
    )
    lines.append(
        f'<text x="76" y="96" fill="{TEXT}" font-family="GothamPro, Centra, Arial, sans-serif" font-size="34" font-weight="700" letter-spacing="1">{escape(spec.title)}</text>'
    )
    lines.append(
        f'<text x="76" y="132" fill="{TEXT_SOFT}" font-family="Centra, Arial, sans-serif" font-size="23">{escape(spec.subtitle)}</text>'
    )
    lines.append(
        '<text x="1546" y="886" text-anchor="end" fill="#7a7258" font-family="Centra, Arial, sans-serif" font-size="14">Map data: OpenStreetMap contributors</text>'
    )

    lines.append("</svg>")
    return "\n".join(lines)


def load_world_features() -> list[dict]:
    with urllib.request.urlopen(WORLD_GEOJSON_URL, timeout=30) as resp:
        payload = json.load(resp)
    return payload.get("features", [])


def main() -> None:
    root = pathlib.Path(__file__).resolve().parents[1]
    out_dir = root / "btd_main" / "landing" / "assets" / "images"
    out_dir.mkdir(parents=True, exist_ok=True)

    features = load_world_features()
    if not features:
        raise RuntimeError("No features loaded from world geojson")

    for spec in SPECS:
        svg = render_svg(spec, features)
        out_path = out_dir / f"{spec.file_stem}.svg"
        out_path.write_text(svg, encoding="utf-8")
        print(f"wrote {out_path}")


if __name__ == "__main__":
    main()
