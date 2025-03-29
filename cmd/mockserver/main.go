package main

import (
	"flag"
	"fmt"
	"math"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Constants for ADS-B message types
const (
	// Beast message types
	ModeAC    = byte('1') // Mode A/C message
	ModeShort = byte('2') // Mode S short message
	ModeLong  = byte('3') // Mode S long message

	// Downlink Format types
	DF17 = 17 // ADS-B message
	DF18 = 18 // ADS-B message via TIS-B

	// Type codes
	TC_IDENT        = 4  // Aircraft identification
	TC_AIRBORNE_POS = 11 // Airborne position
	TC_AIRBORNE_VEL = 19 // Airborne velocity

	// Other constants
	EscapeChar = byte(0x1A) // Beast protocol escape character
)

// SimAircraft represents a simulated aircraft
type SimAircraft struct {
	ICAO      uint32    // 24-bit ICAO address
	Callsign  string    // Flight number/callsign
	Lat       float64   // Latitude
	Lon       float64   // Longitude
	Alt       int       // Altitude in feet
	Speed     int       // Ground speed in knots
	Heading   int       // Track in degrees
	ClimbRate int       // Vertical rate in ft/min
	Odd       bool      // CPR odd/even flag toggle
	LastSeen  time.Time // Time of last position update
	mutex     sync.Mutex
}

// BeastServer simulates a Beast format data provider
type BeastServer struct {
	aircraft  map[uint32]*SimAircraft
	listeners []net.Conn
	mutex     sync.Mutex
	running   bool
}

// NewBeastServer creates a new Beast server
func NewBeastServer() *BeastServer {
	return &BeastServer{
		aircraft:  make(map[uint32]*SimAircraft),
		listeners: make([]net.Conn, 0),
		running:   false,
	}
}

// AddAircraft adds a new aircraft to the simulation
func (s *BeastServer) AddAircraft(icao uint32, callsign string, lat, lon float64, alt, speed, heading int) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.aircraft[icao] = &SimAircraft{
		ICAO:      icao,
		Callsign:  callsign,
		Lat:       lat,
		Lon:       lon,
		Alt:       alt,
		Speed:     speed,
		Heading:   heading,
		ClimbRate: rand.Intn(1000) - 500, // Random climb rate between -500 and 500 ft/min
		LastSeen:  time.Now(),
	}
}

// Start begins the server on the specified port
func (s *BeastServer) Start(port int) error {
	listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		return fmt.Errorf("failed to start server: %v", err)
	}
	defer listener.Close()

	fmt.Printf("Beast server running on port %d\n", port)

	// Start the update goroutine
	s.running = true
	go s.updateLoop()

	// Accept connections
	for s.running {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Printf("Error accepting connection: %v\n", err)
			continue
		}

		fmt.Printf("Client connected: %s\n", conn.RemoteAddr())

		s.mutex.Lock()
		s.listeners = append(s.listeners, conn)
		s.mutex.Unlock()

		// Handle client in a goroutine
		go s.handleClient(conn)
	}

	return nil
}

// Stop shuts down the server
func (s *BeastServer) Stop() {
	s.running = false

	s.mutex.Lock()
	defer s.mutex.Unlock()

	for _, conn := range s.listeners {
		conn.Close()
	}
	s.listeners = nil
}

// handleClient handles a client connection
func (s *BeastServer) handleClient(conn net.Conn) {
	defer func() {
		conn.Close()

		s.mutex.Lock()
		for i, c := range s.listeners {
			if c == conn {
				s.listeners = append(s.listeners[:i], s.listeners[i+1:]...)
				break
			}
		}
		s.mutex.Unlock()

		fmt.Printf("Client disconnected: %s\n", conn.RemoteAddr())
	}()

	// Read buffer to keep connection alive and detect client disconnect
	buffer := make([]byte, 1024)
	for s.running {
		// Set read deadline
		conn.SetReadDeadline(time.Now().Add(1 * time.Second))

		_, err := conn.Read(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Timeout is normal, continue
				continue
			}
			// Other error - client disconnected
			break
		}
	}
}

