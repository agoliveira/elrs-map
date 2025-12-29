package main

import (
	"fmt"
	"image/color"
	"log"
	"math"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// App is the main application
type App struct {
	client         *GRPCClient
	tileManager    *TileManager
	cockpitHUD     *CockpitHUD
	osd            *OSD
	panel          *Panel
	touchControls  *TouchControls
	gpioController *GPIOController

	// View state
	centerLat  float64
	centerLon  float64
	zoom       int
	width      int
	height     int
	fullscreen bool

	// HUD mode: 0=full map, 1=OSD overlay, 2=Panel+map
	hudMode       int
	showTouchBtns bool

	// Home position
	homeLat    float64
	homeLon    float64
	homeSet    bool

	// Flight path history
	flightPath []struct{ lat, lon float64 }
	maxPathLen int

	// UI state
	showHelp     bool
	selectedPort int
	ports        []string
	lastPortScan time.Time

	// Dragging
	dragging   bool
	dragStartX int
	dragStartY int
	dragLat    float64
	dragLon    float64

	// Auto-follow aircraft
	followAircraft bool
}

// NewApp creates a new application
func NewApp(client *GRPCClient, tileManager *TileManager, width, height int, fullscreen bool) *App {
	app := &App{
		client:         client,
		tileManager:    tileManager,
		cockpitHUD:     NewCockpitHUD(),
		osd:            NewOSD(),
		panel:          NewPanel(),
		touchControls:  NewTouchControls(),
		gpioController: NewGPIOController(),
		centerLat:      -22.9064,  // Default: Campinas, Brazil
		centerLon:      -47.0616,
		zoom:           DefaultZoom,
		width:          width,
		height:         height,
		fullscreen:     fullscreen,
		maxPathLen:     1000,
		followAircraft: true,
		showHelp:       false,
		hudMode:        2, // Default to Panel+map
		showTouchBtns:  false,
	}
	// Setup touch buttons (still available if enabled)
	app.touchControls.SetupDefaultButtons(app)
	// Setup GPIO buttons
	app.gpioController.SetupDefaultButtons(app)
	return app
}

// Run starts the application
func (a *App) Run() error {
	ebiten.SetWindowSize(a.width, a.height)
	ebiten.SetWindowTitle("ELRS Ground Station")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

	if a.fullscreen {
		ebiten.SetFullscreen(true)
	}

	// Connect to gRPC backend
	if err := a.client.Connect(); err != nil {
		log.Printf("Warning: Could not connect to backend: %v", err)
	} else {
		a.client.StartTelemetryStream()
	}

	// Start GPIO controller (will auto-detect if on Pi)
	if err := a.gpioController.Start(); err != nil {
		log.Printf("GPIO controller error: %v", err)
	}

	return ebiten.RunGame(a)
}

// Shutdown cleans up resources
func (a *App) Shutdown() {
	a.gpioController.Stop()
	a.client.StopTelemetryStream()
	a.client.StopLink()
	a.client.Disconnect()
}

// Update handles input and logic updates
func (a *App) Update() error {
	// Get current screen size
	a.width, a.height = ebiten.WindowSize()

	// Handle touch input first (before keyboard to allow touch override)
	if a.showTouchBtns {
		a.touchControls.UpdateLayout(a.width, a.height)
		a.touchControls.Update()
		a.touchControls.UpdateButtonStates(a)
	}

	// Handle keyboard input
	a.handleKeyboard()

	// Handle mouse input
	a.handleMouse()

	// Update port list periodically
	if time.Since(a.lastPortScan) > 2*time.Second {
		a.scanPorts()
		a.lastPortScan = time.Now()
	}

	// Update flight path and follow aircraft
	state := a.client.GetState()
	if state.HasGPS && state.Latitude != 0 && state.Longitude != 0 {
		// Add to flight path
		a.flightPath = append(a.flightPath, struct{ lat, lon float64 }{
			lat: float64(state.Latitude),
			lon: float64(state.Longitude),
		})
		if len(a.flightPath) > a.maxPathLen {
			a.flightPath = a.flightPath[1:]
		}

		// Follow aircraft
		if a.followAircraft {
			a.centerLat = float64(state.Latitude)
			a.centerLon = float64(state.Longitude)
		}
	}

	return nil
}

// Draw renders the application
func (a *App) Draw(screen *ebiten.Image) {
	// Clear screen
	screen.Fill(color.RGBA{30, 30, 30, 255})

	// Calculate map offset based on HUD mode
	mapOffsetX := 0
	if a.hudMode == 2 {
		mapOffsetX = a.panel.GetPanelWidth()
	}

	// Draw map tiles (with offset for panel mode)
	a.drawMapWithOffset(screen, mapOffsetX)

	// Draw flight path
	a.drawFlightPathWithOffset(screen, mapOffsetX)

	// Draw home marker
	a.drawHomeMarkerWithOffset(screen, mapOffsetX)

	// Draw aircraft
	a.drawAircraftWithOffset(screen, mapOffsetX)

	// Get telemetry state for HUD
	state := a.client.GetState()
	homeDist := 0.0
	homeBearing := 0.0
	if a.homeSet && state.HasGPS {
		homeDist = a.calculateDistance(float64(state.Latitude), float64(state.Longitude), a.homeLat, a.homeLon)
		homeBearing = a.calculateBearing(float64(state.Latitude), float64(state.Longitude), a.homeLat, a.homeLon)
	}

	// Draw HUD based on mode
	switch a.hudMode {
	case 0: // Full map only - no overlay
		// Just show minimal status in corner
		a.drawMinimalStatus(screen, state)
	case 1: // OSD overlay on full map
		a.osd.Draw(screen, state, a.homeSet, homeDist, homeBearing)
	case 2: // Panel + map
		a.panel.Draw(screen, state, a.homeSet, homeDist, homeBearing)
	}

	// Draw help overlay
	if a.showHelp {
		a.drawHelp(screen)
	}

	// Draw touch buttons
	if a.showTouchBtns {
		a.touchControls.Draw(screen)
	}

	// Draw status bar
	a.drawStatusBar(screen)
}

// drawMinimalStatus draws minimal info for full-map mode
func (a *App) drawMinimalStatus(screen *ebiten.Image, state TelemetryState) {
	// Small semi-transparent box in top-left
	vector.DrawFilledRect(screen, 5, 5, 200, 35, color.RGBA{0, 0, 0, 180}, true)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%.5f, %.5f", state.Latitude, state.Longitude), 10, 8)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("ALT:%dm SPD:%.0fkm/h", state.Altitude, state.GroundSpeed), 10, 22)
}

// drawMapWithOffset draws map tiles with X offset for panel
func (a *App) drawMapWithOffset(screen *ebiten.Image, offsetX int) {
	// Get visible tiles
	mapWidth := a.width - offsetX
	coords := a.tileManager.GetTilesForView(a.centerLat, a.centerLon, a.zoom, mapWidth, a.height)

	// Calculate center pixel position
	centerPixelX, centerPixelY := LatLonToPixel(a.centerLat, a.centerLon, a.zoom)

	// Screen center (adjusted for panel offset)
	screenCenterX := float64(offsetX + mapWidth/2)
	screenCenterY := float64(a.height / 2)

	for _, coord := range coords {
		tile := a.tileManager.GetTile(coord)
		if tile == nil {
			// Draw placeholder
			tilePixelX := float64(coord.X * TileSize)
			tilePixelY := float64(coord.Y * TileSize)
			screenX := screenCenterX + (tilePixelX - centerPixelX)
			screenY := screenCenterY + (tilePixelY - centerPixelY)

			// Only draw if visible in map area
			if screenX+TileSize > float64(offsetX) && screenX < float64(a.width) {
				vector.DrawFilledRect(screen, float32(screenX), float32(screenY), TileSize, TileSize, color.RGBA{50, 50, 55, 255}, true)
				vector.StrokeRect(screen, float32(screenX), float32(screenY), TileSize, TileSize, 1, color.RGBA{70, 70, 75, 255}, true)
			}
			continue
		}

		// Calculate screen position
		tilePixelX := float64(coord.X * TileSize)
		tilePixelY := float64(coord.Y * TileSize)
		screenX := screenCenterX + (tilePixelX - centerPixelX)
		screenY := screenCenterY + (tilePixelY - centerPixelY)

		// Only draw if visible in map area
		if screenX+TileSize > float64(offsetX) && screenX < float64(a.width) {
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Translate(screenX, screenY)
			screen.DrawImage(tile, op)
		}
	}
}

// drawFlightPathWithOffset draws flight path with X offset
func (a *App) drawFlightPathWithOffset(screen *ebiten.Image, offsetX int) {
	if len(a.flightPath) < 2 {
		return
	}

	mapWidth := a.width - offsetX
	centerPixelX, centerPixelY := LatLonToPixel(a.centerLat, a.centerLon, a.zoom)
	screenCenterX := float64(offsetX + mapWidth/2)
	screenCenterY := float64(a.height / 2)

	for i := 1; i < len(a.flightPath); i++ {
		p1 := a.flightPath[i-1]
		p2 := a.flightPath[i]

		x1, y1 := LatLonToPixel(p1.lat, p1.lon, a.zoom)
		x2, y2 := LatLonToPixel(p2.lat, p2.lon, a.zoom)

		sx1 := float32(screenCenterX + (x1 - centerPixelX))
		sy1 := float32(screenCenterY + (y1 - centerPixelY))
		sx2 := float32(screenCenterX + (x2 - centerPixelX))
		sy2 := float32(screenCenterY + (y2 - centerPixelY))

		// Color gradient (older = more transparent)
		alpha := uint8(100 + (155 * i / len(a.flightPath)))
		pathColor := color.RGBA{255, 200, 0, alpha}

		vector.StrokeLine(screen, sx1, sy1, sx2, sy2, 2, pathColor, true)
	}
}

// drawHomeMarkerWithOffset draws home marker with X offset
func (a *App) drawHomeMarkerWithOffset(screen *ebiten.Image, offsetX int) {
	if !a.homeSet {
		return
	}

	mapWidth := a.width - offsetX
	centerPixelX, centerPixelY := LatLonToPixel(a.centerLat, a.centerLon, a.zoom)
	screenCenterX := float64(offsetX + mapWidth/2)
	screenCenterY := float64(a.height / 2)

	hx, hy := LatLonToPixel(a.homeLat, a.homeLon, a.zoom)
	sx := float32(screenCenterX + (hx - centerPixelX))
	sy := float32(screenCenterY + (hy - centerPixelY))

	// Only draw if in map area
	if sx > float32(offsetX) && sx < float32(a.width) {
		// Home icon - house shape
		vector.DrawFilledCircle(screen, sx, sy, 8, color.RGBA{0, 255, 0, 200}, true)
		vector.StrokeCircle(screen, sx, sy, 8, 2, color.RGBA{255, 255, 255, 255}, true)
		ebitenutil.DebugPrintAt(screen, "H", int(sx)-3, int(sy)-6)
	}
}

// drawAircraftWithOffset draws aircraft with X offset
func (a *App) drawAircraftWithOffset(screen *ebiten.Image, offsetX int) {
	state := a.client.GetState()
	if !state.HasGPS || (state.Latitude == 0 && state.Longitude == 0) {
		return
	}

	mapWidth := a.width - offsetX
	centerPixelX, centerPixelY := LatLonToPixel(a.centerLat, a.centerLon, a.zoom)
	screenCenterX := float64(offsetX + mapWidth/2)
	screenCenterY := float64(a.height / 2)

	ax, ay := LatLonToPixel(float64(state.Latitude), float64(state.Longitude), a.zoom)
	sx := float32(screenCenterX + (ax - centerPixelX))
	sy := float32(screenCenterY + (ay - centerPixelY))

	// Only draw if in map area
	if sx > float32(offsetX) && sx < float32(a.width) {
		// Draw aircraft triangle pointing in heading direction
		a.drawAircraftTriangleAt(screen, sx, sy, state.Heading)
	}
}

// drawAircraftTriangleAt draws aircraft icon at specific position
func (a *App) drawAircraftTriangleAt(screen *ebiten.Image, sx, sy float32, heading float32) {
	headingRad := float64(heading) * math.Pi / 180
	size := float32(15)

	// Triangle points
	// Nose (front)
	noseX := sx + size*float32(math.Sin(headingRad))
	noseY := sy - size*float32(math.Cos(headingRad))

	// Left wing
	leftAngle := headingRad + 2.5
	leftX := sx + size*0.7*float32(math.Sin(leftAngle))
	leftY := sy - size*0.7*float32(math.Cos(leftAngle))

	// Right wing
	rightAngle := headingRad - 2.5
	rightX := sx + size*0.7*float32(math.Sin(rightAngle))
	rightY := sy - size*0.7*float32(math.Cos(rightAngle))

	// Tail
	tailX := sx - size*0.5*float32(math.Sin(headingRad))
	tailY := sy + size*0.5*float32(math.Cos(headingRad))

	// Draw filled aircraft shape
	vector.StrokeLine(screen, noseX, noseY, leftX, leftY, 3, color.RGBA{255, 100, 100, 255}, true)
	vector.StrokeLine(screen, noseX, noseY, rightX, rightY, 3, color.RGBA{255, 100, 100, 255}, true)
	vector.StrokeLine(screen, leftX, leftY, tailX, tailY, 3, color.RGBA{255, 100, 100, 255}, true)
	vector.StrokeLine(screen, rightX, rightY, tailX, tailY, 3, color.RGBA{255, 100, 100, 255}, true)

	// Center dot
	vector.DrawFilledCircle(screen, sx, sy, 3, color.RGBA{255, 255, 0, 255}, true)
}

// Layout returns the screen dimensions
func (a *App) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

func (a *App) handleKeyboard() {
	// Zoom
	if inpututil.IsKeyJustPressed(ebiten.KeyEqual) || inpututil.IsKeyJustPressed(ebiten.KeyKPAdd) {
		if a.zoom < MaxZoom {
			a.zoom++
			a.tileManager.ClearCache()
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyMinus) || inpututil.IsKeyJustPressed(ebiten.KeyKPSubtract) {
		if a.zoom > MinZoom {
			a.zoom--
			a.tileManager.ClearCache()
		}
	}

	// Pan with arrow keys
	panSpeed := 0.001 * math.Pow(2, float64(18-a.zoom))
	if ebiten.IsKeyPressed(ebiten.KeyUp) || ebiten.IsKeyPressed(ebiten.KeyW) {
		a.centerLat += panSpeed
		a.followAircraft = false
	}
	if ebiten.IsKeyPressed(ebiten.KeyDown) || ebiten.IsKeyPressed(ebiten.KeyS) {
		a.centerLat -= panSpeed
		a.followAircraft = false
	}
	if ebiten.IsKeyPressed(ebiten.KeyLeft) || ebiten.IsKeyPressed(ebiten.KeyA) {
		a.centerLon -= panSpeed
		a.followAircraft = false
	}
	if ebiten.IsKeyPressed(ebiten.KeyRight) || ebiten.IsKeyPressed(ebiten.KeyD) {
		a.centerLon += panSpeed
		a.followAircraft = false
	}

	// Toggle follow mode
	if inpututil.IsKeyJustPressed(ebiten.KeyF) {
		a.followAircraft = !a.followAircraft
	}

	// Set home position
	if inpututil.IsKeyJustPressed(ebiten.KeyH) {
		state := a.client.GetState()
		if state.HasGPS {
			a.homeLat = float64(state.Latitude)
			a.homeLon = float64(state.Longitude)
			a.homeSet = true
			log.Printf("Home set to %.6f, %.6f", a.homeLat, a.homeLon)
		}
	}

	// Clear flight path
	if inpututil.IsKeyJustPressed(ebiten.KeyC) {
		a.flightPath = nil
	}

	// Toggle help
	if inpututil.IsKeyJustPressed(ebiten.KeyF1) || inpututil.IsKeyJustPressed(ebiten.KeySlash) {
		a.showHelp = !a.showHelp
	}

	// Cycle HUD mode (0=off, 1=OSD, 2=cockpit)
	if inpututil.IsKeyJustPressed(ebiten.KeyV) {
		a.hudMode = (a.hudMode + 1) % 3
	}

	// Toggle map source (street/satellite)
	if inpututil.IsKeyJustPressed(ebiten.KeyM) {
		source := a.tileManager.ToggleSource()
		log.Printf("Map source: %s", a.tileManager.SourceName())
		_ = source
	}

	// Toggle touch buttons
	if inpututil.IsKeyJustPressed(ebiten.KeyT) {
		a.showTouchBtns = !a.showTouchBtns
	}

	// Connect/disconnect link
	if inpututil.IsKeyJustPressed(ebiten.KeyL) {
		if a.client.IsLinkStarted() {
			a.client.StopLink()
		} else if len(a.ports) > 0 && a.selectedPort < len(a.ports) {
			a.client.StartLink(a.ports[a.selectedPort], 420000)
		}
	}

	// Cycle through ports
	if inpututil.IsKeyJustPressed(ebiten.KeyP) {
		if len(a.ports) > 0 {
			a.selectedPort = (a.selectedPort + 1) % len(a.ports)
		}
	}

	// Fullscreen toggle
	if inpututil.IsKeyJustPressed(ebiten.KeyF11) {
		ebiten.SetFullscreen(!ebiten.IsFullscreen())
	}

	// Quit
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) || inpututil.IsKeyJustPressed(ebiten.KeyQ) {
		a.Shutdown()
	}
}

func (a *App) handleMouse() {
	// Scroll to zoom
	_, dy := ebiten.Wheel()
	if dy > 0 && a.zoom < MaxZoom {
		a.zoom++
	} else if dy < 0 && a.zoom > MinZoom {
		a.zoom--
	}

	// Drag to pan
	x, y := ebiten.CursorPosition()
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		a.dragging = true
		a.dragStartX = x
		a.dragStartY = y
		a.dragLat = a.centerLat
		a.dragLon = a.centerLon
	}

	if a.dragging {
		if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
			dx := float64(x - a.dragStartX)
			dy := float64(y - a.dragStartY)

			// Convert pixel delta to lat/lon delta
			scale := 360.0 / (float64(TileSize) * math.Pow(2, float64(a.zoom)))
			a.centerLon = a.dragLon - dx*scale
			a.centerLat = a.dragLat + dy*scale*math.Cos(a.centerLat*math.Pi/180)
			a.followAircraft = false
		} else {
			a.dragging = false
		}
	}
}

