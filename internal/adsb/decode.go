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

// DisplayTTL is time to show aircraft after last message
const DisplayTTL = 30.0 // seconds

// Aircraft represents a tracked aircraft with all its information
type Aircraft struct {
	ICAO           uint32    // 24-bit ICAO address
	Flight         string    // Flight number/callsign
	Altitude       int       // Altitude in feet
	Speed          int       // Ground speed in knots
	Heading        int       // Track in degrees
	VertRate       int       // Vertical rate in ft/min
	Lat            float64   // Latitude
	Lon            float64   // Longitude
	Seen           time.Time // Last time any message was received
	SeenLatLon     time.Time // Last time position was received
	X              int       // Screen X coordinate
	Y              int       // Screen Y coordinate
	OnGround       bool      // Whether aircraft is on ground
	SignalLevel    [8]byte   // Signal strength history
	EvenCPRLat     int       // Even CPR latitude
	EvenCPRLon     int       // Even CPR longitude
	OddCPRLat      int       // Odd CPR latitude
	OddCPRLon      int       // Odd CPR longitude
	EvenCPRTime    time.Time // Time of last even CPR message
	OddCPRTime     time.Time // Time of last odd CPR message
	Trail          []Position
	LabelX         float64   // Label X position
	LabelY         float64   // Label Y position
	LabelW         float64   // Label width
	LabelH         float64   // Label height
	LabelDX        float64   // Label X velocity
	LabelDY        float64   // Label Y velocity
	LabelOpacity   float64   // Label opacity
	LabelLevel     float64   // Label detail level (0-2)
	LastUpdateTime time.Time // Last update time for animation purposes
	mutex          sync.Mutex
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
			ICAO:           icao,
			Seen:           time.Now(),
			LastUpdateTime: time.Now(),
			Trail:          make([]Position, 0, TrailLength),
			LabelOpacity:   0,
			LabelLevel:     0,
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

// DecodeMessage decodes an ADS-B message from raw data
func DecodeMessage(data []byte, timestamp time.Time) *Message {
	if len(data) < 2 {
		return nil
	}

	// Extract DF (Downlink Format)
	df := int(data[0] >> 3)
	msg := &Message{
		DF:        df,
		Timestamp: timestamp,
		Valid:     true, // Assume valid unless we detect a problem
	}

	// Process based on DF type
	switch df {
	case DF0, DF16:
		// Short/Long air-air surveillance
		if len(data) < 2 {
			return nil
		}
		msg.OnGround = (data[0] & 0x04) != 0

	case DF4, DF20:
		// Surveillance or Comm-B, altitude reply
		if len(data) < 4 {
			return nil
		}
		msg.FS = int(data[0] & 0x07)
		msg.Altitude = decodeAC13Field(int(data[2])<<8 | int(data[3]))

		// Flight status interpretation
		msg.OnGround = (msg.FS & 0x01) != 0

	case DF5, DF21:
		// Surveillance or Comm-B, identity reply
		if len(data) < 4 {
			return nil
		}
		msg.FS = int(data[0] & 0x07)
		msg.ModeA = decodeID13Field(int(data[2])<<8 | int(data[3]))

		// Flight status interpretation
		msg.OnGround = (msg.FS & 0x01) != 0

	case DF11:
		// All Call reply
		if len(data) < 4 {
			return nil
		}
		msg.CA = int(data[0] & 0x07)
		msg.ICAO = uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3])
		// In real systems, we would check CRC and IID here

		// CA=4 indicates airborne, CA=5 indicates on ground
		if msg.CA == 4 {
			msg.OnGround = false
		} else if msg.CA == 5 {
			msg.OnGround = true
		}

	case DF17, DF18:
		// Extended squitter
		if len(data) < 14 {
			return nil
		}

		if df == DF17 {
			msg.CA = int(data[0] & 0x07)
			// CA interpretation for DF17
			if msg.CA == 4 {
				msg.OnGround = true
			} else if msg.CA == 5 {
				msg.OnGround = false
			}
		} else {
			// Control Field for DF18
			msg.CF = int(data[0] & 0x07)
		}

		msg.ICAO = uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3])

		// Decode Type Code
		msg.TypeCode = int(data[4] >> 3)
		msg.SubType = int(data[4] & 0x07)

		// Process ADS-B data based on Type Code
		switch {
		case msg.TypeCode >= 1 && msg.TypeCode <= 4:
			// Aircraft identification
			msg.Flight = decodeAircraftID(data[5:11])

		case msg.TypeCode >= 5 && msg.TypeCode <= 8:
			// Surface position
			msg.OnGround = true
			msg.RawLat = int((uint32(data[6]&0x03) << 15) | (uint32(data[7]) << 7) | (uint32(data[8]) >> 1))
			msg.RawLon = int((uint32(data[8]&0x01) << 16) | (uint32(data[9]) << 8) | uint32(data[10]))
			msg.OddFlag = (data[6] & 0x04) != 0

			// Decode ground movement
			movement := int((uint32(data[4]&0x07) << 4) | (uint32(data[5]) >> 4))
			if movement > 0 && movement < 125 {
				msg.Speed = decodeGroundMovement(movement)
			}

			// Decode heading if available
			if (data[5] & 0x08) != 0 {
				hdg := int(((uint32(data[5]&0x07)<<4)|(uint32(data[6])>>4))*45) >> 4
				msg.Heading = hdg
			}

		case msg.TypeCode >= 9 && msg.TypeCode <= 18:
			// Airborne position (barometric altitude)
			msg.Altitude = decodeAC12Field(int((uint32(data[5]) << 4) | (uint32(data[6]) >> 4)))
			msg.RawLat = int((uint32(data[6]&0x03) << 15) | (uint32(data[7]) << 7) | (uint32(data[8]) >> 1))
			msg.RawLon = int((uint32(data[8]&0x01) << 16) | (uint32(data[9]) << 8) | uint32(data[10]))
			msg.OddFlag = (data[6] & 0x04) != 0

		case msg.TypeCode == 19:
			// Airborne velocity
			subtype := int(data[4] & 0x07)

			if subtype == 1 || subtype == 2 {
				// Subtype 1 and 2: Ground speed
				ewSign := (data[5] & 0x04) >> 2
				ewVel := int((uint32(data[5]&0x03) << 8) | uint32(data[6]))
				nsSign := (data[7] & 0x80) >> 7
				nsVel := int((uint32(data[7]&0x7F) << 3) | (uint32(data[8]) >> 5))

				// Apply direction
				if ewVel > 0 {
					ewVel -= 1
					if ewSign != 0 {
						ewVel = -ewVel
					}
				}

				if nsVel > 0 {
					nsVel -= 1
					if nsSign != 0 {
						nsVel = -nsVel
					}
				}

				// Speed multiplier for supersonic aircraft
				if subtype == 2 {
					ewVel *= 4
					nsVel *= 4
				}

				// Calculate speed and heading from vectors
				if ewVel != 0 || nsVel != 0 {
					msg.Speed = int(math.Sqrt(float64(ewVel*ewVel + nsVel*nsVel)))
					msg.Heading = int(math.Round(math.Atan2(float64(ewVel), float64(nsVel)) * 180 / math.Pi))
					if msg.Heading < 0 {
						msg.Heading += 360
					}
				}

				// Vertical rate
				vRateSign := int((data[8] & 0x08) >> 3)
				vRateData := int((uint32(data[8]&0x07) << 6) | (uint32(data[9]) >> 2))
				if vRateData != 0 {
					vRate := (vRateData - 1) * 64
					if vRateSign != 0 {
						vRate = -vRate
					}
					msg.VertRate = vRate
				}
			} else if subtype == 3 || subtype == 4 {
				// Subtype 3 and 4: Airspeed
				airspeed := int((uint32(data[7]&0x7f) << 3) | (uint32(data[8]) >> 5))
				if airspeed > 0 {
					airspeed -= 1
					if subtype == 4 {
						airspeed *= 4
					}
					msg.Speed = airspeed
				}

				// Heading
				if (data[5] & 0x04) != 0 {
					hdg := int(((uint32(data[5]&0x03)<<8)|uint32(data[6]))*45) >> 7
					msg.Heading = hdg
				}

				// Vertical rate (same as subtype 1/2)
				vRateSign := int((data[8] & 0x08) >> 3)
				vRateData := int((uint32(data[8]&0x07) << 6) | (uint32(data[9]) >> 2))
				if vRateData != 0 {
					vRate := (vRateData - 1) * 64
					if vRateSign != 0 {
						vRate = -vRate
					}
					msg.VertRate = vRate
				}
			}
		}
	}

	return msg
}