// updateLoop periodically updates aircraft positions and sends messages
func (s *BeastServer) updateLoop() {
	ticker := time.NewTicker(200 * time.Millisecond) // 5 updates per second
	defer ticker.Stop()

	for s.running {
		select {
		case <-ticker.C:
			s.updateAircraft()
			s.sendUpdates()
		}
	}
}

// updateAircraft updates the positions of all simulated aircraft
func (s *BeastServer) updateAircraft() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	now := time.Now()

	for _, a := range s.aircraft {
		a.mutex.Lock()

		// Calculate time since last update
		elapsed := now.Sub(a.LastSeen).Seconds()
		a.LastSeen = now

		// Update position based on speed and heading
		distanceNM := float64(a.Speed) * elapsed / 3600.0 // Convert knots to NM/s

		// Convert to radians and calculate movement
		headingRad := float64(a.Heading) * math.Pi / 180.0

		// Move aircraft - adjust longitude based on latitude for accurate movement
		latFactor := math.Cos(a.Lat * math.Pi / 180.0)
		a.Lon += (distanceNM * math.Sin(headingRad)) / (60.0 * latFactor)
		a.Lat += (distanceNM * math.Cos(headingRad)) / 60.0

		// Update altitude based on climb rate
		a.Alt += int((float64(a.ClimbRate) * elapsed) / 60.0)

		// Randomly change heading slightly for realistic variation
		if rand.Float64() < 0.05 { // 5% chance per update
			a.Heading += rand.Intn(3) - 1 // -1, 0, or 1 degree

			// Keep heading in 0-359 range
			if a.Heading < 0 {
				a.Heading += 360
			} else if a.Heading >= 360 {
				a.Heading -= 360
			}
		}

		// Randomly change climb rate occasionally
		if rand.Float64() < 0.02 { // 2% chance per update
			a.ClimbRate = rand.Intn(2000) - 1000 // Between -1000 and 1000 ft/min
		}

		// Toggle odd/even flag
		a.Odd = !a.Odd

		a.mutex.Unlock()
	}
}

// sendUpdates sends ADS-B messages for all aircraft to all connected clients
func (s *BeastServer) sendUpdates() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if len(s.listeners) == 0 {
		return // No clients connected
	}

	timestamp := uint64(time.Now().UnixNano() / 1000000) // Timestamp in milliseconds

	for _, a := range s.aircraft {
		a.mutex.Lock()

		// Create and send messages for this aircraft

		// Only send ID message occasionally (about every 5 seconds)
		if rand.Float64() < 0.05 {
			idMsg := createADSBIdentMessage(a.ICAO, a.Callsign)
			beastMsg := encodeBeastMessage(ModeLong, idMsg, timestamp, byte(rand.Intn(100)+100))
			s.broadcast(beastMsg)
		}

		// Always send position message
		posMsg := createADSBPositionMessage(a.ICAO, a.Lat, a.Lon, a.Alt, a.Odd)
		beastMsg := encodeBeastMessage(ModeLong, posMsg, timestamp, byte(rand.Intn(100)+100))
		s.broadcast(beastMsg)

		// Always send velocity message
		velMsg := createADSBVelocityMessage(a.ICAO, a.Speed, a.Heading, a.ClimbRate)
		beastMsg = encodeBeastMessage(ModeLong, velMsg, timestamp, byte(rand.Intn(100)+100))
		s.broadcast(beastMsg)

		a.mutex.Unlock()

		// Small delay between aircraft updates to prevent flooding
		time.Sleep(5 * time.Millisecond)
	}
}

// broadcast sends a message to all connected clients
func (s *BeastServer) broadcast(msg []byte) {
	for _, conn := range s.listeners {
		_, err := conn.Write(msg)
		if err != nil {
			fmt.Printf("Error writing to client: %v\n", err)
			// We'll remove failed connections in handleClient
		}
	}
}

