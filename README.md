# ELRS Ground Station Map

A lightweight native GUI application for displaying real-time telemetry from ExpressLRS aircraft on a map. Connects to the [elrs-joystick-control](https://github.com/kaack/elrs-joystick-control) backend via gRPC.

**Optimized for 1024x600 touchscreen displays** (7" LCD panels commonly used with Raspberry Pi).

## Features

- Real-time GPS position on OpenStreetMap
- Flight path history trail
- Home position marker with distance/bearing calculation
- **Cockpit HUD instruments:**
  - Artificial horizon (attitude indicator)
  - Compass rose (heading indicator)
  - Speed tape (ground speed)
  - Altitude tape (GPS altitude)
  - Vertical speed indicator
  - Battery gauge
  - RF link quality display
  - GPS status
- **Touch-friendly controls** for touchscreen operation
- Tile caching for offline use
- Follow aircraft mode
- Keyboard and mouse/touch controls

## Requirements

- Go 1.21+
- protoc (Protocol Buffers compiler)
- protoc-gen-go and protoc-gen-go-grpc plugins
- elrs-joystick-control backend running

## Build Instructions

### 1. Install dependencies

```bash
# Ubuntu/Debian
sudo apt install protobuf-compiler

# Install Go protoc plugins
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Add to PATH
export PATH=$PATH:$(go env GOPATH)/bin
```

### 2. Generate proto files

```bash
cd elrs-map
go generate ./...
```

### 3. Build

```bash
go build -o elrs-map .
```

### 4. Cross-compile for Raspberry Pi

```bash
# For Pi 3/4 (64-bit)
GOOS=linux GOARCH=arm64 go build -o elrs-map-arm64 .

# For Pi 3/4 (32-bit)
GOOS=linux GOARCH=arm GOARM=7 go build -o elrs-map-arm .
```

## Usage

### Start the backend first

```bash
# On the machine with ELRS TX connected
./elrs-joystick-control
# gRPC server started on [::]:10000
```

### Run the ground station

```bash
# Same machine
./elrs-map

# Different machine (Pi display station)
./elrs-map -grpc 192.168.1.100:10000

# With touch buttons enabled
./elrs-map -touch

# Fullscreen for dedicated display
./elrs-map -fullscreen
```

### Command line options

```
-grpc string     gRPC server address (default "localhost:10000")
-cache string    Tile cache directory (default "tiles")
-fullscreen      Start in fullscreen mode
-width int       Window width (default 1024)
-height int      Window height (default 600)
-touch           Enable on-screen touch buttons
```

## GPIO Button Wiring (Raspberry Pi)

For a dedicated ground station, wire physical buttons to GPIO pins:

```
Button      BCM Pin    Physical Pin    Wire to
──────────────────────────────────────────────
HOME        GPIO 17    Pin 11          GND when pressed
LINK        GPIO 27    Pin 13          GND when pressed
ZOOM IN     GPIO 22    Pin 15          GND when pressed
ZOOM OUT    GPIO 23    Pin 16          GND when pressed
FOLLOW      GPIO 24    Pin 18          GND when pressed
CLEAR       GPIO 25    Pin 22          GND when pressed
──────────────────────────────────────────────
GND         -          Pin 6, 9, 14, 20, 25, etc.
```

### Wiring

```
GPIO Pin ──────┤ ├────── GND
           (button)
```

No resistors needed - the Pi's internal pull-ups are enabled.
Connect each button between its GPIO pin and any GND pin.

## Controls

### Keyboard (always available)
| Key | Action |
|-----|--------|
| `+/-` or scroll | Zoom in/out |
| Drag or WASD | Pan map |
| `F` | Toggle follow aircraft mode |
| `H` | Set home position at aircraft |
| `C` | Clear flight path |
| `V` | Toggle cockpit HUD |
| `T` | Toggle touch buttons |
| `L` | Start/stop ELRS link |
| `P` | Cycle through serial ports |
| `F11` | Toggle fullscreen |
| `F1` or `?` | Toggle help overlay |
| `Q` or `Esc` | Quit |

### GPIO Buttons (Raspberry Pi)
| Button | Action |
|--------|--------|
| HOME | Set home position at current GPS |
| LINK | Start/stop ELRS link |
| ZOOM+ | Zoom in |
| ZOOM- | Zoom out |
| FOLLOW | Toggle follow aircraft |
| CLEAR | Clear flight path |

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    elrs-map (this app)                      │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌──────────────┐    gRPC     ┌─────────────────────────┐  │
│  │ GRPCClient   │◄───────────►│ elrs-joystick-control   │  │
│  │              │   :10000    │ (Go backend)            │  │
│  └──────┬───────┘             └─────────────────────────┘  │
│         │                                                   │
│         ▼                                                   │
│  ┌──────────────┐    HTTP     ┌─────────────────────────┐  │
│  │ TileManager  │◄───────────►│ OpenStreetMap           │  │
│  │ (cache)      │             │ tile.openstreetmap.org  │  │
│  └──────┬───────┘             └─────────────────────────┘  │
│         │                                                   │
│         ▼                                                   │
│  ┌──────────────┐                                          │
│  │ Ebiten       │──────────► Native window/display         │
│  │ (rendering)  │                                          │
│  └──────────────┘                                          │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

## Telemetry Display

The application displays:

- **GPS**: Latitude, longitude, altitude, satellite count
- **Speed**: Ground speed in km/h
- **Heading**: Current heading in degrees
- **Battery**: Voltage, current
- **Link**: RSSI (both antennas), link quality %, SNR
- **Attitude**: Pitch, roll angles
- **Distance**: Distance to home (when home is set)

## Tile Caching

Map tiles are cached in the `tiles/` directory. For offline use, fly once with internet connectivity to cache your area, then the app will work without network access.

To pre-download tiles for an area, you can use tools like [JTileDownloader](https://wiki.openstreetmap.org/wiki/JTileDownloader) or the mobile OSM apps.

## License

GPL 3.0