func (a *App) scanPorts() {
	if !a.client.IsConnected() {
		return
	}
	ports, err := a.client.GetTransmitters()
	if err != nil {
		return
	}
	a.ports = ports
}

func (a *App) drawMap(screen *ebiten.Image) {
	tiles := a.tileManager.GetTilesForView(a.centerLat, a.centerLon, a.zoom, a.width, a.height)

	// Calculate center pixel position
	centerPx, centerPy := LatLonToPixel(a.centerLat, a.centerLon, a.zoom)

	for _, coord := range tiles {
		tile := a.tileManager.GetTile(coord)
		if tile == nil {
			// Draw placeholder
			tileX := float64(coord.X*TileSize) - centerPx + float64(a.width)/2
			tileY := float64(coord.Y*TileSize) - centerPy + float64(a.height)/2
			vector.DrawFilledRect(screen, float32(tileX), float32(tileY), TileSize, TileSize, color.RGBA{50, 50, 50, 255}, false)
			continue
		}

		// Calculate tile position on screen
		tileX := float64(coord.X*TileSize) - centerPx + float64(a.width)/2
		tileY := float64(coord.Y*TileSize) - centerPy + float64(a.height)/2

		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(tileX, tileY)
		screen.DrawImage(tile, op)
	}
}

func (a *App) drawFlightPath(screen *ebiten.Image) {
	if len(a.flightPath) < 2 {
		return
	}

	centerPx, centerPy := LatLonToPixel(a.centerLat, a.centerLon, a.zoom)

	for i := 1; i < len(a.flightPath); i++ {
		p1 := a.flightPath[i-1]
		p2 := a.flightPath[i]

		x1, y1 := LatLonToPixel(p1.lat, p1.lon, a.zoom)
		x2, y2 := LatLonToPixel(p2.lat, p2.lon, a.zoom)

		sx1 := float32(x1 - centerPx + float64(a.width)/2)
		sy1 := float32(y1 - centerPy + float64(a.height)/2)
		sx2 := float32(x2 - centerPx + float64(a.width)/2)
		sy2 := float32(y2 - centerPy + float64(a.height)/2)

		// Gradient color based on age
		alpha := uint8(100 + 155*i/len(a.flightPath))
		vector.StrokeLine(screen, sx1, sy1, sx2, sy2, 2, color.RGBA{255, 200, 0, alpha}, false)
	}
}