// createADSBIdentMessage creates an ADS-B aircraft identification message
func createADSBIdentMessage(icao uint32, callsign string) []byte {
	// DF17 (Extended squitter) + ADS-B Identification message
	msg := make([]byte, 14)

	// DF17, CA=5
	msg[0] = (DF17 << 3) | 5

	// ICAO address
	msg[1] = byte((icao >> 16) & 0xFF)
	msg[2] = byte((icao >> 8) & 0xFF)
	msg[3] = byte(icao & 0xFF)

	// Type code = 4 (aircraft ID) + category
	msg[4] = (TC_IDENT << 3) | 0

	// Pad callsign to 8 characters
	paddedCallsign := callsign
	if len(paddedCallsign) < 8 {
		paddedCallsign += strings.Repeat(" ", 8-len(paddedCallsign))
	} else if len(paddedCallsign) > 8 {
		paddedCallsign = paddedCallsign[:8]
	}

	// Encode callsign (6 bits per character according to ADS-B spec)
	charset := "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 "

	// First 4 characters
	var c1, c2, c3, c4 int
	for i, char := range paddedCallsign[:4] {
		idx := strings.IndexRune(charset, char)
		if idx == -1 {
			idx = 36 // Space character
		}

		switch i {
		case 0:
			c1 = idx
		case 1:
			c2 = idx
		case 2:
			c3 = idx
		case 3:
			c4 = idx
		}
	}

	msg[5] = byte((c1 << 2) | (c2 >> 4))
	msg[6] = byte(((c2 & 0x0F) << 4) | (c3 >> 2))
	msg[7] = byte(((c3 & 0x03) << 6) | c4)

	// Last 4 characters
	c1, c2, c3, c4 = 0, 0, 0, 0
	for i, char := range paddedCallsign[4:8] {
		idx := strings.IndexRune(charset, char)
		if idx == -1 {
			idx = 36 // Space character
		}

		switch i {
		case 0:
			c1 = idx
		case 1:
			c2 = idx
		case 2:
			c3 = idx
		case 3:
			c4 = idx
		}
	}

	msg[8] = byte((c1 << 2) | (c2 >> 4))
	msg[9] = byte(((c2 & 0x0F) << 4) | (c3 >> 2))
	msg[10] = byte(((c3 & 0x03) << 6) | c4)

	// CRC fields are left at zero for simplicity

	return msg
}

// createADSBPositionMessage creates an ADS-B airborne position message
func createADSBPositionMessage(icao uint32, lat, lon float64, alt int, odd bool) []byte {
	// DF17 (Extended squitter) + ADS-B Airborne Position message
	msg := make([]byte, 14)

	// DF17, CA=5
	msg[0] = (DF17 << 3) | 5

	// ICAO address
	msg[1] = byte((icao >> 16) & 0xFF)
	msg[2] = byte((icao >> 8) & 0xFF)
	msg[3] = byte(icao & 0xFF)

	// Type code = 11 (airborne position) + surveillance status (0) + single antenna flag (0) + odd/even flag
	var tc byte = (TC_AIRBORNE_POS << 3)
	if odd {
		tc |= 1 // Set odd/even flag
	}
	msg[4] = tc

	// Altitude encoding (25ft resolution)
	altCode := (alt + 1000) / 25
	msg[5] = byte((altCode >> 4) & 0xFF)
	msg[6] = byte((altCode & 0x0F) << 4)

	// CPR encoding
	// This is a simplified CPR encoding - real implementation is more complex
	latCPR := uint32((lat / 360.0) * 131072)
	lonCPR := uint32((lon / 360.0) * 131072)

	msg[6] |= byte((latCPR >> 15) & 0x0F)
	msg[7] = byte((latCPR >> 7) & 0xFF)
	msg[8] = byte((latCPR & 0x7F) << 1)
	msg[8] |= byte((lonCPR >> 16) & 0x01)
	msg[9] = byte((lonCPR >> 8) & 0xFF)
	msg[10] = byte(lonCPR & 0xFF)

	// CRC fields are left at zero for simplicity

	return msg
}

