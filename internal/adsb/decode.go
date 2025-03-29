package adsb

import (
	"math"
	"sync"
	"time"
)

// Constants for message types
const (
	DF0  = 0  // Short Air-air surveillance
	DF4  = 4  // Surveillance, altitude reply
	DF5  = 5  // Surveillance, identity reply
	DF11 = 11 // All Call reply
	DF16 = 16 // Long Air-air surveillance
	DF17 = 17 // Extended squitter
	DF18 = 18 // Extended squitter (TIS-B)
	DF20 = 20 // Comm-B, altitude reply
	DF21 = 21 // Comm-B, identity reply
)

// ADS-B Type Codes (Extended Squitter Format Type Codes)
const (
	TC_IDENT         = 4  // Aircraft identification
	TC_SURFACE_POS   = 5  // Surface position (1-8)
	TC_AIRBORNE_POS  = 9  // Airborne position (9-18)
	TC_AIRBORNE_VEL  = 19 // Airborne velocity
	TC_AIRBORNE_POS2 = 20 // Airborne position (20-22)
)

// TrailLength defines how many historical positions to keep
const TrailLength = 120

// Aircraft represents a tracked aircraft with all its information
type Aircraft struct {
	ICAO         uint32    // 24-bit ICAO address
	Flight       string    // Flight number/callsign
	Altitude     int       // Altitude in feet
	Speed        int       // Ground speed in knots
	Heading      int       // Track in degrees
	VertRate     int       // Vertical rate in ft/min
	Lat          float64   // Latitude
	Lon          float64   // Longitude
	Seen         time.Time // Last time any message was received
	SeenLatLon   time.Time // Last time position was received
	X            int       // Screen X coordinate
	Y            int       // Screen Y coordinate
	OnGround     bool      // Whether aircraft is on ground
	SignalLevel  [8]byte   // Signal strength history
	EvenCPRLat   int       // Even CPR latitude
	EvenCPRLon   int       // Even CPR longitude
	OddCPRLat    int       // Odd CPR latitude
	OddCPRLon    int       // Odd CPR longitude
	EvenCPRTime  int64     // Time of last even CPR message
	OddCPRTime   int64     // Time of last odd CPR message
	Trail        []Position
	LabelX       float64 // Label X position
	LabelY       float64 // Label Y position
	LabelW       float64 // Label width
	LabelH       float64 // Label height
	LabelDX      float64 // Label X velocity
	LabelDY      float64 // Label Y velocity
	LabelOpacity float64 // Label opacity
	LabelLevel   float64 // Label detail level (0-2)
	Messages     int     // Number of messages received
	mutex        sync.Mutex
}

// Position represents a historical position with timestamp
type Position struct {
	Lat       float64
	Lon       float64
	Altitude  int
	Heading   int
	Timestamp time.Time
}

// Message represents a decoded ADS-B message
type Message struct {
	DF          int       // Downlink Format
	CA          int       // Capability
	CF          int       // Control Field (for DF18)
	FS          int       // Flight Status (for DF4, DF5, DF20, DF21)
	ModeA       int       // Mode A code (squawk)
	ICAO        uint32    // ICAO address
	TypeCode    int       // Type code (for DF17/18)
	SubType     int       // Message subtype
	Flight      string    // Flight number/callsign
	Altitude    int       // Altitude
	Speed       int       // Velocity
	Heading     int       // Track/heading
	VertRate    int       // Vertical rate
	Lat         float64   // Decoded latitude
	Lon         float64   // Decoded longitude
	RawLat      int       // Raw latitude (CPR format)
	RawLon      int       // Raw longitude (CPR format)
	OddFlag     bool      // CPR odd/even flag
	OnGround    bool      // Aircraft is on ground
	SignalLevel byte      // Signal level
	Timestamp   time.Time // Timestamp when message was received
	CRC         uint32    // Message CRC
	IID         int       // Interrogator Identifier
	Valid       bool      // Message passed CRC check
}

