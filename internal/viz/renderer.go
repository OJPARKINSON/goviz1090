package viz

import (
	"fmt"
	"math"
	"time"

	"github.com/veandco/go-sdl2/sdl"
	"github.com/veandco/go-sdl2/ttf"

	"github.com/OJPARKINSON/viz1090/internal/adsb"
	"github.com/OJPARKINSON/viz1090/internal/map_system"
)

// Constants for visualization
const (
	LATLONMULT   = 111.195 // 6371.0 * M_PI / 180.0 - conversion factor for lat/lon to km
	PAD          = 5       // Padding for UI elements
	ROUND_RADIUS = 3       // Radius of rounded corners
)

// Color definitions
var (
	ColorBackground = sdl.Color{R: 0, G: 0, B: 0, A: 255}
	ColorPlane      = sdl.Color{R: 253, G: 250, B: 31, A: 255}
	ColorPlaneGone  = sdl.Color{R: 127, G: 127, B: 127, A: 255}
	ColorSelected   = sdl.Color{R: 249, G: 38, B: 114, A: 255}
	ColorTrail      = sdl.Color{R: 90, G: 133, B: 50, A: 255}
	ColorLabel      = sdl.Color{R: 255, G: 255, B: 255, A: 255}
	ColorSubLabel   = sdl.Color{R: 127, G: 127, B: 127, A: 255}
	ColorScaleBar   = sdl.Color{R: 196, G: 196, B: 196, A: 255}
	ColorLabelLine  = sdl.Color{R: 64, G: 64, B: 64, A: 255}
	ColorLabelBg    = sdl.Color{R: 0, G: 0, B: 0, A: 200}
	ColorMap        = sdl.Color{R: 33, G: 0, B: 122, A: 255}
	ColorAirport    = sdl.Color{R: 85, G: 0, B: 255, A: 255}
	ColorText       = sdl.Color{R: 196, G: 196, B: 196, A: 255}
	ColorButton     = sdl.Color{R: 196, G: 196, B: 196, A: 255}
	ColorButtonBg   = sdl.Color{R: 0, G: 0, B: 0, A: 255}
)

// Renderer handles drawing the radar display
type Renderer struct {
	renderer    *sdl.Renderer
	regularFont *ttf.Font
	boldFont    *ttf.Font
	labelFont   *ttf.Font
	messageFont *ttf.Font // Added missing font for status box
	width       int
	height      int
	uiScale     int
	metric      bool
	mapTexture  *sdl.Texture
	lastRedraw  time.Time
	mapDrawn    bool
	mapSystem   *map_system.Map
	labelSystem *LabelSystem
}

// NewRenderer creates a new visualization renderer
func NewRenderer(renderer *sdl.Renderer, regularFont, boldFont *ttf.Font, width, height, uiScale int, metric bool) *Renderer {
	// Create the map texture for caching
	mapTexture, err := renderer.CreateTexture(
		sdl.PIXELFORMAT_RGBA8888,
		sdl.TEXTUREACCESS_TARGET,
		int32(width), int32(height))

	if err != nil {
		fmt.Printf("Warning: Failed to create map texture: %v\n", err)
	}

	// Create a label font specifically for aircraft labels
	labelFont, err := ttf.OpenFont("font/TerminusTTF-Bold-4.46.0.ttf", 12*uiScale)
	if err != nil {
		fmt.Printf("Warning: Failed to load label font: %v\n", err)
		labelFont = regularFont // Fallback to regular font
	}

	// Create message font (same as regular font, used for status box)
	messageFont := regularFont

	// Create map system
	mapSystem := map_system.NewMap()
	_ = mapSystem.LoadMapData("mapdata.bin", "airportdata.bin", "mapnames", "airportnames")

	// Create label system
	labelSystem := NewLabelSystem(width, height, uiScale, metric)

	return &Renderer{
		renderer:    renderer,
		regularFont: regularFont,
		boldFont:    boldFont,
		labelFont:   labelFont,
		messageFont: messageFont,
		width:       width,
		height:      height,
		uiScale:     uiScale,
		metric:      metric,
		mapTexture:  mapTexture,
		mapSystem:   mapSystem,
		labelSystem: labelSystem,
	}
}

// GetHeight returns the renderer height
func (r *Renderer) GetHeight() int {
	return r.height
}

