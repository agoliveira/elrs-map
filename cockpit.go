package main

import (
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// CockpitHUD renders flight instruments
type CockpitHUD struct {
	// Layout
	screenW, screenH int

	// Colors
	skyColor     color.RGBA
	groundColor  color.RGBA
	lineColor    color.RGBA
	textColor    color.RGBA
	warningColor color.RGBA
	accentColor  color.RGBA
	bgColor      color.RGBA
}

// NewCockpitHUD creates a new cockpit HUD
func NewCockpitHUD() *CockpitHUD {
	return &CockpitHUD{
		skyColor:     color.RGBA{70, 130, 180, 255},  // Steel blue
		groundColor:  color.RGBA{139, 90, 43, 255},   // Brown
		lineColor:    color.RGBA{255, 255, 255, 255}, // White
		textColor:    color.RGBA{0, 255, 0, 255},     // Green (HUD style)
		warningColor: color.RGBA{255, 50, 50, 255},   // Red
		accentColor:  color.RGBA{255, 200, 0, 255},   // Yellow/Gold
		bgColor:      color.RGBA{0, 0, 0, 180},       // Transparent black
	}
}

// Draw renders all cockpit instruments
func (h *CockpitHUD) Draw(screen *ebiten.Image, state TelemetryState, homeSet bool, homeDist, homeBearing float64) {
	h.screenW, h.screenH = screen.Bounds().Dx(), screen.Bounds().Dy()

	// Layout: instruments on edges, center clear for map
	// Top bar: telemetry text
	// Left edge: speed tape
	// Right edge: altitude + VSI (aligned to border)
	// Bottom corners: horizon (left) and compass (right)

	// === TOP BAR (compact status with backgrounds) ===
	h.drawTopBar(screen, state, homeSet, homeDist, homeBearing)

	// === LEFT EDGE: Speed tape (flush with border) ===
	tapeW := 50
	tapeH := 180
	tapeY := (h.screenH - tapeH) / 2
	h.drawSpeedTape(screen, 0, tapeY+tapeH/2, tapeW, tapeH, state.GroundSpeed)

	// === RIGHT EDGE: Altitude + VSI (flush with border) ===
	// VSI on far right
	vsiW := 25
	h.drawVSI(screen, h.screenW-vsiW, tapeY+tapeH/2, vsiW, tapeH, state.VerticalSpeed)
	// Altitude tape next to VSI
	h.drawAltitudeTape(screen, h.screenW-vsiW-tapeW-5, tapeY+tapeH/2, tapeW, tapeH, float32(state.Altitude))

	// === BOTTOM LEFT: Artificial Horizon ===
	ahSize := 130
	ahX := ahSize/2 + 10
	ahY := h.screenH - ahSize/2 - 40
	h.drawArtificialHorizon(screen, ahX, ahY, ahSize, state.Pitch, state.Roll)

	// === BOTTOM RIGHT: Compass ===
	compassR := 55
	compassX := h.screenW - compassR - 10
	compassY := h.screenH - compassR - 40
	h.drawCompass(screen, compassX, compassY, compassR, state.Heading)
}

// drawTopBar renders compact status bar at top with readable text
func (h *CockpitHUD) drawTopBar(screen *ebiten.Image, state TelemetryState, homeSet bool, homeDist, homeBearing float64) {
	// Full width semi-transparent background
	barH := 24
	vector.DrawFilledRect(screen, 0, 0, float32(h.screenW), float32(barH), color.RGBA{0, 0, 0, 180}, true)

	y := 5

	// Battery
	battStr := fmt.Sprintf("BAT: %.1fV %.1fA %d%%", state.Voltage, state.Current, state.Remaining)
	if state.Remaining < 20 {
		h.drawTextWithBg(screen, battStr, 10, y, h.warningColor)
	} else {
		ebitenutil.DebugPrintAt(screen, battStr, 10, y)
	}

	// RF Link
	rfStr := fmt.Sprintf("RF: LQ:%d%% RSSI:%d/%d SNR:%d", state.LinkQuality, state.RSSI1, state.RSSI2, state.SNR)
	if state.LinkQuality < 50 {
		h.drawTextWithBg(screen, rfStr, 200, y, h.warningColor)
	} else {
		ebitenutil.DebugPrintAt(screen, rfStr, 200, y)
	}

	// GPS
	gpsStr := fmt.Sprintf("GPS: %d sats", state.Satellites)
	if state.Satellites < 4 {
		h.drawTextWithBg(screen, gpsStr, 450, y, h.warningColor)
	} else {
		ebitenutil.DebugPrintAt(screen, gpsStr, 450, y)
	}

	// Home distance
	if homeSet && state.HasGPS {
		homeStr := ""
		if homeDist >= 1000 {
			homeStr = fmt.Sprintf("HOME: %.1fkm %03.0f°", homeDist/1000, homeBearing)
		} else {
			homeStr = fmt.Sprintf("HOME: %.0fm %03.0f°", homeDist, homeBearing)
		}
		if homeDist > 5000 {
			h.drawTextWithBg(screen, homeStr, 580, y, h.warningColor)
		} else {
			ebitenutil.DebugPrintAt(screen, homeStr, 580, y)
		}
	} else {
		ebitenutil.DebugPrintAt(screen, "HOME: ---", 580, y)
	}

	// Attitude (pitch/roll)
	attStr := fmt.Sprintf("P:%+.0f° R:%+.0f°", state.Pitch, state.Roll)
	ebitenutil.DebugPrintAt(screen, attStr, h.screenW-100, y)
}

// drawTextWithBg draws text with a colored background for warnings
func (h *CockpitHUD) drawTextWithBg(screen *ebiten.Image, text string, x, y int, bgColor color.RGBA) {
	w := len(text)*7 + 4
	vector.DrawFilledRect(screen, float32(x-2), float32(y-1), float32(w), 16, bgColor, true)
	ebitenutil.DebugPrintAt(screen, text, x, y)
}

// drawHomeInfo shows distance and bearing to home
func (h *CockpitHUD) drawHomeInfo(screen *ebiten.Image, x, y, width, height int, state TelemetryState, homeSet bool, homeDist, homeBearing float64) {
	// Background
	vector.DrawFilledRect(screen, float32(x), float32(y), float32(width), float32(height), h.bgColor, true)

	ebitenutil.DebugPrintAt(screen, "HOME", x+10, y+5)

	if !homeSet {
		ebitenutil.DebugPrintAt(screen, "NOT SET (H)", x+10, y+22)
	} else if !state.HasGPS {
		ebitenutil.DebugPrintAt(screen, "NO GPS", x+10, y+22)
	} else {
		// Format distance
		distStr := ""
		if homeDist >= 1000 {
			distStr = fmt.Sprintf("%.1f km", homeDist/1000)
		} else {
			distStr = fmt.Sprintf("%.0f m", homeDist)
		}
		ebitenutil.DebugPrintAt(screen, distStr, x+10, y+20)
		ebitenutil.DebugPrintAt(screen, fmt.Sprintf("BRG: %03.0f°", homeBearing), x+80, y+20)

		// Warning if far
		if homeDist > 5000 {
			vector.StrokeRect(screen, float32(x), float32(y), float32(width), float32(height), 2, h.warningColor, true)
			return
		}
	}

	// Border
	vector.StrokeRect(screen, float32(x), float32(y), float32(width), float32(height), 1, h.lineColor, true)
}

// drawArtificialHorizon renders the attitude indicator
func (h *CockpitHUD) drawArtificialHorizon(screen *ebiten.Image, cx, cy, size int, pitch, roll float32) {
	halfSize := float32(size / 2)

	// Clip region (circular mask effect via drawing order)
	// Background circle
	vector.DrawFilledCircle(screen, float32(cx), float32(cy), halfSize+2, color.RGBA{40, 40, 40, 255}, true)

	// Create a sub-image for clipping effect
	// We'll draw the horizon then mask it

	// Calculate horizon offset based on pitch (pixels per degree)
	pitchScale := float32(size) / 40.0 // 40 degrees visible range
	horizonOffset := pitch * pitchScale

	// Roll rotation
	rollRad := float64(-roll) * math.Pi / 180

	// Draw sky and ground split by horizon line
	// This is simplified - proper implementation would clip to circle

	// Sky half
	skyPts := []float32{
		float32(cx) - halfSize, float32(cy) - halfSize,
		float32(cx) + halfSize, float32(cy) - halfSize,
		float32(cx) + halfSize, float32(cy) + horizonOffset,
		float32(cx) - halfSize, float32(cy) + horizonOffset,
	}
	h.drawRotatedQuad(screen, cx, cy, skyPts, rollRad, h.skyColor)

	// Ground half
	groundPts := []float32{
		float32(cx) - halfSize, float32(cy) + horizonOffset,
		float32(cx) + halfSize, float32(cy) + horizonOffset,
		float32(cx) + halfSize, float32(cy) + halfSize,
		float32(cx) - halfSize, float32(cy) + halfSize,
	}
	h.drawRotatedQuad(screen, cx, cy, groundPts, rollRad, h.groundColor)

	// Horizon line
	x1, y1 := h.rotatePoint(float32(cx)-halfSize, float32(cy)+horizonOffset, float32(cx), float32(cy), rollRad)
	x2, y2 := h.rotatePoint(float32(cx)+halfSize, float32(cy)+horizonOffset, float32(cx), float32(cy), rollRad)
	vector.StrokeLine(screen, x1, y1, x2, y2, 2, h.lineColor, true)

	// Pitch ladder (every 10 degrees)
	for deg := -30; deg <= 30; deg += 10 {
		if deg == 0 {
			continue
		}
		offset := horizonOffset - float32(deg)*pitchScale
		lineLen := float32(40)
		if deg%20 != 0 {
			lineLen = 20
		}

		lx1, ly1 := h.rotatePoint(float32(cx)-lineLen/2, float32(cy)+offset, float32(cx), float32(cy), rollRad)
		lx2, ly2 := h.rotatePoint(float32(cx)+lineLen/2, float32(cy)+offset, float32(cx), float32(cy), rollRad)

		// Only draw if within bounds
		if ly1 > float32(cy)-halfSize && ly1 < float32(cy)+halfSize {
			vector.StrokeLine(screen, lx1, ly1, lx2, ly2, 1, h.lineColor, true)
			// Degree label
			ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%d", -deg), int(lx2)+5, int(ly2)-6)
		}
	}

	// Aircraft reference symbol (fixed in center)
	// Wings
	vector.StrokeLine(screen, float32(cx)-40, float32(cy), float32(cx)-15, float32(cy), 3, h.accentColor, true)
	vector.StrokeLine(screen, float32(cx)+15, float32(cy), float32(cx)+40, float32(cy), 3, h.accentColor, true)
	// Center dot
	vector.DrawFilledCircle(screen, float32(cx), float32(cy), 4, h.accentColor, true)
	// Tail
	vector.StrokeLine(screen, float32(cx), float32(cy)+5, float32(cx), float32(cy)+15, 3, h.accentColor, true)

	// Roll indicator arc (top)
	h.drawRollIndicator(screen, cx, cy, int(halfSize), roll)

	// Border circle
	vector.StrokeCircle(screen, float32(cx), float32(cy), halfSize, 2, h.lineColor, true)

	// Pitch readout
	pitchStr := fmt.Sprintf("P %+.1f°", pitch)
	ebitenutil.DebugPrintAt(screen, pitchStr, cx-30, cy+int(halfSize)+5)

	// Roll readout
	rollStr := fmt.Sprintf("R %+.1f°", roll)
	ebitenutil.DebugPrintAt(screen, rollStr, cx-30, cy+int(halfSize)+20)
}

// drawRollIndicator draws the roll scale arc at top of attitude indicator
func (h *CockpitHUD) drawRollIndicator(screen *ebiten.Image, cx, cy, radius int, roll float32) {
	// Draw arc markers at standard angles
	angles := []int{-60, -45, -30, -20, -10, 0, 10, 20, 30, 45, 60}

	for _, ang := range angles {
		rad := float64(ang-90) * math.Pi / 180
		innerR := float32(radius) - 10
		outerR := float32(radius) - 2

		// Longer marks at major angles
		if ang%30 == 0 {
			innerR = float32(radius) - 15
		}

		x1 := float32(cx) + innerR*float32(math.Cos(rad))
		y1 := float32(cy) + innerR*float32(math.Sin(rad))
		x2 := float32(cx) + outerR*float32(math.Cos(rad))
		y2 := float32(cy) + outerR*float32(math.Sin(rad))

		vector.StrokeLine(screen, x1, y1, x2, y2, 1, h.lineColor, true)
	}

	// Roll pointer (triangle)
	rollRad := float64(-roll-90) * math.Pi / 180
	pointerR := float32(radius) - 18
	px := float32(cx) + pointerR*float32(math.Cos(rollRad))
	py := float32(cy) + pointerR*float32(math.Sin(rollRad))

	// Small triangle pointing inward
	vector.DrawFilledCircle(screen, px, py, 5, h.accentColor, true)
}

// drawCompass renders the heading indicator
func (h *CockpitHUD) drawCompass(screen *ebiten.Image, cx, cy, radius int, heading float32) {
	// Background
	vector.DrawFilledCircle(screen, float32(cx), float32(cy), float32(radius)+2, h.bgColor, true)

	// Compass rose
	for deg := 0; deg < 360; deg += 10 {
		rad := float64(deg-int(heading)-90) * math.Pi / 180
		innerR := float32(radius) - 15
		outerR := float32(radius) - 2

		// Longer marks at cardinal/intercardinal
		if deg%30 == 0 {
			innerR = float32(radius) - 20
		}

		x1 := float32(cx) + innerR*float32(math.Cos(rad))
		y1 := float32(cy) + innerR*float32(math.Sin(rad))
		x2 := float32(cx) + outerR*float32(math.Cos(rad))
		y2 := float32(cy) + outerR*float32(math.Sin(rad))

		vector.StrokeLine(screen, x1, y1, x2, y2, 1, h.lineColor, true)

		// Cardinal labels
		if deg%90 == 0 {
			labelR := float32(radius) - 30
			lx := float32(cx) + labelR*float32(math.Cos(rad))
			ly := float32(cy) + labelR*float32(math.Sin(rad))

			label := ""
			switch deg {
			case 0:
				label = "N"
			case 90:
				label = "E"
			case 180:
				label = "S"
			case 270:
				label = "W"
			}
			ebitenutil.DebugPrintAt(screen, label, int(lx)-4, int(ly)-6)
		}
	}

	// Fixed heading pointer at top
	vector.DrawFilledRect(screen, float32(cx)-2, float32(cy-radius)+2, 4, 15, h.accentColor, true)

	// Aircraft symbol in center
	vector.DrawFilledCircle(screen, float32(cx), float32(cy), 3, h.accentColor, true)

	// Border
	vector.StrokeCircle(screen, float32(cx), float32(cy), float32(radius), 2, h.lineColor, true)

	// Heading readout
	hdgStr := fmt.Sprintf("%03.0f°", heading)
	ebitenutil.DebugPrintAt(screen, hdgStr, cx-15, cy+radius+5)
}

// drawSpeedTape renders the airspeed indicator tape
func (h *CockpitHUD) drawSpeedTape(screen *ebiten.Image, x, y, width, height int, speed float32) {
	// Background
	vector.DrawFilledRect(screen, float32(x), float32(y-height/2), float32(width), float32(height), h.bgColor, true)

	// Speed scale (pixels per km/h)
	scale := float32(height) / 100.0 // 100 km/h visible range

	// Draw speed ladder
	minSpeed := int(speed) - 50
	maxSpeed := int(speed) + 50

	for spd := (minSpeed / 10) * 10; spd <= maxSpeed; spd += 10 {
		offset := (speed - float32(spd)) * scale
		ly := float32(y) + offset

		if ly < float32(y-height/2) || ly > float32(y+height/2) {
			continue
		}

		// Tick mark
		tickLen := float32(10)
		if spd%50 == 0 {
			tickLen = 20
		}

		vector.StrokeLine(screen, float32(x+width)-tickLen, ly, float32(x+width), ly, 1, h.lineColor, true)

		// Label
		if spd%20 == 0 && spd >= 0 {
			ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%d", spd), x+2, int(ly)-6)
		}
	}

	// Current speed box
	boxH := float32(20)
	vector.DrawFilledRect(screen, float32(x), float32(y)-boxH/2, float32(width), boxH, color.RGBA{0, 0, 0, 255}, true)
	vector.StrokeRect(screen, float32(x), float32(y)-boxH/2, float32(width), boxH, 2, h.accentColor, true)

	// Speed value
	spdStr := fmt.Sprintf("%.0f", speed)
	ebitenutil.DebugPrintAt(screen, spdStr, x+5, y-6)

	// Border
	vector.StrokeRect(screen, float32(x), float32(y-height/2), float32(width), float32(height), 1, h.lineColor, true)

	// Label
	ebitenutil.DebugPrintAt(screen, "KM/H", x+5, y-height/2-15)
}

// drawAltitudeTape renders the altitude indicator tape
func (h *CockpitHUD) drawAltitudeTape(screen *ebiten.Image, x, y, width, height int, altitude float32) {
	// Background
	vector.DrawFilledRect(screen, float32(x), float32(y-height/2), float32(width), float32(height), h.bgColor, true)

	// Altitude scale (pixels per meter)
	scale := float32(height) / 200.0 // 200m visible range

	// Draw altitude ladder
	minAlt := int(altitude) - 100
	maxAlt := int(altitude) + 100

	for alt := (minAlt / 20) * 20; alt <= maxAlt; alt += 20 {
		offset := (altitude - float32(alt)) * scale
		ly := float32(y) + offset

		if ly < float32(y-height/2) || ly > float32(y+height/2) {
			continue
		}

		// Tick mark
		tickLen := float32(10)
		if alt%100 == 0 {
			tickLen = 20
		}

		vector.StrokeLine(screen, float32(x), ly, float32(x)+tickLen, ly, 1, h.lineColor, true)

		// Label
		if alt%50 == 0 {
			ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%d", alt), x+15, int(ly)-6)
		}
	}

	// Current altitude box
	boxH := float32(20)
	vector.DrawFilledRect(screen, float32(x), float32(y)-boxH/2, float32(width), boxH, color.RGBA{0, 0, 0, 255}, true)
	vector.StrokeRect(screen, float32(x), float32(y)-boxH/2, float32(width), boxH, 2, h.accentColor, true)

	// Altitude value
	altStr := fmt.Sprintf("%.0f", altitude)
	ebitenutil.DebugPrintAt(screen, altStr, x+5, y-6)

	// Border
	vector.StrokeRect(screen, float32(x), float32(y-height/2), float32(width), float32(height), 1, h.lineColor, true)

	// Label
	ebitenutil.DebugPrintAt(screen, "ALT m", x+2, y-height/2-15)
}

// drawVSI renders the vertical speed indicator
func (h *CockpitHUD) drawVSI(screen *ebiten.Image, x, y, width, height int, vspeed float32) {
	// Background
	vector.DrawFilledRect(screen, float32(x), float32(y-height/2), float32(width), float32(height), h.bgColor, true)

	// Scale: +/- 10 m/s range
	maxVS := float32(10.0)
	scale := float32(height/2) / maxVS

	// Center line (0)
	vector.StrokeLine(screen, float32(x), float32(y), float32(x+width), float32(y), 1, h.lineColor, true)

	// Tick marks
	for vs := -10; vs <= 10; vs += 2 {
		ly := float32(y) - float32(vs)*scale
		tickLen := float32(5)
		if vs%5 == 0 {
			tickLen = 10
			if vs != 0 {
				ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%+d", vs), x-20, int(ly)-6)
			}
		}
		vector.StrokeLine(screen, float32(x+width)-tickLen, ly, float32(x+width), ly, 1, h.lineColor, true)
	}

	// Current VS pointer
	clampedVS := vspeed
	if clampedVS > maxVS {
		clampedVS = maxVS
	} else if clampedVS < -maxVS {
		clampedVS = -maxVS
	}

	pointerY := float32(y) - clampedVS*scale
	pointerColor := h.textColor
	if vspeed < -3 {
		pointerColor = h.warningColor
	} else if vspeed > 3 {
		pointerColor = h.accentColor
	}

	// Pointer triangle
	vector.DrawFilledRect(screen, float32(x), pointerY-3, float32(width-5), 6, pointerColor, true)

	// Border
	vector.StrokeRect(screen, float32(x), float32(y-height/2), float32(width), float32(height), 1, h.lineColor, true)

	// Label and value
	ebitenutil.DebugPrintAt(screen, "VS", x+2, y-height/2-15)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%+.1f", vspeed), x-25, y+height/2+5)
}

// drawBatteryGauge renders the battery status
func (h *CockpitHUD) drawBatteryGauge(screen *ebiten.Image, x, y, width, height int, voltage, current float32, remaining uint32) {
	// Background
	vector.DrawFilledRect(screen, float32(x), float32(y), float32(width), float32(height), h.bgColor, true)

	// Battery icon outline
	battX := x + 5
	battY := y + 5
	battW := 30
	battH := 15

	// Determine color based on remaining
	battColor := h.textColor
	if remaining < 20 {
		battColor = h.warningColor
	} else if remaining < 40 {
		battColor = h.accentColor
	}

	// Battery outline
	vector.StrokeRect(screen, float32(battX), float32(battY), float32(battW), float32(battH), 1, h.lineColor, true)
	// Battery tip
	vector.DrawFilledRect(screen, float32(battX+battW), float32(battY+4), 3, 7, h.lineColor, true)

	// Fill based on remaining
	fillW := float32(battW-4) * float32(remaining) / 100.0
	if fillW > 0 {
		vector.DrawFilledRect(screen, float32(battX+2), float32(battY+2), fillW, float32(battH-4), battColor, true)
	}

	// Text info
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%.1fV", voltage), x+45, y+5)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%.1fA", current), x+95, y+5)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%d%%", remaining), x+45, y+25)

	// Label
	ebitenutil.DebugPrintAt(screen, "BATTERY", x+5, y+height-15)

	// Border
	vector.StrokeRect(screen, float32(x), float32(y), float32(width), float32(height), 1, h.lineColor, true)
}