// decodeAC12Field decodes the 12-bit AC altitude field
func decodeAC12Field(ac12Field int) int {
	if ac12Field == 0 {
		return 0
	}

	// Q bit (bit 4) set = 25ft encoding, clear = Gillham Mode C encoding
	qBit := ac12Field & 0x10

	if qBit != 0 {
		// Altitude is represented by bits 1-4 and 6-12 (removing Q bit at bit 5)
		n := ((ac12Field & 0x0FE0) >> 1) | (ac12Field & 0x000F)
		return (n * 25) - 1000
	} else {
		// Gillham encoded altitude
		return gillhamToAltitude(ac12Field)
	}
}

// decodeAC13Field decodes the 13-bit AC altitude field
func decodeAC13Field(ac13Field int) int {
	if ac13Field == 0 {
		return 0
	}

	// M bit (bit 7) set = meters, clear = feet
	mBit := ac13Field & 0x0040

	if mBit == 0 {
		// Altitude in feet

		// Q bit (bit 5) set = 25ft encoding, clear = Gillham Mode C encoding
		qBit := ac13Field & 0x0010

		if qBit != 0 {
			// N is the 11-bit integer (removing M and Q bits)
			n := ((ac13Field & 0x1F80) >> 2) | ((ac13Field & 0x0020) >> 1) | (ac13Field & 0x000F)
			return (n * 25) - 1000
		} else {
			// Gillham encoded altitude
			return gillhamToAltitude(ac13Field)
		}
	} else {
		// Altitude in meters (simplified)
		return 0
	}
}

