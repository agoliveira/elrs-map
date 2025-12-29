package main

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// TouchButton represents an on-screen touch button
type TouchButton struct {
	X, Y, W, H int
	Label      string
	Icon       string // Optional icon character
	Active     bool   // Toggle state for toggle buttons
	Visible    bool
	OnPress    func()
}

// TouchControls manages touch UI elements
type TouchControls struct {
	buttons  []*TouchButton
	screenW  int
	screenH  int
	btnColor color.RGBA
	actColor color.RGBA
	txtColor color.RGBA
}

// NewTouchControls creates touch control manager
func NewTouchControls() *TouchControls {
	return &TouchControls{
		buttons:  make([]*TouchButton, 0),
		btnColor: color.RGBA{60, 60, 60, 200},
		actColor: color.RGBA{0, 150, 0, 200},
		txtColor: color.RGBA{255, 255, 255, 255},
	}
}

// AddButton adds a touch button
func (tc *TouchControls) AddButton(x, y, w, h int, label, icon string, onPress func()) *TouchButton {
	btn := &TouchButton{
		X:       x,
		Y:       y,
		W:       w,
		H:       h,
		Label:   label,
		Icon:    icon,
		Visible: true,
		OnPress: onPress,
	}
	tc.buttons = append(tc.buttons, btn)
	return btn
}

// Update checks for touch/click events
func (tc *TouchControls) Update() {
	// Handle mouse clicks
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		mx, my := ebiten.CursorPosition()
		tc.handlePress(mx, my)
	}

	// Handle touch
	touchIDs := inpututil.AppendJustPressedTouchIDs(nil)
	for _, id := range touchIDs {
		tx, ty := ebiten.TouchPosition(id)
		tc.handlePress(tx, ty)
	}
}

func (tc *TouchControls) handlePress(x, y int) {
	for _, btn := range tc.buttons {
		if !btn.Visible {
			continue
		}
		if x >= btn.X && x <= btn.X+btn.W && y >= btn.Y && y <= btn.Y+btn.H {
			if btn.OnPress != nil {
				btn.OnPress()
			}
			break
		}
	}
}

// Draw renders all touch buttons
func (tc *TouchControls) Draw(screen *ebiten.Image) {
	for _, btn := range tc.buttons {
		if !btn.Visible {
			continue
		}

		// Background
		bgColor := tc.btnColor
		if btn.Active {
			bgColor = tc.actColor
		}
		vector.DrawFilledRect(screen, float32(btn.X), float32(btn.Y), float32(btn.W), float32(btn.H), bgColor, true)

		// Border
		vector.StrokeRect(screen, float32(btn.X), float32(btn.Y), float32(btn.W), float32(btn.H), 2, tc.txtColor, true)

		// Label
		labelX := btn.X + btn.W/2 - len(btn.Label)*3
		labelY := btn.Y + btn.H/2 - 6
		if btn.Icon != "" {
			ebitenutil.DebugPrintAt(screen, btn.Icon, btn.X+btn.W/2-4, btn.Y+5)
			labelY = btn.Y + btn.H - 18
		}
		ebitenutil.DebugPrintAt(screen, btn.Label, labelX, labelY)
	}
}

// UpdateLayout repositions buttons based on screen size
func (tc *TouchControls) UpdateLayout(screenW, screenH int) {
	if tc.screenW == screenW && tc.screenH == screenH {
		return // No change
	}
	tc.screenW = screenW
	tc.screenH = screenH

	btnW := 60
	btnH := 45
	margin := 5
	bottomY := screenH - btnH - 30 // Above status bar

	// Position each button by label
	for _, btn := range tc.buttons {
		switch btn.Label {
		case "ZOOM+":
			btn.X, btn.Y = margin, bottomY-btnH-margin
		case "ZOOM-":
			btn.X, btn.Y = margin, bottomY
		case "FLLW":
			btn.X, btn.Y = margin+btnW+margin, bottomY
		case "HOME":
			btn.X, btn.Y = margin+btnW+margin, bottomY-btnH-margin
		case "CLR":
			btn.X, btn.Y = margin+(btnW+margin)*2, bottomY
		case "HUD":
			btn.X, btn.Y = margin+(btnW+margin)*2, bottomY-btnH-margin
		case "LINK":
			btn.X, btn.Y, btn.W = screenW/2-40, margin, 80
		case "PORT":
			btn.X, btn.Y = screenW/2-40-btnW-margin, margin
		}
	}
}

// SetupDefaultButtons creates the standard control buttons
func (tc *TouchControls) SetupDefaultButtons(app *App) {
	// These will be repositioned in UpdateLayout
	tc.AddButton(0, 0, 60, 45, "ZOOM+", "", func() {
		if app.zoom < MaxZoom {
			app.zoom++
		}
	})

	tc.AddButton(0, 0, 60, 45, "ZOOM-", "", func() {
		if app.zoom > MinZoom {
			app.zoom--
		}
	})

	tc.AddButton(0, 0, 60, 45, "FLLW", "", func() {
		app.followAircraft = !app.followAircraft
	})

	tc.AddButton(0, 0, 60, 45, "HOME", "", func() {
		state := app.client.GetState()
		if state.HasGPS {
			app.homeLat = float64(state.Latitude)
			app.homeLon = float64(state.Longitude)
			app.homeSet = true
		}
	})

	tc.AddButton(0, 0, 60, 45, "CLR", "", func() {
		app.flightPath = nil
	})

	tc.AddButton(0, 0, 60, 45, "HUD", "", func() {
		app.hudMode = (app.hudMode + 1) % 3
	})

	tc.AddButton(0, 0, 80, 45, "LINK", "", func() {
		if app.client.IsLinkStarted() {
			app.client.StopLink()
		} else if len(app.ports) > 0 && app.selectedPort < len(app.ports) {
			app.client.StartLink(app.ports[app.selectedPort], 420000)
		}
	})

	tc.AddButton(0, 0, 60, 45, "PORT", "", func() {
		if len(app.ports) > 0 {
			app.selectedPort = (app.selectedPort + 1) % len(app.ports)
		}
	})
}

// UpdateButtonStates updates active states based on app state
func (tc *TouchControls) UpdateButtonStates(app *App) {
	for _, btn := range tc.buttons {
		switch btn.Label {
		case "FLLW":
			btn.Active = app.followAircraft
		case "HUD":
			btn.Active = app.hudMode > 0
		case "LINK":
			btn.Active = app.client.IsLinkStarted()
		}
	}
}
