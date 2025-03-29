package app

import (
	"fmt"
	"math"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/OJPARKINSON/viz1090/internal/adsb"
	"github.com/OJPARKINSON/viz1090/internal/beast"
	"github.com/OJPARKINSON/viz1090/internal/config"
	"github.com/OJPARKINSON/viz1090/internal/viz"
	"github.com/veandco/go-sdl2/sdl"
)

// App represents the main application
type App struct {
	config       *config.Config
	aircraft     *adsb.AircraftMap
	selectedICAO uint32
	centerLat    float64
	centerLon    float64
	maxDistance  float64

	vizRenderer *viz.Renderer
	running     bool

	beastConn               net.Conn
	isConnected             bool
	connectionRetryInterval time.Duration
	lastFrameTime           time.Time
	lastCleanup             time.Time

	mutex sync.RWMutex

	// Statistics
	numVisiblePlanes int
	numPlanes        int
	msgRate          float64
	msgRateAcc       float64
	sigAvg           float64
	sigAcc           float64
}

// New creates a new application instance
func New(cfg *config.Config) *App {
	return &App{
		config:                  cfg,
		aircraft:                adsb.NewAircraftMap(),
		centerLat:               cfg.InitialLat,
		centerLon:               cfg.InitialLon,
		maxDistance:             cfg.InitialZoom,
		running:                 false,
		lastCleanup:             time.Now(),
		lastFrameTime:           time.Now(),
		connectionRetryInterval: 5 * time.Second,
	}
}

// Initialize sets up the application
func (a *App) Initialize() error {
	var err error

	// Create visualization renderer
	a.vizRenderer, err = viz.NewRenderer(a.config.ScreenWidth, a.config.ScreenHeight,
		a.config.UIScale, a.config.Metric)
	if err != nil {
		return fmt.Errorf("failed to create renderer: %v", err)
	}

	return nil
}

// connectToBeast attempts to connect to a Beast data server
func (a *App) connectToBeast() {
	if a.isConnected {
		return
	}

	addr := fmt.Sprintf("%s:%d", a.config.ServerAddress, a.config.ServerPort)
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		fmt.Printf("Failed to connect to Beast server: %v (retrying in %v)\n",
			err, a.connectionRetryInterval)
		a.isConnected = false
		return
	}

	a.beastConn = conn
	a.isConnected = true

	// Start receiver goroutine
	go a.receiveBeastData()
	fmt.Printf("Connected to Beast server at %s\n", addr)
}

// receiveBeastData receives and processes Beast protocol data
func (a *App) receiveBeastData() {
	decoder := beast.NewDecoder(a.beastConn)

	for a.running {
		// Try to read a message
		msg, err := decoder.ReadMessage()
		if err != nil {
			if a.running {
				fmt.Printf("Beast protocol error: %v\n", err)
				a.isConnected = false

				// Close the connection
				if a.beastConn != nil {
					a.beastConn.Close()
					a.beastConn = nil
				}

				return
			}
			break
		}

		// Process the message if it's a Mode S message
		if msg.Type == beast.ModeLong {
			a.processModeS(msg.Data, msg.Timestamp)
		}
	}
}