// drawLinkQuality renders RF link status
func (h *CockpitHUD) drawLinkQuality(screen *ebiten.Image, x, y, width, height int, state TelemetryState) {
	// Background
	vector.DrawFilledRect(screen, float32(x), float32(y), float32(width), float32(height), h.bgColor, true)

	// Signal bars
	barW := 8
	barSpacing := 3
	maxBars := 5
	barsX := x + 10

	// Determine how many bars based on LQ
	activeBars := int(state.LinkQuality) / 20
	if activeBars > maxBars {
		activeBars = maxBars
	}

	barColor := h.textColor
	if state.LinkQuality < 50 {
		barColor = h.warningColor
	} else if state.LinkQuality < 70 {
		barColor = h.accentColor
	}

	for i := 0; i < maxBars; i++ {
		barH := 10 + i*5
		barY := y + height - 25 - barH

		c := color.RGBA{60, 60, 60, 255}
		if i < activeBars {
			c = barColor
		}

		vector.DrawFilledRect(screen, float32(barsX+i*(barW+barSpacing)), float32(barY), float32(barW), float32(barH), c, true)
	}

	// Text
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("LQ:%d%%", state.LinkQuality), x+70, y+10)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("RSSI:%d/%d", state.RSSI1, state.RSSI2), x+70, y+25)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("SNR:%d TX:%dW", state.SNR, state.TXPower), x+70, y+40)

	// Label
	ebitenutil.DebugPrintAt(screen, "RF LINK", x+5, y+5)

	// Border
	vector.StrokeRect(screen, float32(x), float32(y), float32(width), float32(height), 1, h.lineColor, true)
}