// gillhamToAltitude converts Gillham encoded altitude to feet
func gillhamToAltitude(code int) int {
	// Simplification - in real systems this is more complex
	return code * 100
}

// decodeID13Field decodes the Mode A 13-bit identity code
func decodeID13Field(id13Field int) int {
	hex := 0

	// Gillham coding (C1-A1-C2-A2-C4-A4-X-B1-D1-B2-D2-B4-D4)
	if id13Field&0x1000 != 0 {
		hex |= 0x0010
	} // C1
	if id13Field&0x0800 != 0 {
		hex |= 0x1000
	} // A1
	if id13Field&0x0400 != 0 {
		hex |= 0x0020
	} // C2
	if id13Field&0x0200 != 0 {
		hex |= 0x2000
	} // A2
	if id13Field&0x0100 != 0 {
		hex |= 0x0040
	} // C4
	if id13Field&0x0080 != 0 {
		hex |= 0x4000
	} // A4
	// Bit 6 is not used
	if id13Field&0x0020 != 0 {
		hex |= 0x0100
	} // B1
	if id13Field&0x0010 != 0 {
		hex |= 0x0001
	} // D1
	if id13Field&0x0008 != 0 {
		hex |= 0x0200
	} // B2
	if id13Field&0x0004 != 0 {
		hex |= 0x0002
	} // D2
	if id13Field&0x0002 != 0 {
		hex |= 0x0400
	} // B4
	if id13Field&0x0001 != 0 {
		hex |= 0x0004
	} // D4

	return hex
}