func (a *App) drawHomeMarker(screen *ebiten.Image) {
	if !a.homeSet {
		return
	}

	centerPx, centerPy := LatLonToPixel(a.centerLat, a.centerLon, a.zoom)
	hx, hy := LatLonToPixel(a.homeLat, a.homeLon, a.zoom)

	sx := float32(hx - centerPx + float64(a.width)/2)
	sy := float32(hy - centerPy + float64(a.height)/2)

	// Draw home icon (house shape)
	vector.DrawFilledCircle(screen, sx, sy, 8, color.RGBA{0, 255, 0, 255}, false)
	vector.StrokeCircle(screen, sx, sy, 12, 2, color.RGBA{0, 200, 0, 255}, false)

	// Draw "H" label
	ebitenutil.DebugPrintAt(screen, "H", int(sx)-4, int(sy)-6)
}

func (a *App) drawAircraft(screen *ebiten.Image) {
	state := a.client.GetState()
	if !state.HasGPS || (state.Latitude == 0 && state.Longitude == 0) {
		return
	}

	centerPx, centerPy := LatLonToPixel(a.centerLat, a.centerLon, a.zoom)
	ax, ay := LatLonToPixel(float64(state.Latitude), float64(state.Longitude), a.zoom)

	sx := float32(ax - centerPx + float64(a.width)/2)
	sy := float32(ay - centerPy + float64(a.height)/2)

	// Draw aircraft triangle pointing in heading direction
	heading := float64(state.Heading) * math.Pi / 180

	// Triangle points
	size := float32(12)
	p1x := sx + size*float32(math.Sin(heading))
	p1y := sy - size*float32(math.Cos(heading))
	p2x := sx + size*0.5*float32(math.Sin(heading+2.5))
	p2y := sy - size*0.5*float32(math.Cos(heading+2.5))
	p3x := sx + size*0.5*float32(math.Sin(heading-2.5))
	p3y := sy - size*0.5*float32(math.Cos(heading-2.5))

	// Fill triangle
	path := vector.Path{}
	path.MoveTo(p1x, p1y)
	path.LineTo(p2x, p2y)
	path.LineTo(p3x, p3y)
	path.Close()

	vs, is := path.AppendVerticesAndIndicesForFilling(nil, nil)
	for i := range vs {
		vs[i].SrcX = 1
		vs[i].SrcY = 1
		vs[i].ColorR = 1
		vs[i].ColorG = 0
		vs[i].ColorB = 0
		vs[i].ColorA = 1
	}

	op := &ebiten.DrawTrianglesOptions{}
	screen.DrawTriangles(vs, is, emptyImage, op)

	// Outline
	vector.StrokeLine(screen, p1x, p1y, p2x, p2y, 2, color.RGBA{255, 100, 100, 255}, false)
	vector.StrokeLine(screen, p2x, p2y, p3x, p3y, 2, color.RGBA{255, 100, 100, 255}, false)
	vector.StrokeLine(screen, p3x, p3y, p1x, p1y, 2, color.RGBA{255, 100, 100, 255}, false)
}

