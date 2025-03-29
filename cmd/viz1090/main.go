package main

import (
	"fmt"
	"math"
	"os"
	"time"

	"github.com/veandco/go-sdl2/sdl"
	"github.com/veandco/go-sdl2/ttf"
)

func main() {
	fmt.Println("Starting advanced viz1090 test...")

	// Initialize SDL
	if err := sdl.Init(sdl.INIT_VIDEO); err != nil {
		fmt.Printf("Failed to initialize SDL: %v\n", err)
		os.Exit(1)
	}
	defer sdl.Quit()

	// Initialize TTF
	if err := ttf.Init(); err != nil {
		fmt.Printf("Failed to initialize TTF: %v\n", err)
		sdl.Quit()
		os.Exit(1)
	}
	defer ttf.Quit()

	// Create window and renderer
	window, err := sdl.CreateWindow(
		"viz1090 Advanced Test",
		sdl.WINDOWPOS_CENTERED,
		sdl.WINDOWPOS_CENTERED,
		800,
		600,
		sdl.WINDOW_SHOWN,
	)
	if err != nil {
		fmt.Printf("Failed to create window: %v\n", err)
		os.Exit(1)
	}
	defer window.Destroy()

	renderer, err := sdl.CreateRenderer(window, -1, sdl.RENDERER_ACCELERATED)
	if err != nil {
		fmt.Printf("Failed to create renderer: %v\n", err)
		os.Exit(1)
	}
	defer renderer.Destroy()

	// Create a texture for map caching
	mapTexture, err := renderer.CreateTexture(
		sdl.PIXELFORMAT_RGBA8888,
		sdl.TEXTUREACCESS_TARGET,
		800, 600)
	if err != nil {
		fmt.Printf("Failed to create map texture: %v\n", err)
		os.Exit(1)
	}
	defer mapTexture.Destroy()

	// Load fonts
	regularFont, err := ttf.OpenFont("font/TerminusTTF-4.46.0.ttf", 12)
	if err != nil {
		fmt.Printf("Failed to load regular font: %v\n", err)
		os.Exit(1)
	}
	defer regularFont.Close()

	boldFont, err := ttf.OpenFont("font/TerminusTTF-Bold-4.46.0.ttf", 12)
	if err != nil {
		fmt.Printf("Failed to load bold font: %v\n", err)
		os.Exit(1)
	}
	defer boldFont.Close()

	// Simulate aircraft data
	type Aircraft struct {
		ICAO    uint32
		Lat     float64
		Lon     float64
		Alt     int
		Heading int
		X       int
		Y       int
	}

	aircraft := []*Aircraft{
		{ICAO: 0xABCDEF, Lat: 37.6188, Lon: -122.3756, Alt: 10000, Heading: 45},
		{ICAO: 0x123456, Lat: 37.7749, Lon: -122.4194, Alt: 25000, Heading: 270},
		{ICAO: 0x789ABC, Lat: 37.8716, Lon: -122.2727, Alt: 35000, Heading: 180},
	}

	// Generate texture for map background
	renderer.SetRenderTarget(mapTexture)
	renderer.SetDrawColor(0, 0, 0, 255)
	renderer.Clear()

	// Draw grid for map
	renderer.SetDrawColor(33, 0, 122, 255) // Map color
	for i := 0; i < 800; i += 50 {
		renderer.DrawLine(int32(i), 0, int32(i), 600)
		renderer.DrawLine(0, int32(i), 800, int32(i))
	}

	renderer.SetRenderTarget(nil)

	// Main loop
	running := true
	frameCount := 0
	startTime := time.Now()
	centerLat := 37.6188
	centerLon := -122.3756

	fmt.Println("Starting main loop...")

	for running {
		// Process events
		for event := sdl.PollEvent(); event != nil; event = sdl.PollEvent() {
			switch e := event.(type) {
			case *sdl.QuitEvent:
				running = false
				break
			case *sdl.KeyboardEvent:
				if e.Keysym.Sym == sdl.K_ESCAPE {
					running = false
				}
			}
		}

		// Calculate aircraft screen positions
		for _, a := range aircraft {
			dx := (a.Lon - centerLon) * 5000
			dy := (a.Lat - centerLat) * 5000
			a.X = 400 + int(dx)
			a.Y = 300 - int(dy)
		}

		// Clear screen
		renderer.SetDrawColor(0, 0, 0, 255)
		renderer.Clear()

		// Draw map background
		renderer.Copy(mapTexture, nil, nil)

		// Draw aircraft
		for _, a := range aircraft {
			// Draw aircraft symbol
			renderer.SetDrawColor(253, 250, 31, 255) // Yellow

			// Convert heading to radians and calculate direction vectors
			headingRad := float64(a.Heading) * math.Pi / 180.0
			dirX := int(15.0 * math.Sin(headingRad))
			dirY := int(-15.0 * math.Cos(headingRad))

			// Draw aircraft shape
			renderer.DrawLine(int32(a.X), int32(a.Y), int32(a.X+dirX), int32(a.Y+dirY))
			renderer.DrawLine(int32(a.X-dirY/2), int32(a.Y-dirX/2), int32(a.X+dirY/2), int32(a.Y+dirX/2))

			// Draw text label
			text := fmt.Sprintf("%06X %d'", a.ICAO, a.Alt)
			surface, _ := regularFont.RenderUTF8Solid(text, sdl.Color{R: 255, G: 255, B: 255, A: 255})
			if surface != nil {
				texture, _ := renderer.CreateTextureFromSurface(surface)
				if texture != nil {
					renderer.Copy(texture, nil, &sdl.Rect{X: int32(a.X + 10), Y: int32(a.Y - 15), W: surface.W, H: surface.H})
					texture.Destroy()
				}
				surface.Free()
			}
		}

		// Draw status info
		statusText := fmt.Sprintf("Aircraft: %d  Center: %.4f, %.4f", len(aircraft), centerLat, centerLon)
		surface, _ := boldFont.RenderUTF8Solid(statusText, sdl.Color{R: 196, G: 196, B: 196, A: 255})
		if surface != nil {
			texture, _ := renderer.CreateTextureFromSurface(surface)
			if texture != nil {
				renderer.Copy(texture, nil, &sdl.Rect{X: 10, Y: 570, W: surface.W, H: surface.H})
				texture.Destroy()
			}
			surface.Free()
		}

		// Present renderer
		renderer.Present()

		// Report FPS occasionally
		frameCount++
		if frameCount%60 == 0 {
			elapsed := time.Since(startTime).Seconds()
			fmt.Printf("FPS: %.2f\n", float64(frameCount)/elapsed)
		}

		sdl.Delay(16) // ~60 FPS
	}

	fmt.Println("Application exited normally")
}
