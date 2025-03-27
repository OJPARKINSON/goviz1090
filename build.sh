#!/bin/bash
# build.sh - Build and run viz1090

# Set up directories
mkdir -p bin
mkdir -p font

# Check if fonts exist, download if missing
if [ ! -f "font/TerminusTTF-4.46.0.ttf" ] || [ ! -f "font/TerminusTTF-Bold-4.46.0.ttf" ]; then
    echo "Downloading fonts..."
    curl -L -o terminus-ttf.zip "https://files.ax86.net/terminus-ttf/files/terminus-ttf-4.46.0.zip"
    unzip -j terminus-ttf.zip "terminus-ttf-4.46.0/TerminusTTF-*.ttf" -d font/
    rm terminus-ttf.zip
fi

# Build the main application
echo "Building viz1090..."
go build -o bin/viz1090 cmd/viz1090/main.go

# Build the mock server if requested
if [ "$1" == "--mock" ]; then
    echo "Building mock server..."
    go build -o bin/mockserver cmd/mockserver/main.go
fi

# Run with mock server if requested
if [ "$1" == "--mock" ]; then
    echo "Starting mock server on port 30005..."
    ./bin/mockserver &
    MOCK_PID=$!

    # Give the mock server a moment to start
    sleep 1

    # Run the application with default settings
    ./bin/viz1090

    # Cleanup
    kill $MOCK_PID
else
    # Run the application with provided arguments
    ./bin/viz1090 "$@"
fi