var emptyImage = func() *ebiten.Image {
	img := ebiten.NewImage(3, 3)
	img.Fill(color.White)
	return img
}()

func (a *App) drawTelemetry(screen *ebiten.Image) {
	state := a.client.GetState()

	// Background panel
	panelW := 200
	panelH := 180
	panelX := a.width - panelW - 10
	panelY := 10

	vector.DrawFilledRect(screen, float32(panelX), float32(panelY), float32(panelW), float32(panelH), color.RGBA{0, 0, 0, 180}, false)

	// Telemetry text
	y := panelY + 5
	lineHeight := 16

	lines := []string{
		fmt.Sprintf("GPS: %.6f, %.6f", state.Latitude, state.Longitude),
		fmt.Sprintf("Alt: %dm  Sats: %d", state.Altitude, state.Satellites),
		fmt.Sprintf("Speed: %.1f km/h", state.GroundSpeed),
		fmt.Sprintf("Heading: %.0f°", state.Heading),
		fmt.Sprintf("Batt: %.1fV  %.1fA", state.Voltage, state.Current),
		fmt.Sprintf("RSSI: %d/%d dBm", state.RSSI1, state.RSSI2),
		fmt.Sprintf("LQ: %d%%  SNR: %d", state.LinkQuality, state.SNR),
		fmt.Sprintf("Pitch: %.1f° Roll: %.1f°", state.Pitch, state.Roll),
	}

	if a.homeSet {
		dist := a.calculateDistance(float64(state.Latitude), float64(state.Longitude), a.homeLat, a.homeLon)
		lines = append(lines, fmt.Sprintf("Home: %.0fm", dist))
	}

	for _, line := range lines {
		ebitenutil.DebugPrintAt(screen, line, panelX+5, y)
		y += lineHeight
	}
}