// decodeGroundMovement decodes ground movement field
func decodeGroundMovement(movement int) int {
	var speed int

	// Movement codes 0, 125, 126, 127 are invalid
	if movement <= 0 || movement >= 125 {
		return 0
	}

	// Exponential scale
	if movement > 123 {
		speed = 199 // > 175kt
	} else if movement > 108 {
		speed = ((movement - 108) * 5) + 100
	} else if movement > 93 {
		speed = ((movement - 93) * 2) + 70
	} else if movement > 38 {
		speed = (movement - 38) + 15
	} else if movement > 12 {
		speed = ((movement - 11) >> 1) + 2
	} else if movement > 8 {
		speed = ((movement - 6) >> 2) + 1
	} else {
		speed = 0
	}

	return speed
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

	if lat < 10.47047130 {
		return 59
	}
	if lat < 14.82817437 {
		return 58
	}
	if lat < 18.18626357 {
		return 57
	}
	if lat < 21.02939493 {
		return 56
	}
	if lat < 23.54504487 {
		return 55
	}
	if lat < 25.82924707 {
		return 54
	}
	if lat < 27.93898710 {
		return 53
	}
	if lat < 29.91135686 {
		return 52
	}
	if lat < 31.77209708 {
		return 51
	}
	if lat < 33.53993436 {
		return 50
	}
	if lat < 35.22899598 {
		return 49
	}
	if lat < 36.85025108 {
		return 48
	}
	if lat < 38.41241892 {
		return 47
	}
	if lat < 39.92256684 {
		return 46
	}
	if lat < 41.38651832 {
		return 45
	}
	if lat < 42.80914012 {
		return 44
	}
	if lat < 44.19454951 {
		return 43
	}
	if lat < 45.54626723 {
		return 42
	}
	if lat < 46.86733252 {
		return 41
	}
	if lat < 48.16039128 {
		return 40
	}
	if lat < 49.42776439 {
		return 39
	}
	if lat < 50.67150166 {
		return 38
	}
	if lat < 51.89342469 {
		return 37
	}
	if lat < 53.09516153 {
		return 36
	}
	if lat < 54.27817472 {
		return 35
	}
	if lat < 55.44378444 {
		return 34
	}
	if lat < 56.59318756 {
		return 33
	}
	if lat < 57.72747354 {
		return 32
	}
	if lat < 58.84763776 {
		return 31
	}
	if lat < 59.95459277 {
		return 30
	}
	if lat < 61.04917774 {
		return 29
	}
	if lat < 62.13216659 {
		return 28
	}
	if lat < 63.20427479 {
		return 27
	}
	if lat < 64.26616523 {
		return 26
	}
	if lat < 65.31845310 {
		return 25
	}
	if lat < 66.36171008 {
		return 24
	}
	if lat < 67.39646774 {
		return 23
	}
	if lat < 68.42322022 {
		return 22
	}
	if lat < 69.44242631 {
		return 21
	}
	if lat < 70.45451075 {
		return 20
	}
	if lat < 71.45986473 {
		return 19
	}
	if lat < 72.45884545 {
		return 18
	}
	if lat < 73.45177442 {
		return 17
	}
	if lat < 74.43893416 {
		return 16
	}
	if lat < 75.42056257 {
		return 15
	}
	if lat < 76.39684391 {
		return 14
	}
	if lat < 77.36789461 {
		return 13
	}
	if lat < 78.33374083 {
		return 12
	}
	if lat < 79.29428225 {
		return 11
	}
	if lat < 80.24923213 {
		return 10
	}
	if lat < 81.19801349 {
		return 9
	}
	if lat < 82.13956981 {
		return 8
	}
	if lat < 83.07199445 {
		return 7
	}
	if lat < 83.99173563 {
		return 6
	}
	if lat < 84.89166191 {
		return 5
	}
	if lat < 85.75541621 {
		return 4
	}
	if lat < 86.53536998 {
		return 3
	}
	if lat < 87.00000000 {
		return 2
	}
	return 1
}

// cprN returns the number of zones at a specific latitude
func cprN(lat float64, odd bool) int {
	nl := cprNLFunction(lat)
	if !odd {
		return nl
	}
	return nl - 1
}

// cprDlon returns the size in degrees of a longitude zone
func cprDlon(lat float64, odd bool) float64 {
	return 360.0 / float64(cprN(lat, odd))
}