// drawGPSStatus renders GPS fix status
func (h *CockpitHUD) drawGPSStatus(screen *ebiten.Image, x, y, width, height int, state TelemetryState) {
	// Background
	vector.DrawFilledRect(screen, float32(x), float32(y), float32(width), float32(height), h.bgColor, true)

	// GPS icon (satellite dish)
	gpsColor := h.warningColor
	statusText := "NO FIX"
	if state.Satellites >= 4 {
		gpsColor = h.accentColor
		statusText = "3D FIX"
	}
	if state.Satellites >= 6 {
		gpsColor = h.textColor
		statusText = "GOOD"
	}

	// Satellite icon
	vector.DrawFilledCircle(screen, float32(x+20), float32(y+25), 8, gpsColor, true)
	vector.StrokeLine(screen, float32(x+20), float32(y+17), float32(x+28), float32(y+10), 2, gpsColor, true)

	// Text
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("SAT:%d", state.Satellites), x+40, y+10)
	ebitenutil.DebugPrintAt(screen, statusText, x+40, y+25)

	// Coords (truncated)
	if state.HasGPS {
		ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%.4f,%.4f", state.Latitude, state.Longitude), x+5, y+45)
	}

	// Border
	vector.StrokeRect(screen, float32(x), float32(y), float32(width), float32(height), 1, h.lineColor, true)
}