// RenderFrame draws a complete frame with all aircraft
func (r *Renderer) RenderFrame(aircraft map[uint32]*adsb.Aircraft, centerLat, centerLon, maxDistance float64, selectedICAO uint32) {
	// Clear screen
	r.renderer.SetDrawColor(ColorBackground.R, ColorBackground.G, ColorBackground.B, ColorBackground.A)
	r.renderer.Clear()

	// Calculate screen positions for all aircraft
	r.calculateScreenPositions(aircraft, centerLat, centerLon, maxDistance)

	// Update label positions to avoid overlaps
	r.labelSystem.UpdateLabels(aircraft)

	// Draw map if needed
	if !r.mapDrawn || time.Since(r.lastRedraw) > 2*time.Second {
		r.drawMap(centerLat, centerLon, maxDistance)
	}

	// Copy map from texture to screen
	r.renderer.Copy(r.mapTexture, nil, nil)

	// Draw aircraft trails
	r.drawTrails(aircraft, centerLat, centerLon, maxDistance)

	// Draw all aircraft
	r.drawAircraft(aircraft, selectedICAO)

	// Draw scale bar
	r.drawScaleBars(maxDistance)

	// Draw status information
	r.drawStatus(len(aircraft), r.countVisibleAircraft(aircraft), centerLat, centerLon)

	// Present the renderer
	r.renderer.Present()
}

// calculateScreenPositions calculates screen coordinates for all aircraft
func (r *Renderer) calculateScreenPositions(aircraft map[uint32]*adsb.Aircraft, centerLat, centerLon, maxDistance float64) {
	for _, a := range aircraft {
		if a.Lat == 0 && a.Lon == 0 {
			continue // Skip aircraft without position
		}

		// Calculate screen position
		x, y := r.latLonToScreen(a.Lat, a.Lon, centerLat, centerLon, maxDistance)
		a.X = x
		a.Y = y
	}
}

// countVisibleAircraft counts aircraft with valid positions
func (r *Renderer) countVisibleAircraft(aircraft map[uint32]*adsb.Aircraft) int {
	count := 0
	for _, a := range aircraft {
		if a.Lat != 0 && a.Lon != 0 {
			count++
		}
	}
	return count
}

// drawMap renders the geographic map to a texture
func (r *Renderer) drawMap(centerLat, centerLon, maxDistance float64) {
	// Save original render target
	original, _ := r.renderer.GetRenderTarget()

	// Set map texture as render target
	r.renderer.SetRenderTarget(r.mapTexture)

	// Clear map texture
	r.renderer.SetDrawColor(ColorBackground.R, ColorBackground.G, ColorBackground.B, ColorBackground.A)
	r.renderer.Clear()

	// Calculate visible area bounds
	// Convert screen edges to lat/lon
	var latMin, latMax, lonMin, lonMax float64

	// Bottom-left corner
	latLonFromScreenCoords(&latMin, &lonMin, 0, r.height, centerLat, centerLon, maxDistance, r.width, r.height)

	// Top-right corner
	latLonFromScreenCoords(&latMax, &lonMax, r.width, 0, centerLat, centerLon, maxDistance, r.width, r.height)

	// Get visible map features
	mapLines, airportLines := r.mapSystem.GetVisibleLines(latMin, latMax, lonMin, lonMax)

	// Draw map lines
	r.renderer.SetDrawColor(ColorMap.R, ColorMap.G, ColorMap.B, ColorMap.A)
	for _, line := range mapLines {
		x1, y1 := r.latLonToScreen(line.Start.Lat, line.Start.Lon, centerLat, centerLon, maxDistance)
		x2, y2 := r.latLonToScreen(line.End.Lat, line.End.Lon, centerLat, centerLon, maxDistance)

		// Skip if both endpoints are outside screen
		if (x1 < 0 || x1 > r.width || y1 < 0 || y1 > r.height) &&
			(x2 < 0 || x2 > r.width || y2 < 0 || y2 > r.height) {
			continue
		}

		r.renderer.DrawLine(int32(x1), int32(y1), int32(x2), int32(y2))
	}

	// Draw airport lines
	r.renderer.SetDrawColor(ColorAirport.R, ColorAirport.G, ColorAirport.B, ColorAirport.A)
	for _, line := range airportLines {
		x1, y1 := r.latLonToScreen(line.Start.Lat, line.Start.Lon, centerLat, centerLon, maxDistance)
		x2, y2 := r.latLonToScreen(line.End.Lat, line.End.Lon, centerLat, centerLon, maxDistance)

		// Skip if both endpoints are outside screen
		if (x1 < 0 || x1 > r.width || y1 < 0 || y1 > r.height) &&
			(x2 < 0 || x2 > r.width || y2 < 0 || y2 > r.height) {
			continue
		}

		r.renderer.DrawLine(int32(x1), int32(y1), int32(x2), int32(y2))
	}

	// Get visible labels
	placeLabels, airportLabels := r.mapSystem.GetVisibleLabels(latMin, latMax, lonMin, lonMax)

	// Draw place labels
	for _, label := range placeLabels {
		x, y := r.latLonToScreen(label.Location.Lat, label.Location.Lon, centerLat, centerLon, maxDistance)

		// Skip if outside screen
		if x < 0 || x > r.width || y < 0 || y > r.height {
			continue
		}

		r.drawText(label.Text, x, y, r.regularFont, ColorText)
	}

	// Draw airport labels
	for _, label := range airportLabels {
		x, y := r.latLonToScreen(label.Location.Lat, label.Location.Lon, centerLat, centerLon, maxDistance)

		// Skip if outside screen
		if x < 0 || x > r.width || y < 0 || y > r.height {
			continue
		}

		r.drawText(label.Text, x, y, r.boldFont, ColorText)
	}

	// Restore original render target
	r.renderer.SetRenderTarget(original)

	r.mapDrawn = true
	r.lastRedraw = time.Now()
}