// DecodeCPRPosition decodes a pair of CPR positions to get the actual position
func DecodeCPRPosition(evenPos, oddPos *Aircraft) (float64, float64, bool) {
	// Check if we have both even and odd position
	if evenPos.EvenCPRTime.IsZero() || oddPos.OddCPRTime.IsZero() {
		return 0, 0, false
	}

	// Check if the messages are received within 10 seconds
	if math.Abs(float64(evenPos.EvenCPRTime.Sub(oddPos.OddCPRTime).Milliseconds())) > 10000 {
		return 0, 0, false
	}

	// CPR latitude values
	rlat0 := float64(evenPos.EvenCPRLat) / 131072.0
	rlat1 := float64(oddPos.OddCPRLat) / 131072.0

	// CPR longitude values
	rlon0 := float64(evenPos.EvenCPRLon) / 131072.0
	rlon1 := float64(oddPos.OddCPRLon) / 131072.0

	// Compute the latitude index "j"
	airDlat0 := 360.0 / 60.0
	airDlat1 := 360.0 / 59.0

	j := math.Floor(59.0*rlat0 - 60.0*rlat1 + 0.5)

	// Compute the final latitude
	lat0 := airDlat0 * (float64(cprModFunction(int(j), 60)) + rlat0)
	lat1 := airDlat1 * (float64(cprModFunction(int(j), 59)) + rlat1)

	// Adjust latitudes to -90..+90 range
	if lat0 >= 270 {
		lat0 -= 360
	}
	if lat1 >= 270 {
		lat1 -= 360
	}

	// Check latitude range
	if lat0 < -90 || lat0 > 90 || lat1 < -90 || lat1 > 90 {
		return 0, 0, false
	}

	// Check that both are in same latitude zone
	if cprNLFunction(lat0) != cprNLFunction(lat1) {
		return 0, 0, false
	}

	// Determine which latitude to use based on time
	lat := lat0
	usesOdd := false
	if oddPos.OddCPRTime.After(evenPos.EvenCPRTime) {
		lat = lat1
		usesOdd = true
	}

	// Compute the longitude
	var lon float64

	// Get NL(lat)
	ni := cprN(lat, usesOdd)

	if ni != 0 {
		// Compute longitude index "m"
		m := math.Floor((float64(evenPos.EvenCPRLon)*(float64(cprNLFunction(lat)-1))-
			float64(oddPos.OddCPRLon)*float64(cprNLFunction(lat)))/131072.0 + 0.5)

		// Calculate longitude
		if usesOdd {
			lon = cprDlon(lat, true) * (float64(cprModFunction(int(m), ni)) + rlon1)
		} else {
			lon = cprDlon(lat, false) * (float64(cprModFunction(int(m), ni)) + rlon0)
		}

		// Normalize to -180..+180
		if lon > 180 {
			lon -= 360
		}
	} else {
		return 0, 0, false
	}

	return lat, lon, true
}