// AircraftMap is a type-safe map for storing aircraft keyed by ICAO address
type AircraftMap struct {
	data  map[uint32]*Aircraft
	mutex sync.RWMutex
}

// NewAircraftMap creates a new, initialized aircraft map
func NewAircraftMap() *AircraftMap {
	return &AircraftMap{
		data: make(map[uint32]*Aircraft),
	}
}

// Get retrieves an aircraft by ICAO address, returns nil if not found
func (am *AircraftMap) Get(icao uint32) *Aircraft {
	am.mutex.RLock()
	defer am.mutex.RUnlock()
	return am.data[icao]
}

// GetOrCreate retrieves an aircraft or creates a new one if it doesn't exist
func (am *AircraftMap) GetOrCreate(icao uint32) *Aircraft {
	am.mutex.Lock()
	defer am.mutex.Unlock()

	aircraft, exists := am.data[icao]
	if !exists {
		aircraft = &Aircraft{
			ICAO:         icao,
			Seen:         time.Now(),
			Trail:        make([]Position, 0, TrailLength),
			LabelOpacity: 0,
			LabelLevel:   0,
		}
		am.data[icao] = aircraft
	}

	return aircraft
}

// Len returns the number of aircraft in the map
func (am *AircraftMap) Len() int {
	am.mutex.RLock()
	defer am.mutex.RUnlock()
	return len(am.data)
}

// ForEach executes a function for each aircraft in the map
func (am *AircraftMap) ForEach(f func(icao uint32, aircraft *Aircraft)) {
	am.mutex.RLock()
	defer am.mutex.RUnlock()

	for icao, aircraft := range am.data {
		f(icao, aircraft)
	}
}

// RemoveStale removes aircraft that haven't been seen for a while
func (am *AircraftMap) RemoveStale(ttl time.Duration) {
	am.mutex.Lock()
	defer am.mutex.Unlock()

	now := time.Now()
	for icao, aircraft := range am.data {
		if now.Sub(aircraft.Seen) > ttl {
			delete(am.data, icao)
		}
	}
}

// Copy creates a copy of the aircraft map (for thread-safe rendering)
func (am *AircraftMap) Copy() map[uint32]*Aircraft {
	am.mutex.RLock()
	defer am.mutex.RUnlock()

	copy := make(map[uint32]*Aircraft, len(am.data))
	for k, v := range am.data {
		copy[k] = v
	}

	return copy
}

// CPR (Compact Position Reporting) constants
const (
	NL_LOOKUP_TABLE_SIZE = 90
)

// NL lookup table for latitudes 0-90 degrees
var nl_table = [NL_LOOKUP_TABLE_SIZE]int{
	59, 59, 59, 59, 59, 59, 59, 59, 59, 58, 58, 58, 58, 58, 57, 57,
	57, 57, 57, 57, 56, 56, 56, 56, 56, 56, 55, 55, 55, 55, 55, 54, 54, 54, 54,
	54, 53, 53, 53, 53, 52, 52, 52, 52, 51, 51, 51, 51, 50, 50, 50, 49, 49, 49,
	48, 48, 48, 47, 47, 47, 46, 46, 46, 45, 45, 44, 44, 44, 43, 43, 42, 42, 41,
	41, 41, 40, 40, 39, 39, 38, 38, 37, 37, 36, 36, 35, 35, 34, 34, 33,
}

// cprModFunction implements the CPR modulo function
func cprModFunction(a, b int) int {
	res := a % b
	if res < 0 {
		res += b
	}
	return res
}

// cprNLFunction returns the NL lookup value for CPR calculations
func cprNLFunction(lat float64) int {
	if lat < 0 {
		lat = -lat // Table is symmetric about the equator
	}

	// Round to nearest degree
	lat = math.Round(lat)

	if lat >= NL_LOOKUP_TABLE_SIZE {
		return 1 // Very high latitudes
	}

	return nl_table[int(lat)]
}