// drawAircraft renders all aircraft symbols and labels
func (r *Renderer) drawAircraft(aircraft map[uint32]*adsb.Aircraft, selectedICAO uint32) {
	for icao, a := range aircraft {
		if a.X == 0 && a.Y == 0 {
			continue // Skip aircraft without position
		}

		// Determine color based on selection and age
		color := ColorPlane
		if icao == selectedICAO {
			color = ColorSelected
		} else if time.Since(a.Seen).Seconds() > 15 {
			// Fade color the longer we haven't seen the aircraft
			fade := math.Min(1.0, (time.Since(a.Seen).Seconds()-15.0)/15.0)
			color = lerpColor(ColorPlane, ColorPlaneGone, fade)
		}

		// Draw aircraft symbol
		r.drawAircraftSymbol(a.X, a.Y, a.Heading, color, icao == selectedICAO)

		// Draw label if it should be visible
		if a.LabelOpacity > 0 {
			r.drawAircraftLabel(a, color)
		}
	}
}

// drawAircraftSymbol draws an aircraft symbol at the specified position
func (r *Renderer) drawAircraftSymbol(x, y, heading int, color sdl.Color, selected bool) {
	// Convert heading to radians
	headingRad := float64(heading) * math.Pi / 180.0

	// Scale factors for plane size
	bodyLen := float64(8 * r.uiScale)
	wingLen := float64(6 * r.uiScale)
	tailLen := float64(3 * r.uiScale)

	// Calculate direction vectors
	dirX := math.Sin(headingRad)
	dirY := -math.Cos(headingRad)

	// Calculate perpendicular vectors (for wings)
	perpX := -dirY
	perpY := dirX

	// Compute points
	noseX := float64(x) + dirX*bodyLen
	noseY := float64(y) + dirY*bodyLen
	tailX := float64(x) - dirX*bodyLen*0.75
	tailY := float64(y) - dirY*bodyLen*0.75
	leftWingX := float64(x) + perpX*wingLen
	leftWingY := float64(y) + perpY*wingLen
	rightWingX := float64(x) - perpX*wingLen
	rightWingY := float64(y) - perpY*wingLen
	leftTailX := tailX + perpX*tailLen
	leftTailY := tailY + perpY*tailLen
	rightTailX := tailX - perpX*tailLen
	rightTailY := tailY - perpY*tailLen

	// Draw aircraft
	r.renderer.SetDrawColor(color.R, color.G, color.B, color.A)

	// Body
	r.renderer.DrawLine(int32(x), int32(y), int32(noseX), int32(noseY))
	r.renderer.DrawLine(int32(x), int32(y), int32(tailX), int32(tailY))

	// Wings
	r.renderer.DrawLine(int32(x), int32(y), int32(leftWingX), int32(leftWingY))
	r.renderer.DrawLine(int32(x), int32(y), int32(rightWingX), int32(rightWingY))

	// Tail
	r.renderer.DrawLine(int32(tailX), int32(tailY), int32(leftTailX), int32(leftTailY))
	r.renderer.DrawLine(int32(tailX), int32(tailY), int32(rightTailX), int32(rightTailY))

	// Draw selection box if selected
	if selected {
		boxSize := int32(20 * r.uiScale)
		r.renderer.DrawRect(&sdl.Rect{
			X: int32(x) - boxSize/2,
			Y: int32(y) - boxSize/2,
			W: boxSize,
			H: boxSize,
		})
	}
}