// Helper functions

func (h *CockpitHUD) rotatePoint(px, py, cx, cy float32, angle float64) (float32, float32) {
	cos := float32(math.Cos(angle))
	sin := float32(math.Sin(angle))

	px -= cx
	py -= cy

	newX := px*cos - py*sin + cx
	newY := px*sin + py*cos + cy

	return newX, newY
}

func (h *CockpitHUD) drawRotatedQuad(screen *ebiten.Image, cx, cy int, pts []float32, angle float64, c color.RGBA) {
	// Rotate all points
	rotated := make([]float32, len(pts))
	for i := 0; i < len(pts); i += 2 {
		rotated[i], rotated[i+1] = h.rotatePoint(pts[i], pts[i+1], float32(cx), float32(cy), angle)
	}

	// Draw as two triangles
	vs := []ebiten.Vertex{
		{DstX: rotated[0], DstY: rotated[1], SrcX: 0, SrcY: 0, ColorR: float32(c.R) / 255, ColorG: float32(c.G) / 255, ColorB: float32(c.B) / 255, ColorA: float32(c.A) / 255},
		{DstX: rotated[2], DstY: rotated[3], SrcX: 0, SrcY: 0, ColorR: float32(c.R) / 255, ColorG: float32(c.G) / 255, ColorB: float32(c.B) / 255, ColorA: float32(c.A) / 255},
		{DstX: rotated[4], DstY: rotated[5], SrcX: 0, SrcY: 0, ColorR: float32(c.R) / 255, ColorG: float32(c.G) / 255, ColorB: float32(c.B) / 255, ColorA: float32(c.A) / 255},
		{DstX: rotated[6], DstY: rotated[7], SrcX: 0, SrcY: 0, ColorR: float32(c.R) / 255, ColorG: float32(c.G) / 255, ColorB: float32(c.B) / 255, ColorA: float32(c.A) / 255},
	}

	is := []uint16{0, 1, 2, 0, 2, 3}

	screen.DrawTriangles(vs, is, emptySubImage, nil)
}

var emptySubImage = func() *ebiten.Image {
	img := ebiten.NewImage(1, 1)
	img.Fill(color.White)
	return img
}()