// cprNFunction returns the number of longitude zones
func cprNFunction(lat float64, odd bool) int {
	nl := cprNLFunction(lat)
	if odd {
		return nl - 1
	}
	return nl
}

// cprDlonFunction returns the size in degrees of a longitude zone
func cprDlonFunction(lat float64, odd bool, surface bool) float64 {
	if surface {
		return 90.0 / float64(cprNFunction(lat, odd))
	}
	return 360.0 / float64(cprNFunction(lat, odd))
}

// DecodeCPRPosition decodes a pair of CPR positions to get the actual position
func DecodeCPRPosition(evenLat, evenLon, oddLat, oddLon int, lastOdd bool) (float64, float64, bool) {
	// Constants for CPR decoding
	const airDlat0 = 360.0 / 60.0
	const airDlat1 = 360.0 / 59.0

	// Convert from CPR format (0-131071) to floating point (0-1)
	rlat0 := float64(evenLat) / 131072.0
	rlat1 := float64(oddLat) / 131072.0
	rlon0 := float64(evenLon) / 131072.0
	rlon1 := float64(oddLon) / 131072.0

	// Compute the latitude index "j"
	j := int(math.Floor(((59.0*rlat0 - 60.0*rlat1) / 1.0) + 0.5))

	// Calculate global latitudes
	lat0 := airDlat0 * (float64(cprModFunction(j, 60)) + rlat0)
	lat1 := airDlat1 * (float64(cprModFunction(j, 59)) + rlat1)

	// Adjust latitudes to be in the -90 to 90 range
	if lat0 >= 270 {
		lat0 -= 360
	}
	if lat1 >= 270 {
		lat1 -= 360
	}

	// Check that both are in the same latitude zone
	if cprNLFunction(lat0) != cprNLFunction(lat1) {
		return 0, 0, false
	}

	// Determine which latitude to use based on the time of reception
	lat := lat0
	if lastOdd {
		lat = lat1
	}

	// Check latitude is in range
	if lat < -90 || lat > 90 {
		return 0, 0, false
	}

	// Compute the longitude
	var lon float64

	if lastOdd {
		// Use odd packet to calculate longitude
		ni := cprNFunction(lat, true)
		if ni == 0 {
			return 0, 0, false
		}

		m := int(math.Floor((((rlon0 * float64(cprNLFunction(lat)-1)) - (rlon1 * float64(cprNLFunction(lat)))) / 1.0) + 0.5))
		lon = cprDlonFunction(lat, true, false) * (float64(cprModFunction(m, ni)) + rlon1)
	} else {
		// Use even packet to calculate longitude
		ni := cprNFunction(lat, false)
		if ni == 0 {
			return 0, 0, false
		}

		m := int(math.Floor((((rlon0 * float64(cprNLFunction(lat)-1)) - (rlon1 * float64(cprNLFunction(lat)))) / 1.0) + 0.5))
		lon = cprDlonFunction(lat, false, false) * (float64(cprModFunction(m, ni)) + rlon0)
	}

	// Normalize longitude to -180 to 180 range
	if lon > 180 {
		lon -= 360
	}

	return lat, lon, true
}

// DecodeCallsign decodes the 8-character callsign from ADS-B data
func DecodeCallsign(data []byte) string {
	if len(data) < 6 {
		return ""
	}

	// ICAO callsign character set
	charset := "?ABCDEFGHIJKLMNOPQRSTUVWXYZ????? ???????????????0123456789??????"

	callsign := make([]byte, 8)

	// Decode first 4 characters from bytes 0-2
	bits := (uint32(data[0]) << 16) | (uint32(data[1]) << 8) | uint32(data[2])
	callsign[0] = charset[bits>>18&0x3F]
	callsign[1] = charset[bits>>12&0x3F]
	callsign[2] = charset[bits>>6&0x3F]
	callsign[3] = charset[bits&0x3F]

	// Decode last 4 characters from bytes 3-5
	bits = (uint32(data[3]) << 16) | (uint32(data[4]) << 8) | uint32(data[5])
	callsign[4] = charset[bits>>18&0x3F]
	callsign[5] = charset[bits>>12&0x3F]
	callsign[6] = charset[bits>>6&0x3F]
	callsign[7] = charset[bits&0x3F]

	// Trim trailing spaces
	i := 7
	for i >= 0 && callsign[i] == ' ' {
		i--
	}

	return string(callsign[:i+1])
}