// drawAircraftLabel draws a label for the specified aircraft
func (r *Renderer) drawAircraftLabel(a *adsb.Aircraft, color sdl.Color) {
	// Don't draw if opacity is 0
	if a.LabelOpacity <= 0 {
		return
	}

	// Set alpha based on label opacity
	alpha := uint8(255.0 * a.LabelOpacity)

	// Label background
	bgColor := ColorLabelBg
	bgColor.A = alpha

	// Label outline
	lineColor := ColorLabelLine
	lineColor.A = alpha

	// Label text colors
	textColor := ColorLabel
	textColor.A = alpha

	subTextColor := ColorSubLabel
	subTextColor.A = alpha

	// Draw background
	r.renderer.SetDrawColor(bgColor.R, bgColor.G, bgColor.B, bgColor.A)
	r.renderer.FillRect(&sdl.Rect{
		X: int32(a.LabelX),
		Y: int32(a.LabelY),
		W: int32(a.LabelW),
		H: int32(a.LabelH),
	})

	// Draw connecting line from aircraft to label
	anchorX := int32(a.LabelX)
	if a.LabelX+a.LabelW/2 > float64(a.X) {
		anchorX = int32(a.LabelX)
	} else {
		anchorX = int32(a.LabelX + a.LabelW)
	}

	anchorY := int32(a.LabelY)
	if a.LabelY+a.LabelH/2 > float64(a.Y) {
		anchorY = int32(a.LabelY - 4)
	} else {
		anchorY = int32(a.LabelY + a.LabelH + 4)
	}

	// Determine line path points (simply connect aircraft to label anchor)
	r.renderer.SetDrawColor(lineColor.R, lineColor.G, lineColor.B, lineColor.A)
	r.renderer.DrawLine(int32(a.X), int32(a.Y), anchorX, anchorY)

	// Draw label outline
	r.renderer.DrawRect(&sdl.Rect{
		X: int32(a.LabelX),
		Y: int32(a.LabelY),
		W: int32(a.LabelW),
		H: int32(a.LabelH),
	})

	// Draw callsign
	y := int(a.LabelY) + 2

	if a.LabelLevel < 2 {
		r.drawText(a.Flight, int(a.LabelX)+4, y, r.labelFont, textColor)
		y += 14 * r.uiScale
	}

	// Draw altitude and speed if detail level allows
	if a.LabelLevel < 1 {
		// Altitude
		altText := ""
		if r.metric {
			altText = fmt.Sprintf(" %dm", int(float64(a.Altitude)/3.2828))
		} else {
			altText = fmt.Sprintf(" %d'", a.Altitude)
		}
		r.drawText(altText, int(a.LabelX)+4, y, r.regularFont, subTextColor)
		y += 14 * r.uiScale

		// Speed
		speedText := ""
		if r.metric {
			speedText = fmt.Sprintf(" %dkm/h", int(float64(a.Speed)*1.852))
		} else {
			speedText = fmt.Sprintf(" %dkts", a.Speed)
		}
		r.drawText(speedText, int(a.LabelX)+4, y, r.regularFont, subTextColor)
	}
}

