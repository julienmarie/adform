#!/usr/bin/env python3
"""Generate the Caviar de Neuvic origin map as a reproducible static SVG.

Data sources:
- Natural Earth admin boundaries (public domain)
- Route points from producer and destination locations
"""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
from typing import Iterable

import geopandas as gpd
import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt
import numpy as np
from pyproj import Transformer
from shapely.geometry import LineString


@dataclass(frozen=True)
class MapConfig:
    place_name: str
    output_svg: Path
    world_url: str
    figsize: tuple[float, float]
    bg_color: str
    ocean_color: str
    land_color: str
    land_edge: str
    france_color: str
    philippines_color: str
    route_color: str
    route_glow: str


CONFIG = MapConfig(
    place_name="Caviar de Neuvic Origin",
    output_svg=Path("btd_main/landing/assets/images/neuvic/neuvic-origin-map.svg"),
    world_url="https://naturalearth.s3.amazonaws.com/110m_cultural/ne_110m_admin_0_countries.zip",
    figsize=(16, 9),
    bg_color="#f7f3e9",
    ocean_color="#f2efe4",
    land_color="#e8e3d6",
    land_edge="#cec7b6",
    france_color="#d9cfad",
    philippines_color="#f2d382",
    route_color="#463e1a",
    route_glow="#ff9418",
)

# Producer and destination points in WGS84 lon/lat.
NEUVIC = (0.4016, 45.1008)
SOURZAC = (0.3577, 45.0383)
MANILA = (120.9842, 14.5995)


def _load_world(world_url: str) -> tuple[gpd.GeoDataFrame, gpd.GeoDataFrame]:
    world_wgs84 = gpd.read_file(world_url)
    world_3857 = world_wgs84.to_crs(epsg=3857)
    return world_wgs84, world_3857


def _project_points(points: Iterable[tuple[float, float]]) -> list[tuple[float, float]]:
    tx = Transformer.from_crs("EPSG:4326", "EPSG:3857", always_xy=True)
    return [tx.transform(lon, lat) for lon, lat in points]


def _project_bbox(lon_min: float, lat_min: float, lon_max: float, lat_max: float) -> tuple[float, float, float, float]:
    tx = Transformer.from_crs("EPSG:4326", "EPSG:3857", always_xy=True)
    x0, y0 = tx.transform(lon_min, lat_min)
    x1, y1 = tx.transform(lon_max, lat_max)
    minx, maxx = min(x0, x1), max(x0, x1)
    miny, maxy = min(y0, y1), max(y0, y1)
    return minx, miny, maxx, maxy


def _bezier_curve(
    start_xy: tuple[float, float],
    end_xy: tuple[float, float],
    curve_lift: float = 0.30,
    steps: int = 180,
) -> LineString:
    sx, sy = start_xy
    ex, ey = end_xy
    mx, my = (sx + ex) / 2.0, (sy + ey) / 2.0
    dist = float(np.hypot(ex - sx, ey - sy))
    # Lift northward for a transcontinental arc.
    cx, cy = mx, max(sy, ey) + (dist * curve_lift)
    t = np.linspace(0.0, 1.0, steps)
    x = (1 - t) ** 2 * sx + 2 * (1 - t) * t * cx + t**2 * ex
    y = (1 - t) ** 2 * sy + 2 * (1 - t) * t * cy + t**2 * ey
    return LineString(np.column_stack([x, y]))


def _metropolitan_france(france_geom_wgs84: gpd.GeoSeries) -> gpd.GeoDataFrame:
    fr = gpd.GeoDataFrame(geometry=france_geom_wgs84, crs="EPSG:4326").explode(index_parts=False)
    centroids = fr.to_crs(3857).geometry.centroid
    centroids = gpd.GeoSeries(centroids, crs=3857).to_crs(4326)
    metro = fr.loc[centroids.x.between(-6.5, 10.0) & centroids.y.between(41.0, 52.5)]
    return metro