// ProcessMessage updates an aircraft's data based on a new ADS-B message
func ProcessMessage(aircraft *Aircraft, msg *Message) {
	// Update last seen time
	aircraft.Seen = msg.Timestamp

	// Update aircraft identification if present
	if msg.Flight != "" {
		aircraft.Flight = msg.Flight
	}

	// Update position related data
	switch msg.DF {
	case DF17, DF18:
		switch {
		case msg.TypeCode >= 1 && msg.TypeCode <= 4:
			// Aircraft identification
			aircraft.Flight = msg.Flight

		case msg.TypeCode >= 5 && msg.TypeCode <= 8:
			// Surface position
			aircraft.OnGround = true
			if msg.Speed > 0 {
				aircraft.Speed = msg.Speed
			}
			if msg.Heading > 0 {
				aircraft.Heading = msg.Heading
			}

			// Store CPR position
			if msg.OddFlag {
				aircraft.OddCPRLat = msg.RawLat
				aircraft.OddCPRLon = msg.RawLon
				aircraft.OddCPRTime = msg.Timestamp
			} else {
				aircraft.EvenCPRLat = msg.RawLat
				aircraft.EvenCPRLon = msg.RawLon
				aircraft.EvenCPRTime = msg.Timestamp
			}

			// Try to decode position if we have both odd and even messages
			aircraft.mutex.Lock()
			if !aircraft.EvenCPRTime.IsZero() && !aircraft.OddCPRTime.IsZero() {
				lat, lon, ok := DecodeCPRPosition(aircraft, aircraft) // Same aircraft for both parameters
				if ok {
					aircraft.Lat = lat
					aircraft.Lon = lon
					aircraft.SeenLatLon = msg.Timestamp

					// Add to position history trail
					addToTrail(aircraft, lat, lon)
				}
			}
			aircraft.mutex.Unlock()

		case msg.TypeCode >= 9 && msg.TypeCode <= 18, msg.TypeCode >= 20 && msg.TypeCode <= 22:
			// Airborne position
			aircraft.OnGround = false
			if msg.Altitude > 0 {
				aircraft.Altitude = msg.Altitude
			}

			// Store CPR position
			if msg.OddFlag {
				aircraft.OddCPRLat = msg.RawLat
				aircraft.OddCPRLon = msg.RawLon
				aircraft.OddCPRTime = msg.Timestamp
			} else {
				aircraft.EvenCPRLat = msg.RawLat
				aircraft.EvenCPRLon = msg.RawLon
				aircraft.EvenCPRTime = msg.Timestamp
			}

			// Try to decode position if we have both odd and even messages
			aircraft.mutex.Lock()
			if !aircraft.EvenCPRTime.IsZero() && !aircraft.OddCPRTime.IsZero() {
				lat, lon, ok := DecodeCPRPosition(aircraft, aircraft) // Same aircraft for both parameters
				if ok {
					aircraft.Lat = lat
					aircraft.Lon = lon
					aircraft.SeenLatLon = msg.Timestamp

					// Add to position history trail
					addToTrail(aircraft, lat, lon)
				}
			}
			aircraft.mutex.Unlock()

		case msg.TypeCode == 19:
			// Airborne velocity
			if msg.Speed > 0 {
				aircraft.Speed = msg.Speed
			}
			if msg.Heading > 0 {
				aircraft.Heading = msg.Heading
			}
			if msg.VertRate != 0 {
				aircraft.VertRate = msg.VertRate
			}
		}

	case DF4, DF20:
		// Altitude
		aircraft.Altitude = msg.Altitude
		aircraft.OnGround = msg.OnGround

	case DF5, DF21:
		// Identity (squawk)
		aircraft.OnGround = msg.OnGround

	case DF0, DF16:
		// Air-air surveillance
		aircraft.OnGround = msg.OnGround

	case DF11:
		// All-call reply
		if msg.CA == 4 {
			aircraft.OnGround = true
		} else if msg.CA == 5 {
			aircraft.OnGround = false
		}
	}

	// Update signal level by rotating history
	aircraft.mutex.Lock()
	// Shift signal levels
	for i := 7; i > 0; i-- {
		aircraft.SignalLevel[i] = aircraft.SignalLevel[i-1]
	}
	aircraft.SignalLevel[0] = msg.SignalLevel
	aircraft.mutex.Unlock()
}

// addToTrail adds a new position to the aircraft's trail
func addToTrail(aircraft *Aircraft, lat, lon float64) {
	// Create a new position
	pos := Position{
		Lat:       lat,
		Lon:       lon,
		Altitude:  aircraft.Altitude,
		Heading:   aircraft.Heading,
		Timestamp: time.Now(),
	}

	// Ensure we don't exceed the maximum trail length
	if len(aircraft.Trail) >= TrailLength {
		// Remove oldest position
		aircraft.Trail = aircraft.Trail[1:]
	}

	// Add new position
	aircraft.Trail = append(aircraft.Trail, pos)
}

// decodeAircraftID decodes the aircraft identification from ADS-B data
func decodeAircraftID(data []byte) string {
	if len(data) < 6 {
		return ""
	}

	var result [8]byte
	charset := "?ABCDEFGHIJKLMNOPQRSTUVWXYZ????? ???????????????0123456789??????"

	// Decode first 4 characters from bytes 0-2
	bits := (uint64(data[0]) << 16) | (uint64(data[1]) << 8) | uint64(data[2])
	result[0] = charset[bits>>18&0x3F]
	result[1] = charset[bits>>12&0x3F]
	result[2] = charset[bits>>6&0x3F]
	result[3] = charset[bits&0x3F]

	// Decode last 4 characters from bytes 3-5
	bits = (uint64(data[3]) << 16) | (uint64(data[4]) << 8) | uint64(data[5])
	result[4] = charset[bits>>18&0x3F]
	result[5] = charset[bits>>12&0x3F]
	result[6] = charset[bits>>6&0x3F]
	result[7] = charset[bits&0x3F]

	// Trim trailing spaces
	i := 7
	for i >= 0 && result[i] == ' ' {
		i--
	}

	return string(result[:i+1])
}