// drawTrails renders the trail of an aircraft's past positions
func (r *Renderer) drawTrails(aircraft map[uint32]*adsb.Aircraft, centerLat, centerLon, maxDistance float64) {
	for _, a := range aircraft {
		if len(a.Trail) < 2 {
			continue
		}

		// Draw connecting lines between trail points
		for i := 0; i < len(a.Trail)-1; i++ {
			// Calculate opacity based on age
			age := 1.0 - float64(i)/float64(len(a.Trail))
			alpha := uint8(128 * age)

			// Convert trail positions to screen coordinates
			x1, y1 := r.latLonToScreen(a.Trail[i].Lat, a.Trail[i].Lon, centerLat, centerLon, maxDistance)
			x2, y2 := r.latLonToScreen(a.Trail[i+1].Lat, a.Trail[i+1].Lon, centerLat, centerLon, maxDistance)

			// Draw trail segment
			r.renderer.SetDrawColor(ColorTrail.R, ColorTrail.G, ColorTrail.B, alpha)
			r.renderer.DrawLine(int32(x1), int32(y1), int32(x2), int32(y2))
		}
	}
}

// drawScaleBars draws distance scale indicators
func (r *Renderer) drawScaleBars(maxDistance float64) {
	// Find appropriate scale
	scalePower := 0
	for {
		dist := math.Pow10(scalePower)
		screenDist := float64(r.height/2) * (dist / maxDistance)
		if screenDist > 100 {
			break
		}
		scalePower++
	}

	// Convert distance to screen coordinates
	scaleBarDist := int(float64(r.height/2) * (math.Pow10(scalePower) / maxDistance))

	// Draw horizontal scale bar
	r.renderer.SetDrawColor(ColorScaleBar.R, ColorScaleBar.G, ColorScaleBar.B, ColorScaleBar.A)
	r.renderer.DrawLine(10, 10, 10+int32(scaleBarDist), 10)
	r.renderer.DrawLine(10, 10, 10, 20)
	r.renderer.DrawLine(10+int32(scaleBarDist), 10, 10+int32(scaleBarDist), 15)

	// Draw scale label
	scaleLabel := ""
	if r.metric {
		scaleLabel = fmt.Sprintf("%dkm", int(math.Pow10(scalePower)))
	} else {
		scaleLabel = fmt.Sprintf("%dnm", int(math.Pow10(scalePower)))
	}
	r.drawText(scaleLabel, 15+scaleBarDist, 15, r.regularFont, ColorScaleBar)
}

// drawStatus draws status information at the bottom of the screen
func (r *Renderer) drawStatus(aircraftCount, visibleCount int, centerLat, centerLon float64) {
	// Format location text
	locText := fmt.Sprintf("loc %.4fN %.4f%c", centerLat,
		math.Abs(centerLon), map[bool]byte{true: 'E', false: 'W'}[centerLon >= 0])

	// Format aircraft count text
	dispText := fmt.Sprintf("disp %d/%d", visibleCount, aircraftCount)

	// Draw status boxes
	padding := 5
	x := padding
	y := r.height - 30*r.uiScale

	// Draw the status boxes using our helper function
	r.drawStatusBox(&x, &y, "loc", locText, ColorScaleBar)
	r.drawStatusBox(&x, &y, "disp", dispText, ColorScaleBar)
}

// drawStatusBox draws a status box with label and value
func (r *Renderer) drawStatusBox(x *int, y *int, label, value string, color sdl.Color) {
	// Calculate dimensions
	labelFontWidth := 6 * r.uiScale
	messageFontWidth := 6 * r.uiScale
	messageFontHeight := 12 * r.uiScale

	labelWidth := (len(label) + 1) * labelFontWidth
	messageWidth := (len(value) + 1) * messageFontWidth

	// Wrap to next line if needed
	if *x+labelWidth+messageWidth+PAD > r.width {
		*x = PAD
		*y = *y - messageFontHeight - PAD
	}

	// Background
	if messageWidth > 0 {
		r.roundedBox(*x, *y, *x+labelWidth+messageWidth, *y+messageFontHeight, ROUND_RADIUS, ColorButtonBg)
	}

	// Label box
	if labelWidth > 0 {
		r.roundedBox(*x, *y, *x+labelWidth, *y+messageFontHeight, ROUND_RADIUS, color)
	}

	// Outline message box
	if messageWidth > 0 {
		r.roundedRect(*x, *y, *x+labelWidth+messageWidth, *y+messageFontHeight, ROUND_RADIUS, color)
	}

	// Label text (in label box)
	r.drawText(label, *x+labelFontWidth/2, *y, r.labelFont, ColorButtonBg)

	// Value text (in message box)
	r.drawText(value, *x+labelWidth+messageFontWidth/2, *y, r.messageFont, color)

	// Update x position for next box
	*x = *x + labelWidth + messageWidth + PAD
}

