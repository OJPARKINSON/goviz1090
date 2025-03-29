package viz

import (
	"fmt"
	"math"
	"time"

	"github.com/OJPARKINSON/viz1090/internal/adsb"
	"github.com/OJPARKINSON/viz1090/internal/map_system"
	"github.com/veandco/go-sdl2/sdl"
	"github.com/veandco/go-sdl2/ttf"
)

// Constants for visualization
const (
	LATLONMULT   = 111.195 // 6371.0 * math.Pi / 180.0 - conversion factor for lat/lon to km
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

// LabelSystem manages aircraft labels and prevents overlaps
type LabelSystem struct {
	width     int
	height    int
	uiScale   int
	metric    bool
	labelFont *ttf.Font
}

// NewLabelSystem creates a new label system
func NewLabelSystem(width, height, uiScale int, metric bool) *LabelSystem {
	return &LabelSystem{
		width:   width,
		height:  height,
		uiScale: uiScale,
		metric:  metric,
	}
}

// SetFont sets the font for the label system
func (ls *LabelSystem) SetFont(font *ttf.Font) {
	ls.labelFont = font
}

// UpdateLabels updates all aircraft labels to avoid overlaps
func (ls *LabelSystem) UpdateLabels(aircraft map[uint32]*adsb.Aircraft) {
	// Resolve label conflicts - simplified version for now
	for i := 0; i < 4; i++ { // Iterate a few times for better results
		ls.resolveOverlaps(aircraft)
	}
}

// resolveOverlaps detects and resolves label overlaps
func (ls *LabelSystem) resolveOverlaps(aircraft map[uint32]*adsb.Aircraft) {
	// Algorithm to prevent label overlaps
	// This is a simplified implementation

	// Clear forces
	for _, a := range aircraft {
		a.LabelDX = 0
		a.LabelDY = 0
	}

	// Calculate forces based on overlaps
	for _, a1 := range aircraft {
		if a1.LabelW == 0 || a1.LabelH == 0 {
			continue
		}

		// Apply screen edge forces
		edge := float64(15 * ls.uiScale)
		if a1.LabelX < edge {
			a1.LabelDX += 0.01 * (edge - a1.LabelX)
		}
		if a1.LabelX+a1.LabelW > float64(ls.width)-edge {
			a1.LabelDX += 0.01 * (float64(ls.width) - edge - a1.LabelX - a1.LabelW)
		}
		if a1.LabelY < edge {
			a1.LabelDY += 0.01 * (edge - a1.LabelY)
		}
		if a1.LabelY+a1.LabelH > float64(ls.height)-edge {
			a1.LabelDY += 0.01 * (float64(ls.height) - edge - a1.LabelY - a1.LabelH)
		}

		// Force to keep near aircraft
		ax, ay := float64(a1.X), float64(a1.Y)
		dx := a1.LabelX + a1.LabelW/2 - ax
		dy := a1.LabelY + a1.LabelH/2 - ay
		dist := math.Sqrt(dx*dx + dy*dy)

		if dist > 0 {
			// Pull towards aircraft
			targetDist := 40.0 * float64(ls.uiScale)
			force := 0.0015 * (dist - targetDist)
			if force != 0 {
				a1.LabelDX -= force * dx / dist
				a1.LabelDY -= force * dy / dist
			}
		}

		// Forces from other labels
		for _, a2 := range aircraft {
			if a1 == a2 || a2.LabelW == 0 || a2.LabelH == 0 {
				continue
			}

			dx := a1.LabelX + a1.LabelW/2 - (a2.LabelX + a2.LabelW/2)
			dy := a1.LabelY + a1.LabelH/2 - (a2.LabelY + a2.LabelH/2)
			dist := math.Sqrt(dx*dx + dy*dy)

			if dist < 0.001 {
				continue // Avoid division by zero
			}

			// Calculate repulsion
			targetDist := (a1.LabelW + a2.LabelW + a1.LabelH + a2.LabelH) / 4

			if dist < targetDist {
				force := 0.001 * (targetDist - dist)
				a1.LabelDX += force * dx / dist
				a1.LabelDY += force * dy / dist
			}
		}
	}

	// Apply forces with damping
	for _, a := range aircraft {
		if a.LabelW == 0 || a.LabelH == 0 {
			continue
		}

		// Apply damping
		a.LabelDX *= 0.85
		a.LabelDY *= 0.85

		// Limit velocity
		maxVel := 2.0
		if math.Abs(a.LabelDX) > maxVel {
			a.LabelDX = math.Copysign(maxVel, a.LabelDX)
		}
		if math.Abs(a.LabelDY) > maxVel {
			a.LabelDY = math.Copysign(maxVel, a.LabelDY)
		}

		// Apply movement
		a.LabelX += a.LabelDX
		a.LabelY += a.LabelDY
	}
}

// Renderer handles drawing the radar display
type Renderer struct {
	window      *sdl.Window
	renderer    *sdl.Renderer
	regularFont *ttf.Font
	boldFont    *ttf.Font
	labelFont   *ttf.Font
	mapTexture  *sdl.Texture
	width       int
	height      int
	uiScale     int
	metric      bool
	lastRedraw  time.Time
	mapDrawn    bool
	mapSystem   *map_system.Map
	labelSystem *LabelSystem

	// Mouse and interaction
	mouseMoved bool
	mouseX     int
	mouseY     int
	clickX     int
	clickY     int
	clickTime  time.Time
}

// NewRenderer creates a new visualization renderer
func NewRenderer(width, height, uiScale int, metric bool) (*Renderer, error) {
	var err error
	r := &Renderer{
		width:    width,
		height:   height,
		uiScale:  uiScale,
		metric:   metric,
		mapDrawn: false,
	}

	// Initialize SDL
	if err = sdl.Init(sdl.INIT_VIDEO); err != nil {
		return nil, fmt.Errorf("failed to initialize SDL: %v", err)
	}

	// Initialize TTF
	if err = ttf.Init(); err != nil {
		sdl.Quit()
		return nil, fmt.Errorf("failed to initialize TTF: %v", err)
	}

	// Get display size if needed
	if width == 0 || height == 0 {
		displayCount, err := sdl.GetNumVideoDisplays()
		if err != nil {
			return nil, fmt.Errorf("failed to get display count: %v", err)
		}

		for i := 0; i < displayCount; i++ {
			bounds, err := sdl.GetDisplayBounds(i)
			if err != nil {
				continue
			}
			width = int(bounds.W)
			height = int(bounds.H)
			break
		}
	}

	// Create window
	r.window, err = sdl.CreateWindow("viz1090-go", sdl.WINDOWPOS_CENTERED, sdl.WINDOWPOS_CENTERED,
		int32(width), int32(height), sdl.WINDOW_SHOWN)
	if err != nil {
		return nil, fmt.Errorf("failed to create window: %v", err)
	}

	// Create renderer
	r.renderer, err = sdl.CreateRenderer(r.window, -1, sdl.RENDERER_ACCELERATED)
	if err != nil {
		r.window.Destroy()
		return nil, fmt.Errorf("failed to create renderer: %v", err)
	}

	// Create map texture
	r.mapTexture, err = r.renderer.CreateTexture(
		sdl.PIXELFORMAT_RGBA8888,
		sdl.TEXTUREACCESS_TARGET,
		int32(width), int32(height))
	if err != nil {
		r.renderer.Destroy()
		r.window.Destroy()
		return nil, fmt.Errorf("failed to create map texture: %v", err)
	}

	// Load fonts
	r.regularFont, err = ttf.OpenFont("font/TerminusTTF-4.46.0.ttf", 12*uiScale)
	if err != nil {
		r.mapTexture.Destroy()
		r.renderer.Destroy()
		r.window.Destroy()
		return nil, fmt.Errorf("failed to load regular font: %v", err)
	}

	r.boldFont, err = ttf.OpenFont("font/TerminusTTF-Bold-4.46.0.ttf", 12*uiScale)
	if err != nil {
		r.regularFont.Close()
		r.mapTexture.Destroy()
		r.renderer.Destroy()
		r.window.Destroy()
		return nil, fmt.Errorf("failed to load bold font: %v", err)
	}

	r.labelFont = r.boldFont

	// Initialize the label system
	r.labelSystem = NewLabelSystem(width, height, uiScale, metric)
	r.labelSystem.SetFont(r.labelFont)

	// Initialize the map system
	r.mapSystem = map_system.NewMap()
	err = r.mapSystem.LoadMapData("mapdata.bin", "airportdata.bin", "mapnames", "airportnames")
	if err != nil {
		fmt.Printf("Warning: Failed to load map data: %v\n", err)
	}

	return r, nil
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
	r.drawStatus(countAircraft(aircraft), countVisibleAircraft(aircraft), centerLat, centerLon)

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

		// Initialize label position if needed
		if a.LabelX == 0 && a.LabelY == 0 {
			a.LabelX = float64(x)
			a.LabelY = float64(y) + 20*float64(r.uiScale)
		}
	}
}

