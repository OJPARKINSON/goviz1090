package map_system

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
)

// Constants for the map system
const (
	LATLONMULT = 111.195 // 6371.0 * M_PI / 180.0 - conversion factor for lat/lon to km
)

// Point represents a geographic point with latitude and longitude
type Point struct {
	Lat float64
	Lon float64
}

// Line represents a map line segment
type Line struct {
	Start Point
	End   Point
	// Bounding box for quick intersection tests
	LatMin float64
	LatMax float64
	LonMin float64
	LonMax float64
}

// MapLabel represents a geographic label (city, airport, etc.)
type MapLabel struct {
	Location Point
	Text     string
}

// QuadTree implements a quadtree for efficient map feature lookup
type QuadTree struct {
	LatMin float64
	LatMax float64
	LonMin float64
	LonMax float64
	Lines  []*Line
	NW     *QuadTree
	NE     *QuadTree
	SW     *QuadTree
	SE     *QuadTree
}

// Map contains all map data structures
type Map struct {
	Root         *QuadTree
	AirportRoot  *QuadTree
	MapLines     []*Line
	AirportLines []*Line
	PlaceNames   []*MapLabel
	AirportNames []*MapLabel
}

// NewMap creates a new map instance
func NewMap() *Map {
	return &Map{
		Root:         &QuadTree{LatMin: 180.0, LatMax: -180.0, LonMin: 180.0, LonMax: -180.0},
		AirportRoot:  &QuadTree{LatMin: 180.0, LatMax: -180.0, LonMin: 180.0, LonMax: -180.0},
		MapLines:     make([]*Line, 0),
		AirportLines: make([]*Line, 0),
		PlaceNames:   make([]*MapLabel, 0),
		AirportNames: make([]*MapLabel, 0),
	}
}

// LoadMapData loads map data from binary and text files
func (m *Map) LoadMapData(mapDataFile, airportDataFile, placeNamesFile, airportNamesFile string) error {
	// Load map geometry
	if err := m.loadMapGeometry(mapDataFile, &m.Root, &m.MapLines); err != nil {
		fmt.Printf("Warning: Failed to load map data: %v\n", err)
	}

	// Load airport geometry
	if err := m.loadMapGeometry(airportDataFile, &m.AirportRoot, &m.AirportLines); err != nil {
		fmt.Printf("Warning: Failed to load airport data: %v\n", err)
	}

	// Load place names
	if err := m.loadLabels(placeNamesFile, &m.PlaceNames); err != nil {
		fmt.Printf("Warning: Failed to load place names: %v\n", err)
	}

	// Load airport names
	if err := m.loadLabels(airportNamesFile, &m.AirportNames); err != nil {
		fmt.Printf("Warning: Failed to load airport names: %v\n", err)
	}

	return nil
}

// loadMapGeometry loads map line geometry from a binary file
func (m *Map) loadMapGeometry(filename string, root **QuadTree, lines *[]*Line) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	// Read the entire file
	data, err := io.ReadAll(file)
	if err != nil {
		return err
	}

	// Each point is 4 bytes (float32)
	numFloats := len(data) / 4
	points := make([]float32, numFloats)

	// Convert byte array to float32 array
	for i := 0; i < numFloats; i++ {
		points[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4 : (i+1)*4]))
	}

	// Update quadtree bounds by examining all points
	for i := 0; i < numFloats; i += 2 {
		if points[i] == 0 {
			continue
		}

		lon := float64(points[i])
		lat := float64(points[i+1])

		if lon < (*root).LonMin {
			(*root).LonMin = lon
		} else if lon > (*root).LonMax {
			(*root).LonMax = lon
		}

		if lat < (*root).LatMin {
			(*root).LatMin = lat
		} else if lat > (*root).LatMax {
			(*root).LatMax = lat
		}
	}

	// Create lines from points
	for i := 0; i < numFloats-2; i += 2 {
		if points[i] == 0 || points[i+1] == 0 || points[i+2] == 0 || points[i+3] == 0 {
			continue
		}

		startPoint := Point{Lon: float64(points[i]), Lat: float64(points[i+1])}
		endPoint := Point{Lon: float64(points[i+2]), Lat: float64(points[i+3])}

		line := &Line{
			Start:  startPoint,
			End:    endPoint,
			LatMin: math.Min(startPoint.Lat, endPoint.Lat),
			LatMax: math.Max(startPoint.Lat, endPoint.Lat),
			LonMin: math.Min(startPoint.Lon, endPoint.Lon),
			LonMax: math.Max(startPoint.Lon, endPoint.Lon),
		}

		*lines = append(*lines, line)
		m.insertIntoQuadTree(*root, line, 0)
	}

	return nil
}

// loadLabels loads text labels from a file
func (m *Map) loadLabels(filename string, labels *[]*MapLabel) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		lon, err := strconv.ParseFloat(fields[0], 64)
		if err != nil {
			continue
		}

		lat, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			continue
		}

		// Combine remaining fields as the label text
		text := strings.Join(fields[2:], " ")

		*labels = append(*labels, &MapLabel{
			Location: Point{Lon: lon, Lat: lat},
			Text:     text,
		})
	}

	return scanner.Err()
}

