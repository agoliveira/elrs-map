package main

import (
	"fmt"
	"image"
	_ "image/jpeg" // Register JPEG decoder
	_ "image/png"  // Register PNG decoder
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

const (
	TileSize    = 256
	MaxZoom     = 19
	MinZoom     = 1
	DefaultZoom = 15
)

// MapSource represents the map tile source
type MapSource int

const (
	MapSourceStreet    MapSource = iota // ESRI World Street Map
	MapSourceSatellite                  // ESRI World Imagery
)

// TileCoord represents a tile coordinate
type TileCoord struct {
	X, Y, Z int
}

// TileCacheKey includes source to separate caches
type TileCacheKey struct {
	Coord  TileCoord
	Source MapSource
}

// TileManager handles map tile downloading and caching
type TileManager struct {
	cacheDir  string
	source    MapSource
	tiles     map[TileCacheKey]*ebiten.Image
	loading   map[TileCacheKey]bool
	mu        sync.RWMutex
	client    *http.Client
}

// NewTileManager creates a new tile manager
func NewTileManager(cacheDir string) *TileManager {
	return &TileManager{
		cacheDir: cacheDir,
		source:   MapSourceSatellite, // Default to satellite for FPV
		tiles:    make(map[TileCacheKey]*ebiten.Image),
		loading:  make(map[TileCacheKey]bool),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SetSource changes the map source
func (tm *TileManager) SetSource(source MapSource) {
	tm.mu.Lock()
	tm.source = source
	tm.mu.Unlock()
}

// GetSource returns the current map source
func (tm *TileManager) GetSource() MapSource {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.source
}

// ToggleSource switches between street and satellite
func (tm *TileManager) ToggleSource() MapSource {
	tm.mu.Lock()
	if tm.source == MapSourceStreet {
		tm.source = MapSourceSatellite
	} else {
		tm.source = MapSourceStreet
	}
	source := tm.source
	tm.mu.Unlock()
	return source
}

// SourceName returns human-readable source name
func (tm *TileManager) SourceName() string {
	switch tm.GetSource() {
	case MapSourceStreet:
		return "Street"
	case MapSourceSatellite:
		return "Satellite"
	default:
		return "Unknown"
	}
}

// LatLonToTile converts lat/lon to tile coordinates at given zoom
func LatLonToTile(lat, lon float64, zoom int) (int, int) {
	n := math.Pow(2, float64(zoom))
	x := int((lon + 180.0) / 360.0 * n)
	latRad := lat * math.Pi / 180.0
	y := int((1.0 - math.Asinh(math.Tan(latRad))/math.Pi) / 2.0 * n)
	return x, y
}

// LatLonToPixel converts lat/lon to pixel coordinates within a tile at given zoom
func LatLonToPixel(lat, lon float64, zoom int) (float64, float64) {
	n := math.Pow(2, float64(zoom))
	x := (lon + 180.0) / 360.0 * n * TileSize
	latRad := lat * math.Pi / 180.0
	y := (1.0 - math.Asinh(math.Tan(latRad))/math.Pi) / 2.0 * n * TileSize
	return x, y
}

// TileToLatLon converts tile coordinates to lat/lon (top-left corner)
func TileToLatLon(x, y, zoom int) (float64, float64) {
	n := math.Pow(2, float64(zoom))
	lon := float64(x)/n*360.0 - 180.0
	latRad := math.Atan(math.Sinh(math.Pi * (1 - 2*float64(y)/n)))
	lat := latRad * 180.0 / math.Pi
	return lat, lon
}

// GetTile returns a tile image, loading it if necessary
func (tm *TileManager) GetTile(coord TileCoord) *ebiten.Image {
	source := tm.GetSource()
	key := TileCacheKey{Coord: coord, Source: source}

	tm.mu.RLock()
	if tile, ok := tm.tiles[key]; ok {
		tm.mu.RUnlock()
		return tile
	}
	if tm.loading[key] {
		tm.mu.RUnlock()
		return nil
	}
	tm.mu.RUnlock()

	// Mark as loading and start async load
	tm.mu.Lock()
	tm.loading[key] = true
	tm.mu.Unlock()

	go tm.loadTile(coord, source)
	return nil
}

func (tm *TileManager) loadTile(coord TileCoord, source MapSource) {
	key := TileCacheKey{Coord: coord, Source: source}

	defer func() {
		tm.mu.Lock()
		delete(tm.loading, key)
		tm.mu.Unlock()
	}()

	// Try cache first
	img := tm.loadFromCache(coord, source)
	if img != nil {
		tm.mu.Lock()
		tm.tiles[key] = img
		tm.mu.Unlock()
		return
	}

	// Download from ESRI
	img = tm.downloadTile(coord, source)
	if img != nil {
		tm.mu.Lock()
		tm.tiles[key] = img
		tm.mu.Unlock()
	}
}

func (tm *TileManager) cachePath(coord TileCoord, source MapSource) string {
	sourceDir := "satellite"
	if source == MapSourceStreet {
		sourceDir = "street"
	}
	return filepath.Join(tm.cacheDir, sourceDir, fmt.Sprintf("%d_%d_%d.jpg", coord.Z, coord.X, coord.Y))
}

func (tm *TileManager) loadFromCache(coord TileCoord, source MapSource) *ebiten.Image {
	path := tm.cachePath(coord, source)
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	// ESRI returns JPEG for satellite, PNG for street
	img, _, err := image.Decode(f)
	if err != nil {
		return nil
	}

	return ebiten.NewImageFromImage(img)
}

func (tm *TileManager) downloadTile(coord TileCoord, source MapSource) *ebiten.Image {
	// ESRI tile URLs
	// Note: ESRI uses {z}/{y}/{x} order (not {z}/{x}/{y} like OSM)
	var url string
	switch source {
	case MapSourceStreet:
		url = fmt.Sprintf("https://server.arcgisonline.com/ArcGIS/rest/services/World_Street_Map/MapServer/tile/%d/%d/%d", coord.Z, coord.Y, coord.X)
	case MapSourceSatellite:
		url = fmt.Sprintf("https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/%d/%d/%d", coord.Z, coord.Y, coord.X)
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("Tile request error %v: %v", coord, err)
		return nil
	}
	req.Header.Set("User-Agent", "ELRS-GroundStation/1.0")

	resp, err := tm.client.Do(req)
	if err != nil {
		log.Printf("Tile download error %v: %v", coord, err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Tile HTTP error %v: status %d", coord, resp.StatusCode)
		return nil
	}

	// Read image data
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Tile read error %v: %v", coord, err)
		return nil
	}

	// Ensure cache directory exists
	cacheDir := filepath.Dir(tm.cachePath(coord, source))
	os.MkdirAll(cacheDir, 0755)

	// Save to cache
	cachePath := tm.cachePath(coord, source)
	if err := os.WriteFile(cachePath, data, 0644); err != nil {
		log.Printf("Tile cache write error %v: %v", coord, err)
	}

	// Decode for display
	img, _, err := image.Decode(NewByteReader(data))
	if err != nil {
		log.Printf("Tile decode error %v: %v", coord, err)
		return nil
	}

	return ebiten.NewImageFromImage(img)
}

// GetTilesForView returns all tile coordinates needed for the given view
func (tm *TileManager) GetTilesForView(centerLat, centerLon float64, zoom, screenW, screenH int) []TileCoord {
	// Calculate center tile
	centerX, centerY := LatLonToTile(centerLat, centerLon, zoom)

	// Calculate how many tiles we need in each direction
	tilesX := (screenW / TileSize) + 2
	tilesY := (screenH / TileSize) + 2

	var coords []TileCoord
	for dx := -tilesX / 2; dx <= tilesX/2; dx++ {
		for dy := -tilesY / 2; dy <= tilesY/2; dy++ {
			x := centerX + dx
			y := centerY + dy

			// Wrap X coordinate
			n := 1 << zoom
			x = ((x % n) + n) % n

			// Skip invalid Y
			if y < 0 || y >= n {
				continue
			}

			coords = append(coords, TileCoord{X: x, Y: y, Z: zoom})
		}
	}
	return coords
}

// ClearCache removes all cached tiles
func (tm *TileManager) ClearCache() {
	tm.mu.Lock()
	tm.tiles = make(map[TileCacheKey]*ebiten.Image)
	tm.mu.Unlock()
}

// ByteReader wraps a byte slice for io.Reader
type ByteReader struct {
	data []byte
	pos  int
}

func NewByteReader(data []byte) *ByteReader {
	return &ByteReader{data: data}
}

func (r *ByteReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
