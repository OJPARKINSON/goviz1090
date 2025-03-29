# viz1090-go

A Go-based ADS-B aircraft visualization tool with interactive map display.

## Features

- Real-time display of aircraft positions, altitude, speed, and other data
- Geographic map with coastlines, borders, and airports
- Interactive interface with zoom, pan, and aircraft selection
- Smart label placement with collision avoidance
- Aircraft trails for tracking movement history
- Connect to any Beast format data provider (like dump1090)
- Cross-platform support (Linux, macOS including M1/M2, Windows)

## Installation

### Prerequisites

#### On Linux

```bash
# Install dependencies
sudo apt-get install libsdl2-dev libsdl2-ttf-dev
```

#### On macOS

```bash
# Install dependencies with Homebrew
brew install sdl2 sdl2_ttf go
```

#### On Windows

Install Go from https://golang.org/dl/
Install MSYS2 from https://www.msys2.org/ and the SDL2 development libraries

### Building

```bash
# Clone the repository
git clone https://github.com/OJPARKINSON/viz1090.git
cd viz1090

# Build the application
go build -o bin/viz1090 cmd/viz1090/main.go
```

## Running

### With a real ADS-B receiver

```bash
# If dump1090 is running on localhost
./bin/viz1090

# If dump1090 is running on another machine
./bin/viz1090 --server 192.168.1.10
```

### With the built-in simulator

```bash
# Build and run the simulator
go build -o bin/mockserver cmd/mockserver/main.go
./bin/mockserver &
./bin/viz1090
```

## Command Line Options

```
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
```

## Controls

### Keyboard

- **ESC**: Exit program
- **+/=**: Zoom in
- **-**: Zoom out

### Mouse

- **Click**: Select aircraft
- **Double-click**: Zoom in at point
- **Drag**: Pan map
- **Scroll wheel**: Zoom in/out

## Map Data

The application uses map data in a custom binary format for efficient storage and rendering. To generate map data:

1. Install Python and required libraries:

```bash
pip install fiona shapely tqdm
```

2. Run the map converter:

```bash
./scripts/getmap.sh
```

Pre-generated map data files are included in the repository for convenience.

## Credits

This project is inspired by the original viz1090 by Nathan Matsuda and the dump1090 project by Salvatore Sanfilippo and Malcolm Robb.
