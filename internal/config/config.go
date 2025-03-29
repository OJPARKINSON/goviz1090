package config

// Config stores application configuration settings
type Config struct {
	// Network settings
	ServerAddress string
	ServerPort    int

	// Display settings
	ScreenWidth  int
	ScreenHeight int
	Fullscreen   bool
	UIScale      int
	Metric       bool

	// Initial map settings
	InitialLat  float64
	InitialLon  float64
	InitialZoom float64

	// Visualization options
	ShowTrails  bool
	TrailLength int
	LabelDetail int
	DisplayTTL  int

	// Debug options
	Debug bool
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		ServerAddress: "localhost",
		ServerPort:    30005,
		ScreenWidth:   0, // Auto-detect
		ScreenHeight:  0, // Auto-detect
		Fullscreen:    false,
		UIScale:       1,
		Metric:        false,
		InitialLat:    37.6188,
		InitialLon:    -122.3756,
		InitialZoom:   50.0, // NM
		ShowTrails:    true,
		TrailLength:   50,
		LabelDetail:   2,
		DisplayTTL:    30,
		Debug:         false,
	}
}