// DecodeAltitude decodes the altitude from ADS-B data
func DecodeAltitude(data []byte) int {
	if len(data) < 7 {
		return 0
	}

	// Extract altitude bits
	ac12Field := ((uint16(data[5]) & 0x1F) << 7) | ((uint16(data[6]) & 0xFE) >> 1)

	// Check if the altitude is Gillham coded or not
	qBit := (ac12Field & 0x10) != 0

	if qBit {
		// Extract the 11-bit altitude value
		n := ((ac12Field & 0x0FE0) >> 1) | (ac12Field & 0x000F)
		return (int(n) * 25) - 1000
	}

	// Gillham coded altitude - would need more complex decoding
	// For simplicity, return 0
	return 0
}

// DecodeVelocity decodes the velocity from ADS-B data
func DecodeVelocity(data []byte) (speed, heading, vertRate int, ok bool) {
	if len(data) < 10 {
		return 0, 0, 0, false
	}

	// Get message subtype
	subtype := data[4] & 0x07

	// Only handle subtypes 1-4
	if subtype < 1 || subtype > 4 {
		return 0, 0, 0, false
	}

	// Decode vertical rate
	vertRateBit := (data[8] & 0x08) != 0
	vertRateRaw := ((int(data[8]) & 0x07) << 6) | (int(data[9]) >> 2)
	if vertRateRaw != 0 {
		vertRateRaw--
		if vertRateBit {
			vertRateRaw = -vertRateRaw
		}
		vertRate = vertRateRaw * 64
	}

	// Airborne velocity subtypes 1 & 2
	if subtype == 1 || subtype == 2 {
		// Decode East-West velocity component
		ewSign := (data[5] & 0x04) != 0
		ewRaw := ((int(data[5]) & 0x03) << 8) | int(data[6])
		ewVel := ewRaw - 1
		if ewSign {
			ewVel = -ewVel
		}

		// Decode North-South velocity component
		nsSign := (data[7] & 0x80) != 0
		nsRaw := ((int(data[7]) & 0x7F) << 3) | (int(data[8]) >> 5)
		nsVel := nsRaw - 1
		if nsSign {
			nsVel = -nsVel
		}

		// Adjust for supersonic aircraft
		if subtype == 2 {
			ewVel *= 4
			nsVel *= 4
		}

		// Calculate speed and heading from components
		speed = int(math.Sqrt(float64(ewVel*ewVel + nsVel*nsVel)))
		if speed > 0 {
			heading = int(math.Round(math.Atan2(float64(ewVel), float64(nsVel)) * 180.0 / math.Pi))
			if heading < 0 {
				heading += 360
			}
		}

		return speed, heading, vertRate, true
	}

	// Airborne velocity subtypes 3 & 4
	if subtype == 3 || subtype == 4 {
		// Decode airspeed
		airspeed := ((int(data[7]) & 0x7F) << 3) | (int(data[8]) >> 5)
		if airspeed != 0 {
			airspeed -= 1
			if subtype == 4 {
				airspeed *= 4
			}
			speed = airspeed
		}

		// Decode heading if available
		if (data[5] & 0x04) != 0 {
			hdgRaw := ((int(data[5]) & 0x03) << 8) | int(data[6])
			heading = (hdgRaw * 360) / 1024
		}

		return speed, heading, vertRate, true
	}

	return 0, 0, 0, false
}
