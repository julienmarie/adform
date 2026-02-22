#!/usr/bin/env python3
from pathlib import Path

import geopandas as gpd
import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt


OUTPUT = Path("dist/france_map.png")
WORLD_URL = "https://naturalearth.s3.amazonaws.com/10m_cultural/ne_10m_admin_0_countries.zip"


def main() -> None:
    world = gpd.read_file(WORLD_URL)
    france_row = world.loc[world["ADMIN"] == "France"]
    if france_row.empty:
        raise RuntimeError("France boundary not found in Natural Earth dataset.")

    france_geom = france_row.geometry.iloc[0]
    france = gpd.GeoDataFrame(geometry=[france_geom], crs=world.crs).explode(index_parts=False)

    # Keep metropolitan France + Corsica by filtering for polygons in European France bounds.
    centroids = france.to_crs(3857).geometry.centroid
    centroids = gpd.GeoSeries(centroids, crs=3857).to_crs(4326)
    metro = france.loc[
        centroids.x.between(-6.5, 10.0) & centroids.y.between(41.0, 52.5)
    ]
    if metro.empty:
        raise RuntimeError("Could not isolate metropolitan France geometry.")

    minx, miny, maxx, maxy = metro.total_bounds
    padx = (maxx - minx) * 0.08
    pady = (maxy - miny) * 0.08

    fig, ax = plt.subplots(figsize=(8, 8))
    ax.set_facecolor("#f7f9fc")
    metro.plot(ax=ax, color="#d7e8ff", edgecolor="#0f294d", linewidth=1.0)
    ax.set_xlim(minx - padx, maxx + padx)
    ax.set_ylim(miny - pady, maxy + pady)
    ax.set_axis_off()
    fig.tight_layout()

    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    fig.savefig(OUTPUT, dpi=300, bbox_inches="tight", pad_inches=0.02)
    plt.close(fig)
    print(str(OUTPUT))


if __name__ == "__main__":
    main()
