package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// Command line flags
	grpcAddr := flag.String("grpc", "localhost:10000", "gRPC server address")
	cacheDir := flag.String("cache", "tiles", "Tile cache directory")
	fullscreen := flag.Bool("fullscreen", false, "Start in fullscreen mode")
	width := flag.Int("width", 1024, "Window width")
	height := flag.Int("height", 600, "Window height")
	touchBtns := flag.Bool("touch", false, "Enable on-screen touch buttons")
	defaultLat := flag.Float64("lat", -22.9064, "Default latitude (used before GPS fix)")
	defaultLon := flag.Float64("lon", -47.0616, "Default longitude (used before GPS fix)")
	flag.Parse()

	log.Println("ELRS Ground Station Map")
	log.Printf("Connecting to gRPC backend at %s", *grpcAddr)
	log.Printf("Default location: %.4f, %.4f", *defaultLat, *defaultLon)

	// Create tile cache directory
	if err := os.MkdirAll(*cacheDir, 0755); err != nil {
		log.Fatalf("Failed to create cache directory: %v", err)
	}

	// Initialize components
	client := NewGRPCClient(*grpcAddr)
	tileManager := NewTileManager(*cacheDir)
	app := NewApp(client, tileManager, *width, *height, *fullscreen)
	app.showTouchBtns = *touchBtns
	app.centerLat = *defaultLat
	app.centerLon = *defaultLon

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Shutting down...")
		app.Shutdown()
		os.Exit(0)
	}()

	// Run the application
	if err := app.Run(); err != nil {
		log.Fatalf("Application error: %v", err)
	}
}