// processModeS decodes and handles a Mode S message
func (a *App) processModeS(data []byte, timestamp uint64) {
	// Skip processing if data is too short
	if len(data) < 4 {
		return
	}

	// Extract downlink format (DF)
	df := data[0] >> 3

	// Only process DF17 and DF18 (ADS-B messages) for simplicity
	if df != 17 && df != 18 {
		return
	}

	// Extract ICAO address from bytes 1-3
	icao := uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3])

	// Create a basic message structure
	mm := &adsb.Message{
		DF:          int(df),
		ICAO:        icao,
		Timestamp:   time.Now(),
		SignalLevel: 100, // Default signal level
	}

	// Get or create aircraft entry
	aircraft := a.aircraft.GetOrCreate(icao)

	// Process based on message type
	if len(data) >= 5 {
		// Extended squitter message type
		metype := data[4] >> 3

		if metype >= 1 && metype <= 4 {
			// Aircraft identification
			callsign := adsb.DecodeCallsign(data[5:11])
			if callsign != "" {
				aircraft.Flight = callsign
			}
		} else if metype >= 9 && metype <= 18 {
			// Airborne position
			alt := adsb.DecodeAltitude(data)
			if alt != 0 {
				aircraft.Altitude = alt
			}

			// Extract CPR position
			cprLat := ((uint32(data[6]) & 0x03) << 15) | (uint32(data[7]) << 7) | (uint32(data[8]) >> 1)
			cprLon := ((uint32(data[8]) & 0x01) << 16) | (uint32(data[9]) << 8) | uint32(data[10])
			odd := (data[6] & 0x04) != 0

			// Store CPR position
			if odd {
				aircraft.OddCPRLat = int(cprLat)
				aircraft.OddCPRLon = int(cprLon)
				aircraft.OddCPRTime = time.Now().UnixNano() / int64(time.Millisecond)
			} else {
				aircraft.EvenCPRLat = int(cprLat)
				aircraft.EvenCPRLon = int(cprLon)
				aircraft.EvenCPRTime = time.Now().UnixNano() / int64(time.Millisecond)
			}

			// Try to decode position if we have both odd and even
			if aircraft.EvenCPRTime > 0 && aircraft.OddCPRTime > 0 {
				if math.Abs(float64(aircraft.EvenCPRTime-aircraft.OddCPRTime)) <= 10000 {
					lat, lon, ok := adsb.DecodeCPRPosition(aircraft.EvenCPRLat, aircraft.EvenCPRLon,
						aircraft.OddCPRLat, aircraft.OddCPRLon, odd)
					if ok {
						aircraft.Lat = lat
						aircraft.Lon = lon
						aircraft.SeenLatLon = time.Now()

						// Add to trail
						if len(aircraft.Trail) >= a.config.TrailLength {
							aircraft.Trail = aircraft.Trail[1:]
						}

						aircraft.Trail = append(aircraft.Trail, adsb.Position{
							Lat:       lat,
							Lon:       lon,
							Altitude:  aircraft.Altitude,
							Heading:   aircraft.Heading,
							Timestamp: time.Now(),
						})
					}
				}
			}
		} else if metype == 19 {
			// Airborne velocity
			speed, heading, vertRate, ok := adsb.DecodeVelocity(data)
			if ok {
				aircraft.Speed = speed
				aircraft.Heading = heading
				aircraft.VertRate = vertRate
			}
		}
	}

	// Update last seen time and signal level
	aircraft.Seen = time.Now()
	aircraft.SignalLevel[aircraft.Messages%8] = mm.SignalLevel
	aircraft.Messages++

	// Update statistics
	a.msgRateAcc++
	a.sigAcc += float64(mm.SignalLevel)
}

// cleanupStaleAircraft removes aircraft that haven't been seen recently
func (a *App) cleanupStaleAircraft() {
	now := time.Now()

	// Only do cleanup once per second
	if now.Sub(a.lastCleanup) < time.Second {
		return
	}
	a.lastCleanup = now

	ttl := time.Duration(a.config.DisplayTTL) * time.Second
	a.aircraft.RemoveStale(ttl)
}

// updateStatistics calculates various statistics
func (a *App) updateStatistics() {
	numVisible := 0
	numTotal := 0

	a.aircraft.ForEach(func(icao uint32, aircraft *adsb.Aircraft) {
		numTotal++
		if aircraft.Lat != 0 && aircraft.Lon != 0 {
			numVisible++
		}

		// Calculate sum of signal levels
		for i := 0; i < 8; i++ {
			a.sigAcc += float64(aircraft.SignalLevel[i])
		}
	})

	// Update statistics
	a.numVisiblePlanes = numVisible
	a.numPlanes = numTotal

	// Calculate average signal strength and message rate
	if numTotal > 0 {
		a.sigAvg = a.sigAcc / (float64(numTotal) * 8.0)
	} else {
		a.sigAvg = 0
	}

	// Reset accumulators
	a.sigAcc = 0
	a.msgRate = a.msgRateAcc
	a.msgRateAcc = 0
}