// insertIntoQuadTree inserts a line into the quadtree
func (m *Map) insertIntoQuadTree(tree *QuadTree, line *Line, depth int) bool {
	// Check if line intersects with this quad
	startInside := line.Start.Lat >= tree.LatMin &&
		line.Start.Lat <= tree.LatMax &&
		line.Start.Lon >= tree.LonMin &&
		line.Start.Lon <= tree.LonMax

	endInside := line.End.Lat >= tree.LatMin &&
		line.End.Lat <= tree.LatMax &&
		line.End.Lon >= tree.LonMin &&
		line.End.Lon <= tree.LonMax

	// If neither end is inside, line may still cross the quad, but for simplicity we'll skip it
	if !startInside && !endInside {
		return false
	}

	// If only one end is inside, add to this node
	if startInside != endInside {
		tree.Lines = append(tree.Lines, line)
		return true
	}

	// If we're at a very deep level, just add it here
	if depth > 25 {
		tree.Lines = append(tree.Lines, line)
		return true
	}

	// Otherwise, try to add to a child
	inserted := false

	// Create child nodes if they don't exist
	if tree.NW == nil {
		midLat := tree.LatMin + 0.5*(tree.LatMax-tree.LatMin)
		midLon := tree.LonMin + 0.5*(tree.LonMax-tree.LonMin)

		tree.NW = &QuadTree{
			LatMin: tree.LatMin,
			LatMax: midLat,
			LonMin: tree.LonMin,
			LonMax: midLon,
		}

		tree.NE = &QuadTree{
			LatMin: tree.LatMin,
			LatMax: midLat,
			LonMin: midLon,
			LonMax: tree.LonMax,
		}

		tree.SW = &QuadTree{
			LatMin: midLat,
			LatMax: tree.LatMax,
			LonMin: tree.LonMin,
			LonMax: midLon,
		}

		tree.SE = &QuadTree{
			LatMin: midLat,
			LatMax: tree.LatMax,
			LonMin: midLon,
			LonMax: tree.LonMax,
		}
	}

	// Try to insert into child nodes
	if m.insertIntoQuadTree(tree.NW, line, depth+1) {
		inserted = true
	} else if m.insertIntoQuadTree(tree.NE, line, depth+1) {
		inserted = true
	} else if m.insertIntoQuadTree(tree.SW, line, depth+1) {
		inserted = true
	} else if m.insertIntoQuadTree(tree.SE, line, depth+1) {
		inserted = true
	}

	// If we couldn't insert into any child, add it to this node
	if !inserted {
		tree.Lines = append(tree.Lines, line)
	}

	return true
}

// GetVisibleLines returns all lines visible in the specified geographic area
func (m *Map) GetVisibleLines(latMin, latMax, lonMin, lonMax float64) ([]*Line, []*Line) {
	// Get map features
	mapLines := m.getLinesFromQuadTree(m.Root, latMin, latMax, lonMin, lonMax)

	// Get airport features
	airportLines := m.getLinesFromQuadTree(m.AirportRoot, latMin, latMax, lonMin, lonMax)

	return mapLines, airportLines
}

// getLinesFromQuadTree recursively gets lines from the quadtree that are visible in the specified area
func (m *Map) getLinesFromQuadTree(tree *QuadTree, latMin, latMax, lonMin, lonMax float64) []*Line {
	if tree == nil {
		return nil
	}

	// If this quad doesn't overlap with the visible area, return nothing
	if tree.LatMax < latMin || tree.LatMin > latMax || tree.LonMax < lonMin || tree.LonMin > lonMax {
		return nil
	}

	// Start with lines in this node
	lines := make([]*Line, len(tree.Lines))
	copy(lines, tree.Lines)

	// Add lines from children
	if tree.NW != nil {
		lines = append(lines, m.getLinesFromQuadTree(tree.NW, latMin, latMax, lonMin, lonMax)...)
		lines = append(lines, m.getLinesFromQuadTree(tree.NE, latMin, latMax, lonMin, lonMax)...)
		lines = append(lines, m.getLinesFromQuadTree(tree.SW, latMin, latMax, lonMin, lonMax)...)
		lines = append(lines, m.getLinesFromQuadTree(tree.SE, latMin, latMax, lonMin, lonMax)...)
	}

	return lines
}

// GetVisibleLabels returns all labels visible in the specified geographic area
func (m *Map) GetVisibleLabels(latMin, latMax, lonMin, lonMax float64) ([]*MapLabel, []*MapLabel) {
	var visiblePlaces []*MapLabel
	var visibleAirports []*MapLabel

	// Check place names
	for _, label := range m.PlaceNames {
		if label.Location.Lat >= latMin && label.Location.Lat <= latMax &&
			label.Location.Lon >= lonMin && label.Location.Lon <= lonMax {
			visiblePlaces = append(visiblePlaces, label)
		}
	}

	// Check airport names
	for _, label := range m.AirportNames {
		if label.Location.Lat >= latMin && label.Location.Lat <= latMax &&
			label.Location.Lon >= lonMin && label.Location.Lon <= lonMax {
			visibleAirports = append(visibleAirports, label)
		}
	}

	return visiblePlaces, visibleAirports
}
