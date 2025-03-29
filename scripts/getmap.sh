#!/bin/bash

# Create directories for map data
mkdir -p mapdata
pushd mapdata > /dev/null

# Download map data files
echo "Downloading map data files..."
curl  https://naciscdn.org/naturalearth/10m/cultural/ne_10m_admin_1_states_provinces.zip -o ne_10m_admin_1_states_provinces.zip
curl  https://naciscdn.org/naturalearth/10m/cultural/ne_10m_populated_places.zip -o ne_10m_populated_places.zip
curl  https://naciscdn.org/naturalearth/10m/cultural/ne_10m_airports.zip -o ne_10m_airports.zip

# Download runway data
echo "Downloading runway data..."
# For airports and runways, we need to make sure we get the right file names
# The OpenData structure may change, so let's use a more reliable direct link
curl  "https://opendata.arcgis.com/api/v3/datasets/4d8fa46181aa470d809776c57a8ab1f6_0/downloads/data?format=shp&spatialRefId=4326" -O runways.zip

# Extract files
echo "Extracting files..."
for file in *.zip; do
    unzip -o -q "${file}"
    # Don't remove zip files yet
done

# Check for the runway file and rename if needed
find . -name "*.shp" | grep -i runway > /dev/null
if [ $? -ne 0 ]; then
    # If we can't find a runway file, look for any possible candidate
    RUNWAY_FILE=$(find . -name "*.shp" | head -1)
    if [ -n "$RUNWAY_FILE" ]; then
        echo "Renaming $RUNWAY_FILE to Runways.shp"
        cp "$RUNWAY_FILE" Runways.shp
    fi
fi

# Now we can clean up zip files
rm -f *.zip

popd > /dev/null

# Convert map data
echo "Converting map data..."
python3 mapconverter.py \
    --mapfile mapdata/ne_10m_admin_1_states_provinces.shp \
    --mapnames mapdata/ne_10m_populated_places.shp \
    --airportfile mapdata/Runways.shp \
    --airportnames mapdata/ne_10m_airports.shp \
    --minpop 100000 \
    --tolerance 0.001

echo "Map data processing complete."