// Run starts the main application loop
func (a *App) Run() error {
	a.running = true

	// Setup cleanup ticker
	cleanupTicker := time.NewTicker(1 * time.Second)
	defer cleanupTicker.Stop()

	// Setup a connection attempt ticker
	connectionTicker := time.NewTicker(a.connectionRetryInterval)
	defer connectionTicker.Stop()

	// Setup signal handling for clean shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println("\nReceived shutdown signal. Exiting...")
		a.running = false
	}()

	fmt.Println("Starting viz1090-go...")

	// Main loop
	for a.running {
		// Handle input - quit if requested
		if !a.HandleInput() {
			a.running = false
			break
		}

		// Check for cleanup
		select {
		case <-cleanupTicker.C:
			a.cleanupStaleAircraft()
			a.updateStatistics()
		case <-connectionTicker.C:
			// Try to connect if not already connected
			if !a.isConnected {
				go a.connectToBeast()
			}
		default:
			// Continue without blocking
		}

		// Render frame
		a.mutex.RLock()
		a.vizRenderer.RenderFrame(a.aircraft.Copy(), a.centerLat, a.centerLon, a.maxDistance, a.selectedICAO)
		a.mutex.RUnlock()

		// Cap frame rate
		elapsed := time.Since(a.lastFrameTime)
		targetFrameTime := 33 * time.Millisecond // ~30fps
		if elapsed < targetFrameTime {
			time.Sleep(targetFrameTime - elapsed)
		}
		a.lastFrameTime = time.Now()
	}

	return nil
}

// Cleanup releases all resources
func (a *App) Cleanup() {
	a.running = false

	if a.beastConn != nil {
		a.beastConn.Close()
		a.beastConn = nil
	}

	if a.vizRenderer != nil {
		a.vizRenderer.Cleanup()
	}

	fmt.Println("Cleanup complete")
}

// HandleInput processes all SDL events and updates the application state accordingly
func (a *App) HandleInput() bool {
	for event := sdl.PollEvent(); event != nil; event = sdl.PollEvent() {
		switch e := event.(type) {
		case *sdl.QuitEvent:
			return false

		case *sdl.KeyboardEvent:
			if e.Type == sdl.KEYDOWN {
				switch e.Keysym.Sym {
				case sdl.K_ESCAPE:
					return false
				case sdl.K_EQUALS, sdl.K_PLUS:
					// Zoom in
					a.maxDistance *= 0.8
				case sdl.K_MINUS:
					// Zoom out
					a.maxDistance *= 1.25
				}
			}

		case *sdl.MouseWheelEvent:
			// Handle mouse wheel for zooming
			zoomFactor := 1.0
			if e.Y > 0 {
				zoomFactor = 0.8 // Zoom in
			} else if e.Y < 0 {
				zoomFactor = 1.25 // Zoom out
			}
			a.maxDistance *= zoomFactor

		case *sdl.MouseButtonEvent:
			if e.Type == sdl.MOUSEBUTTONDOWN {
				a.handleMouseButtonDown(e.X, e.Y, e.Button, int32(e.Clicks))
			}

		case *sdl.MouseMotionEvent:
			// Handle panning when mouse is dragged
			if e.State != 0 {
				a.handleMapPan(int(e.XRel), int(e.YRel))
			}
		}
	}
	return true
}

// handleMouseButtonDown processes mouse button events
func (a *App) handleMouseButtonDown(x, y int32, button uint8, clicks int32) {
	if button == sdl.BUTTON_LEFT {
		if clicks == 2 {
			// Double-click: Zoom in at the clicked location
			a.zoomToPosition(int(x), int(y), 0.5)
		} else {
			// Single click: Select aircraft
			a.selectAircraftAt(int(x), int(y))
		}
	}
}