func (a *App) drawStatusBar(screen *ebiten.Image) {
	// Bottom status bar
	barH := 24
	barY := a.height - barH

	vector.DrawFilledRect(screen, 0, float32(barY), float32(a.width), float32(barH), color.RGBA{0, 0, 0, 200}, false)

	// Connection status
	connStatus := "Disconnected"
	connColor := color.RGBA{255, 100, 100, 255}
	if a.client.IsConnected() {
		connStatus = "Connected"
		connColor = color.RGBA{100, 255, 100, 255}
	}

	// Link status
	linkStatus := "Link: OFF"
	if a.client.IsLinkStarted() {
		linkStatus = "Link: ON"
	}

	// Port
	portStr := "No ports"
	if len(a.ports) > 0 && a.selectedPort < len(a.ports) {
		portStr = a.ports[a.selectedPort]
	}

	// Follow status
	followStr := "Manual"
	if a.followAircraft {
		followStr = "Follow"
	}

	// HUD status
	hudStr := "HUD:MAP"
	switch a.hudMode {
	case 1:
		hudStr = "HUD:OSD"
	case 2:
		hudStr = "HUD:PANEL"
	}

	// Map source
	mapStr := a.tileManager.SourceName()

	status := fmt.Sprintf(" %s | %s | Port: %s | Zoom: %d | %s | %s | %s | F1=Help", connStatus, linkStatus, portStr, a.zoom, followStr, mapStr, hudStr)
	_ = connColor // Would use for colored indicator

	ebitenutil.DebugPrintAt(screen, status, 5, barY+5)
}