// latLonToScreen converts geographical coordinates to screen coordinates
func (r *Renderer) latLonToScreen(lat, lon, centerLat, centerLon, maxDistance float64) (int, int) {
	// Convert lat/lon to distance in NM
	dx := (lon - centerLon) * math.Cos(((lat+centerLat)/2.0)*math.Pi/180.0) * 60
	dy := (lat - centerLat) * 60

	// Scale to screen coordinates
	scale := float64(r.height) / (maxDistance * 2)

	x := r.width/2 + int(dx*scale)
	y := r.height/2 - int(dy*scale) // Note the minus sign for Y

	return x, y
}

// drawMap renders the geographic map to a texture
func (r *Renderer) drawMap(centerLat, centerLon, maxDistance float64) {
	// Save original render target
	original := r.renderer.GetRenderTarget()

	// Set map texture as render target
	r.renderer.SetRenderTarget(r.mapTexture)

	// Clear map texture
	r.renderer.SetDrawColor(ColorBackground.R, ColorBackground.G, ColorBackground.B, ColorBackground.A)
	r.renderer.Clear()

	// Calculate visible area bounds
	latMin, lonMin, latMax, lonMax := r.calculateVisibleBounds(centerLat, centerLon, maxDistance)

	// Draw map elements if available
	if r.mapSystem != nil {
		// Get visible map features
		mapLines, airportLines := r.mapSystem.GetVisibleLines(latMin, latMax, lonMin, lonMax)

		// Draw map lines
		r.renderer.SetDrawColor(ColorMap.R, ColorMap.G, ColorMap.B, ColorMap.A)
		for _, line := range mapLines {
			x1, y1 := r.latLonToScreen(line.Start.Lat, line.Start.Lon, centerLat, centerLon, maxDistance)
			x2, y2 := r.latLonToScreen(line.End.Lat, line.End.Lon, centerLat, centerLon, maxDistance)

			// Skip if outside viewport
			if r.outOfBounds(x1, y1) && r.outOfBounds(x2, y2) {
				continue
			}

			r.renderer.DrawLine(int32(x1), int32(y1), int32(x2), int32(y2))
		}

		// Draw airport lines
		r.renderer.SetDrawColor(ColorAirport.R, ColorAirport.G, ColorAirport.B, ColorAirport.A)
		for _, line := range airportLines {
			x1, y1 := r.latLonToScreen(line.Start.Lat, line.Start.Lon, centerLat, centerLon, maxDistance)
			x2, y2 := r.latLonToScreen(line.End.Lat, line.End.Lon, centerLat, centerLon, maxDistance)

			// Skip if outside viewport
			if r.outOfBounds(x1, y1) && r.outOfBounds(x2, y2) {
				continue
			}

			r.renderer.DrawLine(int32(x1), int32(y1), int32(x2), int32(y2))
		}

		// Draw place labels
		placeLabels, airportLabels := r.mapSystem.GetVisibleLabels(latMin, latMax, lonMin, lonMax)

		for _, label := range placeLabels {
			x, y := r.latLonToScreen(label.Location.Lat, label.Location.Lon, centerLat, centerLon, maxDistance)
			if r.outOfBounds(x, y) {
				continue
			}
			r.drawText(label.Text, x, y, r.regularFont, ColorText)
		}

		for _, label := range airportLabels {
			x, y := r.latLonToScreen(label.Location.Lat, label.Location.Lon, centerLat, centerLon, maxDistance)
			if r.outOfBounds(x, y) {
				continue
			}
			r.drawText(label.Text, x, y, r.boldFont, ColorText)
		}
	} else {
		// Draw a fallback grid if no map data is loaded
		r.renderer.SetDrawColor(ColorMap.R, ColorMap.G, ColorMap.B, ColorMap.A)
		for i := 0; i < r.width; i += 50 {
			r.renderer.DrawLine(int32(i), 0, int32(i), int32(r.height))
		}
		for i := 0; i < r.height; i += 50 {
			r.renderer.DrawLine(0, int32(i), int32(r.width), int32(i))
		}
	}

	// Restore original render target
	r.renderer.SetRenderTarget(original)

	r.mapDrawn = true
	r.lastRedraw = time.Now()
}

