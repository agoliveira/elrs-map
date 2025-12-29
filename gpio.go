package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

// GPIO Button assignments (BCM numbering)
// These are common pins that don't conflict with other interfaces
const (
	GPIO_BTN_HOME    = 17 // Pin 11
	GPIO_BTN_LINK    = 27 // Pin 13
	GPIO_BTN_ZOOMIN  = 22 // Pin 15
	GPIO_BTN_ZOOMOUT = 23 // Pin 16
	GPIO_BTN_FOLLOW  = 24 // Pin 18
	GPIO_BTN_CLEAR   = 25 // Pin 22
	GPIO_BTN_MAP     = 5  // Pin 29 - Toggle map source
)

// GPIOButton represents a single GPIO button
type GPIOButton struct {
	pin        int
	name       string
	lastState  bool
	debounceMs int64
	lastChange int64
	onPress    func()
}

// GPIOController manages GPIO button inputs
type GPIOController struct {
	buttons  []*GPIOButton
	enabled  bool
	mu       sync.Mutex
	stopChan chan struct{}
}

// NewGPIOController creates a new GPIO controller
func NewGPIOController() *GPIOController {
	return &GPIOController{
		buttons:  make([]*GPIOButton, 0),
		stopChan: make(chan struct{}),
	}
}

// AddButton adds a GPIO button
func (g *GPIOController) AddButton(pin int, name string, onPress func()) {
	g.buttons = append(g.buttons, &GPIOButton{
		pin:        pin,
		name:       name,
		debounceMs: 50,
		onPress:    onPress,
	})
}

// SetupDefaultButtons configures standard button mappings
func (g *GPIOController) SetupDefaultButtons(app *App) {
	g.AddButton(GPIO_BTN_HOME, "HOME", func() {
		state := app.client.GetState()
		if state.HasGPS && (state.Latitude != 0 || state.Longitude != 0) {
			app.homeLat = float64(state.Latitude)
			app.homeLon = float64(state.Longitude)
			app.homeSet = true
			log.Printf("Home set: %.6f, %.6f", app.homeLat, app.homeLon)
		}
	})

	g.AddButton(GPIO_BTN_LINK, "LINK", func() {
		if app.client.IsLinkStarted() {
			app.client.StopLink()
		} else if len(app.ports) > 0 && app.selectedPort < len(app.ports) {
			app.client.StartLink(app.ports[app.selectedPort], 420000)
		}
	})

	g.AddButton(GPIO_BTN_ZOOMIN, "ZOOM+", func() {
		if app.zoom < MaxZoom {
			app.zoom++
		}
	})

	g.AddButton(GPIO_BTN_ZOOMOUT, "ZOOM-", func() {
		if app.zoom > MinZoom {
			app.zoom--
		}
	})

	g.AddButton(GPIO_BTN_FOLLOW, "FOLLOW", func() {
		app.followAircraft = !app.followAircraft
		log.Printf("Follow mode: %v", app.followAircraft)
	})

	g.AddButton(GPIO_BTN_CLEAR, "CLEAR", func() {
		app.flightPath = nil
		log.Println("Flight path cleared")
	})

	g.AddButton(GPIO_BTN_MAP, "MAP", func() {
		source := app.tileManager.ToggleSource()
		log.Printf("Map source: %s", app.tileManager.SourceName())
		_ = source
	})
}

// Start begins polling GPIO pins
func (g *GPIOController) Start() error {
	// Check if we're on a Raspberry Pi by checking for GPIO sysfs
	if _, err := os.Stat("/sys/class/gpio"); os.IsNotExist(err) {
		log.Println("GPIO not available (not running on Pi?) - GPIO buttons disabled")
		return nil
	}

	// Export and configure pins
	for _, btn := range g.buttons {
		if err := g.exportPin(btn.pin); err != nil {
			log.Printf("Warning: Could not export GPIO %d: %v", btn.pin, err)
			continue
		}
		if err := g.setDirection(btn.pin, "in"); err != nil {
			log.Printf("Warning: Could not set GPIO %d direction: %v", btn.pin, err)
			continue
		}
		// Enable pull-up (buttons connect to ground)
		// Note: This requires /sys/class/gpio/gpioX/active_low or device tree config
		// For simplicity, we assume active-low buttons (pressed = 0)
	}

	g.enabled = true
	go g.pollLoop()
	log.Println("GPIO controller started")
	return nil
}

// Stop stops the GPIO polling
func (g *GPIOController) Stop() {
	if g.enabled {
		close(g.stopChan)
		g.enabled = false

		// Unexport pins
		for _, btn := range g.buttons {
			g.unexportPin(btn.pin)
		}
	}
}

func (g *GPIOController) pollLoop() {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-g.stopChan:
			return
		case <-ticker.C:
			g.pollButtons()
		}
	}
}

func (g *GPIOController) pollButtons() {
	now := time.Now().UnixMilli()

	for _, btn := range g.buttons {
		value, err := g.readPin(btn.pin)
		if err != nil {
			continue
		}

		// Active low: pressed when value is 0
		pressed := (value == 0)

		// Debounce
		if pressed != btn.lastState {
			if now-btn.lastChange > btn.debounceMs {
				btn.lastState = pressed
				btn.lastChange = now

				// Trigger on press (not release)
				if pressed && btn.onPress != nil {
					btn.onPress()
				}
			}
		}
	}
}

// GPIO sysfs helpers

func (g *GPIOController) exportPin(pin int) error {
	// Check if already exported
	pinPath := fmt.Sprintf("/sys/class/gpio/gpio%d", pin)
	if _, err := os.Stat(pinPath); err == nil {
		return nil // Already exported
	}

	f, err := os.OpenFile("/sys/class/gpio/export", os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(fmt.Sprintf("%d", pin))
	if err != nil {
		return err
	}

	// Wait for sysfs to create the pin directory
	time.Sleep(100 * time.Millisecond)
	return nil
}

func (g *GPIOController) unexportPin(pin int) error {
	f, err := os.OpenFile("/sys/class/gpio/unexport", os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(fmt.Sprintf("%d", pin))
	return err
}

func (g *GPIOController) setDirection(pin int, direction string) error {
	path := fmt.Sprintf("/sys/class/gpio/gpio%d/direction", pin)
	return os.WriteFile(path, []byte(direction), 0644)
}

func (g *GPIOController) readPin(pin int) (int, error) {
	path := fmt.Sprintf("/sys/class/gpio/gpio%d/value", pin)
	f, err := os.Open(path)
	if err != nil {
		return -1, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if scanner.Scan() {
		text := scanner.Text()
		if text == "0" {
			return 0, nil
		}
		return 1, nil
	}
	return -1, fmt.Errorf("could not read pin value")
}

// IsAvailable returns true if GPIO is available on this system
func (g *GPIOController) IsAvailable() bool {
	_, err := os.Stat("/sys/class/gpio")
	return err == nil
}