func (a *App) drawHelp(screen *ebiten.Image) {
	help := []string{
		"=== ELRS Ground Station ===",
		"",
		"+/-     Zoom in/out",
		"Scroll  Zoom",
		"Drag    Pan map",
		"WASD    Pan map",
		"F       Toggle follow aircraft",
		"H       Set home position",
		"C       Clear flight path",
		"V       Cycle HUD (Map/OSD/Panel)",
		"M       Toggle map (street/sat)",
		"T       Toggle touch buttons",
		"L       Start/stop link",
		"P       Cycle ports",
		"F11     Toggle fullscreen",
		"F1/?    Toggle this help",
		"Q/Esc   Quit",
	}

	panelW := 250
	panelH := len(help)*16 + 20
	panelX := 10
	panelY := 10

	vector.DrawFilledRect(screen, float32(panelX), float32(panelY), float32(panelW), float32(panelH), color.RGBA{0, 0, 0, 200}, false)

	y := panelY + 10
	for _, line := range help {
		ebitenutil.DebugPrintAt(screen, line, panelX+10, y)
		y += 16
	}
}

func (a *App) calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	// Haversine formula
	R := 6371000.0 // Earth radius in meters

	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180

	a1 := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLon/2)*math.Sin(dLon/2)

	c := 2 * math.Atan2(math.Sqrt(a1), math.Sqrt(1-a1))
	return R * c
}

func (a *App) calculateBearing(lat1, lon1, lat2, lon2 float64) float64 {
	// Calculate bearing from point 1 to point 2
	lat1Rad := lat1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180

	x := math.Sin(dLon) * math.Cos(lat2Rad)
	y := math.Cos(lat1Rad)*math.Sin(lat2Rad) - math.Sin(lat1Rad)*math.Cos(lat2Rad)*math.Cos(dLon)

	bearing := math.Atan2(x, y) * 180 / math.Pi

	// Normalize to 0-360
	bearing = math.Mod(bearing+360, 360)
	return bearing
}