// calculateVisibleBounds calculates the lat/lon bounds of the visible area
func (r *Renderer) calculateVisibleBounds(centerLat, centerLon, maxDistance float64) (latMin, lonMin, latMax, lonMax float64) {
	// Calculate how much lat/lon changes per pixel
	latPerPixel := (maxDistance * 2) / float64(r.height) / 60.0
	lonPerPixel := latPerPixel / math.Cos(centerLat*math.Pi/180.0)

	// Calculate bounds with a margin
	halfWidth := float64(r.width) / 2.0
	halfHeight := float64(r.height) / 2.0

	latMin = centerLat - halfHeight*latPerPixel
	latMax = centerLat + halfHeight*latPerPixel
	lonMin = centerLon - halfWidth*lonPerPixel
	lonMax = centerLon + halfWidth*lonPerPixel

	return
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
		r.drawAircraftSymbol(a.X, a.Y, a.Heading, color)

		// Draw label
		r.drawAircraftLabel(a, color)
	}
}

// drawAircraftSymbol draws an aircraft symbol at the specified position
func (r *Renderer) drawAircraftSymbol(x, y, heading int, color sdl.Color) {
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
}

// drawAircraftLabel draws a label for the specified aircraft
func (r *Renderer) drawAircraftLabel(a *adsb.Aircraft, color sdl.Color) {
	// If this is the first time seeing this aircraft, initialize label size
	if a.LabelW == 0 || a.LabelH == 0 {
		a.LabelW = 100
		a.LabelH = 45
		a.LabelOpacity = 0
		a.LabelLevel = 0
	}

	// Fade in opacity
	if a.LabelOpacity < 1.0 {
		a.LabelOpacity += 0.05
	}

	// Don't draw if opacity is too low
	if a.LabelOpacity < 0.05 {
		return
	}

	alpha := uint8(255 * a.LabelOpacity)

	// Background color with alpha
	bgColor := ColorLabelBg
	bgColor.A = alpha

	// Draw background
	r.drawRect(int32(a.LabelX), int32(a.LabelY), int32(a.LabelW), int32(a.LabelH), bgColor)

	// Draw outline
	lineColor := ColorLabelLine
	lineColor.A = alpha
	r.drawRectOutline(int32(a.LabelX), int32(a.LabelY), int32(a.LabelW), int32(a.LabelH), lineColor)

	// Draw label content based on label level
	textY := int(a.LabelY) + 5

	// Always show callsign
	textColor := ColorLabel
	textColor.A = alpha
	flight := a.Flight
	if flight == "" {
		flight = fmt.Sprintf("%06X", a.ICAO)
	}
	r.drawText(flight, int(a.LabelX)+5, textY, r.labelFont, textColor)
	textY += 14

	// Show altitude and speed if level allows
	if a.LabelLevel < 1 {
		subTextColor := ColorSubLabel
		subTextColor.A = alpha

		// Altitude
		altText := ""
		if r.metric {
			altText = fmt.Sprintf(" %dm", int(float64(a.Altitude)/3.2828))
		} else {
			altText = fmt.Sprintf(" %d'", a.Altitude)
		}
		r.drawText(altText, int(a.LabelX)+5, textY, r.regularFont, subTextColor)
		textY += 14

		// Speed
		speedText := ""
		if r.metric {
			speedText = fmt.Sprintf(" %dkm/h", int(float64(a.Speed)*1.852))
		} else {
			speedText = fmt.Sprintf(" %dkts", a.Speed)
		}
		r.drawText(speedText, int(a.LabelX)+5, textY, r.regularFont, subTextColor)
	}

	// Draw connecting line from aircraft to label
	anchorX := int32(a.LabelX)
	if a.LabelX+a.LabelW/2 > float64(a.X) {
		anchorX = int32(a.LabelX)
	} else {
		anchorX = int32(a.LabelX + a.LabelW)
	}

	anchorY := int32(a.LabelY + a.LabelH/2)

	r.renderer.DrawLine(int32(a.X), int32(a.Y), anchorX, anchorY)
}

