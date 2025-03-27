package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/OJPARKINSON/viz1090/internal/app"
	"github.com/OJPARKINSON/viz1090/internal/config"
)

func main() {
	// Parse command line flags
	cfg := config.DefaultConfig()

	// Define flags
	flag.StringVar(&cfg.ServerAddress, "server", cfg.ServerAddress, "Beast server address")
	flag.IntVar(&cfg.ServerPort, "port", cfg.ServerPort, "Beast server port")
	flag.Float64Var(&cfg.InitialLat, "lat", cfg.InitialLat, "Initial latitude")
	flag.Float64Var(&cfg.InitialLon, "lon", cfg.InitialLon, "Initial longitude")
	flag.BoolVar(&cfg.Metric, "metric", cfg.Metric, "Use metric units")
	flag.BoolVar(&cfg.Fullscreen, "fullscreen", cfg.Fullscreen, "Fullscreen mode")
	flag.IntVar(&cfg.ScreenWidth, "width", cfg.ScreenWidth, "Screen width (0 = auto-detect)")
	flag.IntVar(&cfg.ScreenHeight, "height", cfg.ScreenHeight, "Screen height (0 = auto-detect)")
	flag.IntVar(&cfg.UIScale, "uiscale", cfg.UIScale, "UI scaling factor")
	flag.Float64Var(&cfg.InitialZoom, "zoom", cfg.InitialZoom, "Initial zoom level in NM")
	flag.BoolVar(&cfg.ShowTrails, "trails", cfg.ShowTrails, "Show aircraft trails")
	flag.IntVar(&cfg.TrailLength, "traillen", cfg.TrailLength, "Length of aircraft trails")
	flag.IntVar(&cfg.DisplayTTL, "ttl", cfg.DisplayTTL, "Time to display aircraft after last message (seconds)")
	flag.BoolVar(&cfg.Debug, "debug", cfg.Debug, "Enable debug output")

	// Add help flag
	helpFlag := flag.Bool("help", false, "Show help")

	flag.Parse()

	// Show help and exit if requested
	if *helpFlag {
		showHelp()
		os.Exit(0)
	}

	// Create and initialize application
	application := app.New(cfg)
	if err := application.Initialize(); err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing application: %v\n", err)
		os.Exit(1)
	}
	defer application.Cleanup()

	// Setup signal handling for clean shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println("\nReceived shutdown signal. Exiting...")
		application.Cleanup()
		os.Exit(0)
	}()

	// Run the application
	if err := application.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running application: %v\n", err)
		os.Exit(1)
	}
}

func showHelp() {
	fmt.Println(`
-----------------------------------------------------------------------------
|                        viz1090 ADSB Viewer        Ver : 1.0                |
-----------------------------------------------------------------------------
Usage: viz1090 [options]

Options:
  --server <address>      Beast server address (default: localhost)
  --port <port>           Beast server port (default: 30005)
  --lat <latitude>        Initial latitude (default: 37.6188)
  --lon <longitude>       Initial longitude (default: -122.3756)
  --metric                Use metric units
  --fullscreen            Start in fullscreen mode
  --width <pixels>        Screen width (0 = auto-detect)
  --height <pixels>       Screen height (0 = auto-detect)
  --uiscale <factor>      UI scaling factor (default: 1)
  --zoom <nm>             Initial zoom level in nautical miles (default: 50)
  --trails                Show aircraft trails (default: true)
  --traillen <points>     Length of aircraft trails (default: 50)
  --ttl <seconds>         Time to display aircraft after last message (default: 30)
  --debug                 Enable debug output
  --help                  Show this help

Keyboard Controls:
  ESC                     Exit program
  +/=                     Zoom in
  -                       Zoom out
  C                       Center on selected aircraft
  H                       Return to home location

Mouse Controls:
  Click                   Select aircraft
  Double-click            Zoom in at point
  Drag                    Pan map
  Scroll wheel            Zoom in/out
`)
}