def generate_map(config: MapConfig) -> Path:
    world_wgs84, world_3857 = _load_world(config.world_url)
    world_bg = world_3857.loc[world_3857["NAME"] != "Antarctica"].copy()

    fr_wgs84 = world_wgs84.loc[world_wgs84["NAME"] == "France", "geometry"]
    ph_wgs84 = world_wgs84.loc[world_wgs84["NAME"] == "Philippines", "geometry"]
    if fr_wgs84.empty or ph_wgs84.empty:
        raise RuntimeError("France or Philippines geometry missing in Natural Earth dataset.")

    fr_3857 = fr_wgs84.to_crs(epsg=3857)
    ph_3857 = ph_wgs84.to_crs(epsg=3857)

    neuvic_xy, sourzac_xy, manila_xy = _project_points([NEUVIC, SOURZAC, MANILA])
    route = _bezier_curve(neuvic_xy, manila_xy)

    fig, ax = plt.subplots(figsize=config.figsize)
    fig.patch.set_facecolor(config.bg_color)
    ax.set_facecolor(config.ocean_color)

    world_bg.plot(ax=ax, color=config.land_color, edgecolor=config.land_edge, linewidth=0.45, zorder=1)
    fr_3857.plot(ax=ax, color=config.france_color, edgecolor="#9f9370", linewidth=0.65, zorder=2)
    ph_3857.plot(ax=ax, color=config.philippines_color, edgecolor="#9f7e38", linewidth=0.65, zorder=2)

    gpd.GeoSeries([route], crs="EPSG:3857").plot(ax=ax, color=config.route_glow, linewidth=6.5, alpha=0.28, zorder=4)
    gpd.GeoSeries([route], crs="EPSG:3857").plot(ax=ax, color=config.route_color, linewidth=2.2, alpha=0.95, zorder=5)

    ax.scatter(*neuvic_xy, s=64, color=config.route_color, edgecolor="#f6f1df", linewidth=1.2, zorder=6)
    ax.scatter(*sourzac_xy, s=48, color=config.route_color, edgecolor="#f6f1df", linewidth=1.0, zorder=6)
    ax.scatter(*manila_xy, s=74, color="#ff9418", edgecolor="#f7f0da", linewidth=1.4, zorder=6)

    ax.text(
        neuvic_xy[0] + 1.8e5,
        neuvic_xy[1] + 2.0e5,
        "Neuvic, Dordogne",
        fontsize=12,
        weight="bold",
        color="#1c232d",
        zorder=7,
    )
    ax.text(
        sourzac_xy[0] + 1.4e5,
        sourzac_xy[1] - 1.9e5,
        "Sourzac",
        fontsize=10,
        color="#463e1a",
        zorder=7,
    )
    ax.text(
        manila_xy[0] + 2.3e5,
        manila_xy[1] - 1.2e5,
        "Manila",
        fontsize=12,
        weight="bold",
        color="#1c232d",
        zorder=7,
    )

    ax.text(
        0.02,
        0.965,
        "Caviar de Neuvic Producer Route",
        transform=ax.transAxes,
        fontsize=24,
        fontstyle="italic",
        color="#1c232d",
        ha="left",
        va="top",
    )
    ax.text(
        0.02,
        0.928,
        "From Neuvic and Sourzac (Perigord, France) to Manila",
        transform=ax.transAxes,
        fontsize=13,
        color="#4e4532",
        ha="left",
        va="top",
    )

    # World extent with extra breathing room.
    # Lock map window to a stable Mercator frame to avoid Antarctica distortion.
    minx, miny, maxx, maxy = _project_bbox(-180.0, -62.0, 180.0, 84.0)
    pad_x = (maxx - minx) * 0.02
    pad_y = (maxy - miny) * 0.02
    ax.set_xlim(minx - pad_x, maxx + pad_x)
    ax.set_ylim(miny - pad_y, maxy + pad_y)
    ax.set_axis_off()

    # Inset: metropolitan France context for producer locations.
    metro = _metropolitan_france(fr_wgs84)
    metro_3857 = metro.to_crs(epsg=3857)
    inset = fig.add_axes([0.68, 0.10, 0.27, 0.30])
    inset.set_facecolor("#f4efe2")
    metro_3857.plot(ax=inset, color="#dfd6b8", edgecolor="#a49672", linewidth=0.9, zorder=2)
    inset.scatter(*neuvic_xy, s=34, color=config.route_color, edgecolor="#f6f1df", linewidth=0.9, zorder=3)
    inset.scatter(*sourzac_xy, s=26, color="#6a5933", edgecolor="#f6f1df", linewidth=0.8, zorder=3)
    inset.text(neuvic_xy[0] + 18000, neuvic_xy[1] + 12000, "Neuvic", fontsize=8.8, color="#1c232d", zorder=4)

    iminx, iminy, imaxx, imaxy = metro_3857.total_bounds
    ipadx = (imaxx - iminx) * 0.08
    ipady = (imaxy - iminy) * 0.08
    inset.set_xlim(iminx - ipadx, imaxx + ipadx)
    inset.set_ylim(iminy - ipady, imaxy + ipady)
    inset.set_xticks([])
    inset.set_yticks([])
    for spine in inset.spines.values():
        spine.set_edgecolor("#cfc7b0")
        spine.set_linewidth(0.8)
    inset.set_title("Perigord Focus", fontsize=10, color="#3f3728", pad=6)

    ax.text(
        0.995,
        0.018,
        "Data: Natural Earth (public domain) | Styling and routing by Bow Tie Duck",
        transform=ax.transAxes,
        fontsize=9,
        color="#7a7258",
        ha="right",
        va="bottom",
    )

    config.output_svg.parent.mkdir(parents=True, exist_ok=True)
    fig.savefig(config.output_svg, format="svg", dpi=180, bbox_inches="tight", pad_inches=0.02)
    plt.close(fig)
    return config.output_svg


def main() -> None:
    out = generate_map(CONFIG)
    print(out)


if __name__ == "__main__":
    main()
