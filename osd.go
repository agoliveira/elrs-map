package main

import (
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// OSD renders INAV-style on-screen display
type OSD struct {
	screenW, screenH int

	// Colors
	textColor    color.RGBA
	warningColor color.RGBA
	bgColor      color.RGBA
}

// NewOSD creates a new OSD overlay
func NewOSD() *OSD {
	return &OSD{
		textColor:    color.RGBA{255, 255, 255, 255},
		warningColor: color.RGBA{255, 80, 80, 255},
		bgColor:      color.RGBA{0, 0, 0, 160},
	}
}

// Draw renders the OSD overlay
func (o *OSD) Draw(screen *ebiten.Image, state TelemetryState, homeSet bool, homeDist, homeBearing float64) {
	o.screenW, o.screenH = screen.Bounds().Dx(), screen.Bounds().Dy()

	// === TOP LEFT: Coordinates ===
	o.drawTextBox(screen, fmt.Sprintf("%.5f", state.Latitude), 5, 5)
	o.drawTextBox(screen, fmt.Sprintf("%.5f", state.Longitude), 5, 22)

	// === TOP CENTER: Heading ===
	o.drawHeadingBar(screen, o.screenW/2, 5, state.Heading)

	// === TOP RIGHT: GPS sats ===
	satStr := fmt.Sprintf("%d sats", state.Satellites)
	satW := len(satStr)*7 + 8
	if state.Satellites < 4 {
		o.drawTextBoxColored(screen, satStr, o.screenW-satW-5, 5, o.warningColor)
	} else {
		o.drawTextBox(screen, satStr, o.screenW-satW-5, 5)
	}

	// === LEFT SIDE: Speed ===
	spdStr := fmt.Sprintf("%.0f", state.GroundSpeed)
	o.drawTextBox(screen, spdStr, 5, o.screenH/2-20)
	o.drawTextBox(screen, "km/h", 5, o.screenH/2-3)

	// === RIGHT SIDE: Altitude ===
	altStr := fmt.Sprintf("%dm", state.Altitude)
	altW := len(altStr)*7 + 8
	o.drawTextBox(screen, altStr, o.screenW-altW-5, o.screenH/2-20)

	// Home arrow and distance
	if homeSet && state.HasGPS {
		o.drawHomeArrow(screen, o.screenW-35, o.screenH/2+15, state.Heading, homeBearing)
		distStr := ""
		if homeDist >= 1000 {
			distStr = fmt.Sprintf("%.1fkm", homeDist/1000)
		} else {
			distStr = fmt.Sprintf("%.0fm", homeDist)
		}
		distW := len(distStr)*7 + 8
		if homeDist > 5000 {
			o.drawTextBoxColored(screen, distStr, o.screenW-distW-5, o.screenH/2+40, o.warningColor)
		} else {
			o.drawTextBox(screen, distStr, o.screenW-distW-5, o.screenH/2+40)
		}
	}

	// === BOTTOM LEFT: Battery ===
	battStr := fmt.Sprintf("%.1fV %d%%", state.Voltage, state.Remaining)
	if state.Remaining < 20 {
		o.drawTextBoxColored(screen, battStr, 5, o.screenH-55, o.warningColor)
	} else {
		o.drawTextBox(screen, battStr, 5, o.screenH-55)
	}
	o.drawTextBox(screen, fmt.Sprintf("%.1fA", state.Current), 5, o.screenH-38)

	// === BOTTOM CENTER: Link Quality ===
	lqStr := fmt.Sprintf("LQ:%d%% RSSI:%d", state.LinkQuality, state.RSSI1)
	lqW := len(lqStr)*7 + 8
	if state.LinkQuality < 50 {
		o.drawTextBoxColored(screen, lqStr, o.screenW/2-lqW/2, o.screenH-38, o.warningColor)
	} else {
		o.drawTextBox(screen, lqStr, o.screenW/2-lqW/2, o.screenH-38)
	}

	// === BOTTOM RIGHT: Attitude ===
	attStr := fmt.Sprintf("P:%+.0f R:%+.0f", state.Pitch, state.Roll)
	attW := len(attStr)*7 + 8
	o.drawTextBox(screen, attStr, o.screenW-attW-5, o.screenH-38)
}

// drawTextBox draws text with semi-transparent background
func (o *OSD) drawTextBox(screen *ebiten.Image, text string, x, y int) {
	w := len(text)*7 + 6
	h := 16
	vector.DrawFilledRect(screen, float32(x-2), float32(y-1), float32(w), float32(h), o.bgColor, true)
	ebitenutil.DebugPrintAt(screen, text, x, y)
}

// drawTextBoxColored draws text with colored background for warnings
func (o *OSD) drawTextBoxColored(screen *ebiten.Image, text string, x, y int, bgColor color.RGBA) {
	w := len(text)*7 + 6
	h := 16
	vector.DrawFilledRect(screen, float32(x-2), float32(y-1), float32(w), float32(h), bgColor, true)
	ebitenutil.DebugPrintAt(screen, text, x, y)
}

// drawHeadingBar draws a compact heading indicator at top center
func (o *OSD) drawHeadingBar(screen *ebiten.Image, cx, y int, heading float32) {
	barW := 180
	barH := 20

	// Background
	vector.DrawFilledRect(screen, float32(cx-barW/2), float32(y), float32(barW), float32(barH), o.bgColor, true)

	// Cardinals
	cardinals := []struct {
		dir string
		hdg float32
	}{
		{"N", 0}, {"NE", 45}, {"E", 90}, {"SE", 135},
		{"S", 180}, {"SW", 225}, {"W", 270}, {"NW", 315},
	}

	for _, c := range cardinals {
		diff := c.hdg - heading
		for diff > 180 {
			diff -= 360
		}
		for diff < -180 {
			diff += 360
		}

		if diff > -50 && diff < 50 {
			px := cx + int(diff*float32(barW)/100)
			ebitenutil.DebugPrintAt(screen, c.dir, px-len(c.dir)*3, y+3)
		}
	}

	// Center marker
	vector.DrawFilledRect(screen, float32(cx-1), float32(y+barH-5), 3, 5, o.textColor, true)

	// Heading value below
	hdgStr := fmt.Sprintf("%03.0f°", heading)
	o.drawTextBox(screen, hdgStr, cx-20, y+barH+3)
}

// drawHomeArrow draws an arrow pointing to home
func (o *OSD) drawHomeArrow(screen *ebiten.Image, cx, cy int, heading float32, homeBearing float64) {
	// Background circle
	vector.DrawFilledCircle(screen, float32(cx), float32(cy), 18, o.bgColor, true)

	// Calculate relative bearing
	relBearing := homeBearing - float64(heading)
	relBearingRad := relBearing * math.Pi / 180

	// Arrow size
	size := float32(14)

	// Arrow tip
	tipX := float32(cx) + size*float32(math.Sin(relBearingRad))
	tipY := float32(cy) - size*float32(math.Cos(relBearingRad))

	// Arrow base points
	baseAngle1 := relBearingRad + 2.5
	baseAngle2 := relBearingRad - 2.5
	base1X := float32(cx) + size*0.4*float32(math.Sin(baseAngle1))
	base1Y := float32(cy) - size*0.4*float32(math.Cos(baseAngle1))
	base2X := float32(cx) + size*0.4*float32(math.Sin(baseAngle2))
	base2Y := float32(cy) - size*0.4*float32(math.Cos(baseAngle2))

	// Draw arrow
	vector.StrokeLine(screen, float32(cx), float32(cy), tipX, tipY, 2, o.textColor, true)
	vector.StrokeLine(screen, tipX, tipY, base1X, base1Y, 2, o.textColor, true)
	vector.StrokeLine(screen, tipX, tipY, base2X, base2Y, 2, o.textColor, true)

	// Home icon (H)
	ebitenutil.DebugPrintAt(screen, "H", cx-4, cy-5)
}