// drawScaleBars draws distance scale indicators
func (r *Renderer) drawScaleBars(maxDistance float64) {
	// Find appropriate scale - powers of 10
	scalePower := 0
	scaleBarDist := 0

	for {
		dist := math.Pow10(scalePower)
		screenDist := float64(r.height/2) * (dist / maxDistance)

		if screenDist > 100 {
			scaleBarDist = int(screenDist)
			break
		}
		scalePower++
	}

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
	locText := fmt.Sprintf("%.4fN %.4f%c", centerLat,
		math.Abs(centerLon), map[bool]byte{true: 'E', false: 'W'}[centerLon >= 0])

	// Format aircraft count text
	dispText := fmt.Sprintf("%d/%d", visibleCount, aircraftCount)

	// Draw status boxes
	padding := 5
	x := padding
	y := r.height - 30*r.uiScale

	// Draw the status boxes
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
	r.renderer.SetDrawColor(ColorButtonBg.R, ColorButtonBg.G, ColorButtonBg.B, ColorButtonBg.A)
	r.renderer.FillRect(&sdl.Rect{
		X: int32(*x),
		Y: int32(*y),
		W: int32(labelWidth + messageWidth),
		H: int32(messageFontHeight),
	})

	// Label box
	r.renderer.SetDrawColor(color.R, color.G, color.B, color.A)
	r.renderer.FillRect(&sdl.Rect{
		X: int32(*x),
		Y: int32(*y),
		W: int32(labelWidth),
		H: int32(messageFontHeight),
	})

	// Outline
	r.renderer.DrawRect(&sdl.Rect{
		X: int32(*x),
		Y: int32(*y),
		W: int32(labelWidth + messageWidth),
		H: int32(messageFontHeight),
	})

	// Label text (in label box)
	textBgColor := ColorButtonBg
	r.drawText(label, *x+labelFontWidth/2, *y, r.labelFont, textBgColor)

	// Value text (in message box)
	r.drawText(value, *x+labelWidth+messageFontWidth/2, *y, r.regularFont, color)

	// Update x position for next box
	*x = *x + labelWidth + messageWidth + PAD
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

// drawRect draws a filled rectangle
func (r *Renderer) drawRect(x, y, w, h int32, color sdl.Color) {
	r.renderer.SetDrawColor(color.R, color.G, color.B, color.A)
	r.renderer.FillRect(&sdl.Rect{X: x, Y: y, W: w, H: h})
}

// drawRectOutline draws a rectangle outline
func (r *Renderer) drawRectOutline(x, y, w, h int32, color sdl.Color) {
	r.renderer.SetDrawColor(color.R, color.G, color.B, color.A)
	r.renderer.DrawRect(&sdl.Rect{X: x, Y: y, W: w, H: h})
}

// outOfBounds checks if a point is off the screen
func (r *Renderer) outOfBounds(x, y int) bool {
	return x < 0 || x >= r.width || y < 0 || y >= r.height
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

// countAircraft returns the total number of aircraft
func countAircraft(aircraft map[uint32]*adsb.Aircraft) int {
	return len(aircraft)
}

// countVisibleAircraft counts aircraft with valid positions
func countVisibleAircraft(aircraft map[uint32]*adsb.Aircraft) int {
	count := 0
	for _, a := range aircraft {
		if a.Lat != 0 && a.Lon != 0 {
			count++
		}
	}
	return count
}

// Cleanup releases all resources
func (r *Renderer) Cleanup() {
	if r.labelFont != nil && r.labelFont != r.regularFont {
		r.labelFont.Close()
	}

	if r.boldFont != nil {
		r.boldFont.Close()
	}

	if r.regularFont != nil {
		r.regularFont.Close()
	}

	if r.mapTexture != nil {
		r.mapTexture.Destroy()
	}

	if r.renderer != nil {
		r.renderer.Destroy()
	}

	if r.window != nil {
		r.window.Destroy()
	}

	ttf.Quit()
	sdl.Quit()
}

func (r *Renderer) GetWidth() int {
	return r.width
}

// GetHeight returns the renderer height
func (r *Renderer) GetHeight() int {
	return r.height
}