// handleMapPan pans the map based on mouse motion
func (a *App) handleMapPan(xrel, yrel int) {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	// Convert pixel movement to longitude/latitude change
	// The formula adjusts for current zoom level and latitude
	scale := a.maxDistance / float64(a.vizRenderer.GetHeight()/2)

	// Y movement (latitude change)
	latChange := float64(yrel) * scale / 60.0

	// X movement needs to account for longitude compression at higher latitudes
	lonFactor := math.Cos(a.centerLat * math.Pi / 180.0)
	lonChange := float64(xrel) * scale / (60.0 * lonFactor)

	// Apply the changes (note: invert directions for natural map movement)
	a.centerLat -= latChange
	a.centerLon -= lonChange
}

// zoomToPosition zooms the map to a specific position
func (a *App) zoomToPosition(x, y int, factor float64) {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	// First calculate lat/lon at the clicked position
	lat, lon := a.pixelToLatLon(x, y)

	// Set the new center to this position
	a.centerLat = lat
	a.centerLon = lon

	// Apply zoom factor
	a.maxDistance *= factor
}

// selectAircraftAt tries to select an aircraft at the given screen position
func (a *App) selectAircraftAt(x, y int) {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	// First deselect current selection
	a.selectedICAO = 0

	// Find the closest aircraft to the click position
	var closestAircraft uint32
	closestDistance := 400.0 // Max selection distance squared (20px radius)

	a.aircraft.ForEach(func(icao uint32, aircraft *adsb.Aircraft) {
		// Skip aircraft without position
		if aircraft.Lat == 0 && aircraft.Lon == 0 {
			return
		}

		// Calculate screen position of aircraft
		aircraftX, aircraftY := a.latLonToPixel(aircraft.Lat, aircraft.Lon)

		// Calculate squared distance
		dx := float64(aircraftX - x)
		dy := float64(aircraftY - y)
		distSquared := dx*dx + dy*dy

		// Update if this is closer
		if distSquared < closestDistance {
			closestDistance = distSquared
			closestAircraft = icao
		}
	})

	// Set the selected aircraft
	if closestAircraft != 0 {
		a.selectedICAO = closestAircraft
		fmt.Printf("Selected aircraft: %06X\n", closestAircraft)
	}
}

// pixelToLatLon converts screen coordinates to latitude/longitude
func (a *App) pixelToLatLon(x, y int) (float64, float64) {
	// Get screen dimensions
	h := a.vizRenderer.GetHeight()
	w := a.vizRenderer.GetWidth()

	// Calculate offset from center in screen coordinates
	dx := float64(x - w/2)
	dy := float64(y - h/2)

	// Scale to match the current zoom level
	scale := a.maxDistance / float64(h/2)

	// Convert to lat/lon offsets
	// For latitude, each degree is roughly 60 nautical miles
	latOffset := -dy * scale / 60.0

	// For longitude, we need to account for the latitude
	lonFactor := math.Cos(a.centerLat * math.Pi / 180.0)
	lonOffset := dx * scale / (60.0 * lonFactor)

	return a.centerLat + latOffset, a.centerLon + lonOffset
}

// latLonToPixel converts latitude/longitude to screen coordinates
func (a *App) latLonToPixel(lat, lon float64) (int, int) {
	// Get screen dimensions
	h := a.vizRenderer.GetHeight()
	w := a.vizRenderer.GetWidth()

	// Calculate latitude offset from center in nautical miles
	latOffset := (lat - a.centerLat) * 60.0

	// Calculate longitude offset from center in nautical miles
	// accounting for latitude compression
	lonFactor := math.Cos(a.centerLat * math.Pi / 180.0)
	lonOffset := (lon - a.centerLon) * 60.0 * lonFactor

	// Scale to screen coordinates
	scale := float64(h/2) / a.maxDistance

	// Convert to screen offsets from center
	dx := lonOffset * scale
	dy := -latOffset * scale // Invert Y because screen coordinates increase downward

	// Return final screen coordinates
	return int(float64(w)/2.0 + dx), int(float64(h)/2.0 + dy)
}