// createADSBVelocityMessage creates an ADS-B airborne velocity message
func createADSBVelocityMessage(icao uint32, speed, heading, climbRate int) []byte {
	// DF17 (Extended squitter) + ADS-B Airborne Velocity message
	msg := make([]byte, 14)

	// DF17, CA=5
	msg[0] = (DF17 << 3) | 5

	// ICAO address
	msg[1] = byte((icao >> 16) & 0xFF)
	msg[2] = byte((icao >> 8) & 0xFF)
	msg[3] = byte(icao & 0xFF)

	// Type code = 19 (airborne velocity) + subtype 1 (ground speed)
	msg[4] = (TC_AIRBORNE_VEL << 3) | 1

	// Intent change flag (0), IFR capability flag (1), navigation uncertainty (0)
	msg[5] = 0x40

	// East-West velocity component
	ewVel := int(float64(speed) * math.Sin(float64(heading)*math.Pi/180.0))
	ewDir := 0
	if ewVel < 0 {
		ewDir = 1
		ewVel = -ewVel
	}
	ewVel = ewVel + 1 // 1-knot resolution
	msg[5] |= byte(ewDir << 2)
	msg[5] |= byte((ewVel >> 8) & 0x03)
	msg[6] = byte(ewVel & 0xFF)

	// North-South velocity component
	nsVel := int(float64(speed) * math.Cos(float64(heading)*math.Pi/180.0))
	nsDir := 0
	if nsVel < 0 {
		nsDir = 1
		nsVel = -nsVel
	}
	nsVel = nsVel + 1 // 1-knot resolution
	msg[7] = byte(nsDir << 7)
	msg[7] |= byte((nsVel >> 3) & 0x7F)
	msg[8] = byte((nsVel & 0x07) << 5)

	// Vertical rate
	vertRate := climbRate
	vertSign := 0
	if vertRate < 0 {
		vertSign = 1
		vertRate = -vertRate
	}

	// 64 fpm resolution, remove LSB
	vertRate = (vertRate + 32) / 64
	msg[8] |= byte(vertSign << 3)
	msg[8] |= byte((vertRate >> 6) & 0x07)
	msg[9] = byte((vertRate & 0x3F) << 2)

	// CRC fields are left at zero for simplicity

	return msg
}

// encodeBeastMessage encodes a Beast format message
func encodeBeastMessage(msgType byte, data []byte, timestamp uint64, signalLevel byte) []byte {
	// Estimate buffer size (message + possible escape bytes)
	buf := make([]byte, 0, 2+6+1+len(data)*2)

	// Message tag
	buf = append(buf, EscapeChar, msgType)

	// Timestamp (big endian) - 6 bytes
	for i := 5; i >= 0; i-- {
		b := byte((timestamp >> (8 * i)) & 0xFF)
		buf = append(buf, b)
		if b == EscapeChar {
			buf = append(buf, b) // Escape
		}
	}

	// Signal level
	buf = append(buf, signalLevel)
	if signalLevel == EscapeChar {
		buf = append(buf, signalLevel) // Escape
	}

	// Data
	for _, b := range data {
		buf = append(buf, b)
		if b == EscapeChar {
			buf = append(buf, b) // Escape
		}
	}

	return buf
}

func main() {
	port := flag.Int("port", 30005, "TCP port to listen on")
	flag.Parse()

	rand.Seed(time.Now().UnixNano())

	// Create server
	server := NewBeastServer()

	// Add some sample aircraft around San Francisco Bay Area
	server.AddAircraft(0xABCDEF, "SWA1234", 37.6188, -122.3756, 10000, 450, 45)
	server.AddAircraft(0x123456, "UAL789", 37.7749, -122.4194, 25000, 500, 270)
	server.AddAircraft(0x789ABC, "DAL456", 37.8716, -122.2727, 35000, 550, 180)
	server.AddAircraft(0x456DEF, "AAL100", 38.0100, -122.1000, 15000, 400, 135)
	server.AddAircraft(0xFEDCBA, "JBU202", 37.5000, -122.5000, 28000, 480, 90)

	// Setup signal handling for clean shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Println("\nReceived shutdown signal")
		server.Stop()
		os.Exit(0)
	}()

	// Start server
	fmt.Printf("Starting Beast server on port %d...\n", *port)
	if err := server.Start(*port); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}
