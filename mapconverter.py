import fiona
from shapely.geometry import shape
import numpy as np
from tqdm import tqdm
import zipfile
from io import BytesIO
import requests
import argparse
import os
import glob

def convertLinestring(linestring):
    outlist = []

    pointx = linestring.coords.xy[0]
    pointy = linestring.coords.xy[1]

    for j in range(len(pointx)):
        outlist.extend([float(pointx[j]),float(pointy[j])])

    outlist.extend([0,0])
    return outlist

def extractLines(shapefile, tolerance):
    print("Extracting map lines")
    outlist = []

    for i in tqdm(range(len(shapefile))):
        if(tolerance > 0):
            simplified = shape(shapefile[i]['geometry']).simplify(tolerance, preserve_topology=False)
        else:
            simplified = shape(shapefile[i]['geometry'])

        if(simplified.geom_type == "LineString"):
            outlist.extend(convertLinestring(simplified))

        elif(simplified.geom_type == "MultiPolygon" or simplified.geom_type == "Polygon"):
            if(simplified.boundary.geom_type == "MultiLineString"):
                for boundary in simplified.boundary.geoms:
                    outlist.extend(convertLinestring(boundary))
            else:
                outlist.extend(convertLinestring(simplified.boundary))

        else:
            print("Unsupported type: " + simplified.geom_type)

    return outlist

def find_shapefile(pattern):
    """Find a shapefile that matches a pattern"""
    matches = glob.glob(f"mapdata/*{pattern}*.shp")
    if matches:
        return matches[0]
    return None

parser = argparse.ArgumentParser(description='viz1090 Natural Earth Data Map Converter')
parser.add_argument("--mapfile", type=str, help="shapefile for main map")
parser.add_argument("--mapnames", type=str, help="shapefile for map place names")
parser.add_argument("--airportfile", type=str, help="shapefile for airport runway outlines")
parser.add_argument("--airportnames", type=str, help="shapefile for airport IATA names")
parser.add_argument("--minpop", default=100000, type=int, help="minimum population for place names")
parser.add_argument("--tolerance", default=0.001, type=float, help="map simplification tolerance")

args = parser.parse_args()

# If specific files weren't provided, try to find them by pattern
if not args.mapfile:
    args.mapfile = find_shapefile("states")
    if args.mapfile:
        print(f"Found map file: {args.mapfile}")

if not args.mapnames:
    args.mapnames = find_shapefile("populated")
    if args.mapnames:
        print(f"Found place names file: {args.mapnames}")

if not args.airportfile:
    args.airportfile = find_shapefile("runway")
    if not args.airportfile:
        args.airportfile = "mapdata/Runways.shp"
    print(f"Using airport file: {args.airportfile}")

if not args.airportnames:
    args.airportnames = find_shapefile("airport")
    if args.airportnames:
        print(f"Found airport names file: {args.airportnames}")

# mapfile
if args.mapfile and os.path.exists(args.mapfile):
    shapefile = fiona.open(args.mapfile)

    outlist = extractLines(shapefile, args.tolerance)

    bin_file = open("mapdata.bin", "wb")
    np.asarray(outlist).astype(np.single).tofile(bin_file)
    bin_file.close()

    print("Wrote %d points" % (len(outlist) / 2))
else:
    print(f"Map file not found: {args.mapfile}")

# mapnames
bin_file = open("mapnames", "w")

if args.mapnames and os.path.exists(args.mapnames):
    shapefile = fiona.open(args.mapnames)

    count = 0

    for i in tqdm(range(len(shapefile))):
        try:
            xcoord = shapefile[i]['geometry']['coordinates'][0]
            ycoord = shapefile[i]['geometry']['coordinates'][1]
            pop = shapefile[i]['properties'].get('POP_MIN', 0)
            name = shapefile[i]['properties'].get('NAME', '')

            if pop > args.minpop:
                outstring = "{0} {1} {2}\n".format(xcoord, ycoord, name)
                bin_file.write(outstring)
                count = count + 1
        except (KeyError, TypeError) as e:
            continue

    bin_file.close()

    print("Wrote %d place names" % count)
else:
    print(f"Place names file not found: {args.mapnames}")
    bin_file.close()

#airportfile
if args.airportfile and os.path.exists(args.airportfile):
    try:
        shapefile = fiona.open(args.airportfile)

        outlist = extractLines(shapefile, 0)

        bin_file = open("airportdata.bin", "wb")
        np.asarray(outlist).astype(np.single).tofile(bin_file)
        bin_file.close()

        print("Wrote %d points" % (len(outlist) / 2))
    except Exception as e:
        print(f"Error reading airport file: {e}")
else:
    print(f"Airport file not found: {args.airportfile}")

#airportnames
if args.airportnames and os.path.exists(args.airportnames):
    bin_file = open("airportnames", "w")

    try:
        shapefile = fiona.open(args.airportnames)

        count = 0

        for i in tqdm(range(len(shapefile))):
            try:
                xcoord = shapefile[i]['geometry']['coordinates'][0]
                ycoord = shapefile[i]['geometry']['coordinates'][1]
                name = shapefile[i]['properties'].get('iata_code', '')

                if not name:
                    name = shapefile[i]['properties'].get('IATA', '')

                if not name:
                    # Try other possible property names for the IATA code
                    for key in ['IATA_CODE', 'iata', 'code', 'CODE']:
                        if key in shapefile[i]['properties']:
                            name = shapefile[i]['properties'][key]
                            if name:
                                break

                if name:
                    outstring = "{0} {1} {2}\n".format(xcoord, ycoord, name)
                    bin_file.write(outstring)
                    count = count + 1
            except (KeyError, TypeError) as e:
                continue

        bin_file.close()

        print("Wrote %d airport names" % count)
    except Exception as e:
        print(f"Error reading airport names file: {e}")
        bin_file.close()
else:
    print(f"Airport names file not found: {args.airportnames}")
    open("airportnames", "w").close()  # Create empty file