// roundedBox draws a filled rounded rectangle
func (r *Renderer) roundedBox(x1, y1, x2, y2, radius int, color sdl.Color) {
	r.renderer.SetDrawColor(color.R, color.G, color.B, color.A)

	// Draw filled rectangle
	r.renderer.FillRect(&sdl.Rect{
		X: int32(x1),
		Y: int32(y1),
		W: int32(x2 - x1),
		H: int32(y2 - y1),
	})

	// In a full implementation, we would draw properly rounded corners
	// but for simplicity, we'll just draw a filled rectangle
}

// roundedRect draws a rounded rectangle outline
func (r *Renderer) roundedRect(x1, y1, x2, y2, radius int, color sdl.Color) {
	r.renderer.SetDrawColor(color.R, color.G, color.B, color.A)

	// Draw rectangle outline
	r.renderer.DrawRect(&sdl.Rect{
		X: int32(x1),
		Y: int32(y1),
		W: int32(x2 - x1),
		H: int32(y2 - y1),
	})

	// In a full implementation, we would draw properly rounded corners
	// but for simplicity, we'll just draw a rectangle outline
}

// drawText renders text and returns its dimensions
func (r *Renderer) drawText(text string, x, y int, font *ttf.Font, color sdl.Color) (int, int) {
	if len(text) == 0 {
		return 0, 0
	}

	surface, err := font.RenderUTF8Solid(text, color)
	if err != nil {
		return 0, 0
	}
	defer surface.Free()

	texture, err := r.renderer.CreateTextureFromSurface(surface)
	if err != nil {
		return 0, 0
	}
	defer texture.Destroy()

	rect := &sdl.Rect{
		X: int32(x),
		Y: int32(y),
		W: surface.W,
		H: surface.H,
	}
	r.renderer.Copy(texture, nil, rect)

	return int(surface.W), int(surface.H)
}

// latLonToScreen converts geographical coordinates to screen coordinates
func (r *Renderer) latLonToScreen(lat, lon, centerLat, centerLon, maxDistance float64) (int, int) {
	// Convert lat/lon to distance in NM
	dx := (lon - centerLon) * math.Cos((lat+centerLat)/2*math.Pi/180) * 60
	dy := (lat - centerLat) * 60

	// Scale to screen coordinates
	scale := float64(r.height) / (maxDistance * 2)

	x := r.width/2 + int(dx*scale)
	y := r.height/2 - int(dy*scale) // Note the minus sign for Y

	return x, y
}

// lerpColor linearly interpolates between two colors
func lerpColor(a, b sdl.Color, t float64) sdl.Color {
	t = math.Max(0, math.Min(1, t)) // Clamp t to 0-1

	return sdl.Color{
		R: uint8(float64(a.R) + t*float64(b.R-a.R)),
		G: uint8(float64(a.G) + t*float64(b.G-a.G)),
		B: uint8(float64(a.B) + t*float64(b.B-a.B)),
		A: uint8(float64(a.A) + t*float64(b.A-a.A)),
	}
}

// latLonFromScreenCoords converts screen coordinates to geographic coordinates
func latLonFromScreenCoords(lat, lon *float64, x, y int, centerLat, centerLon, maxDistance float64, width, height int) {
	// Convert screen coordinates to normalized distance
	scale := maxDistance / float64(height/2)
	dx := float64(x-width/2) * scale
	dy := float64(height/2-y) * scale // Note the reversal for Y

	// Convert distance to lat/lon
	*lat = centerLat + dy/60.0
	*lon = centerLon + dx/(60.0*math.Cos((*lat+centerLat)/2*math.Pi/180))
}

// Clean up resources
func (r *Renderer) Cleanup() {
	if r.mapTexture != nil {
		r.mapTexture.Destroy()
	}

	if r.labelFont != nil && r.labelFont != r.regularFont {
		r.labelFont.Close()
	}
}
