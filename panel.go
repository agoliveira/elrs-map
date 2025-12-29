package main

import (
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const (
	PanelWidth = 280 // Left instrument panel width
)

// Panel renders the left instrument panel (INAV style)
type Panel struct {
	screenW, screenH int
	panelW           int

	// Colors
	panelBg       color.RGBA
	darkBg        color.RGBA
	tapeBg        color.RGBA // Semi-transparent for tapes
	textColor     color.RGBA
	skyColor      color.RGBA
	groundColor   color.RGBA
	accentColor   color.RGBA
	warningColor  color.RGBA
	goodColor     color.RGBA
	yellowColor   color.RGBA
}

// NewPanel creates a new instrument panel
func NewPanel() *Panel {
	return &Panel{
		panelW:       PanelWidth,
		panelBg:      color.RGBA{25, 25, 30, 255},
		darkBg:       color.RGBA{15, 15, 20, 255},
		tapeBg:       color.RGBA{0, 0, 0, 180}, // Semi-transparent black
		textColor:    color.RGBA{255, 255, 255, 255},
		skyColor:     color.RGBA{0, 119, 190, 255},
		groundColor:  color.RGBA{139, 90, 43, 255},
		accentColor:  color.RGBA{0, 200, 255, 255},
		warningColor: color.RGBA{255, 60, 60, 255},
		goodColor:    color.RGBA{0, 200, 0, 255},
		yellowColor:  color.RGBA{255, 200, 0, 255},
	}
}

// GetPanelWidth returns the panel width for map offset calculation
func (p *Panel) GetPanelWidth() int {
	return p.panelW
}

// Draw renders the full instrument panel
func (p *Panel) Draw(screen *ebiten.Image, state TelemetryState, homeSet bool, homeDist, homeBearing float64) {
	p.screenW, p.screenH = screen.Bounds().Dx(), screen.Bounds().Dy()

	// Panel background
	vector.DrawFilledRect(screen, 0, 0, float32(p.panelW), float32(p.screenH), p.panelBg, true)

	// === TOP STATUS BAR ===
	topBarH := 35
	p.drawTopBar(screen, state, homeSet, homeDist, homeBearing)

	// === MAIN ATTITUDE DISPLAY (with integrated tapes and compass) ===
	ahX := 10
	ahY := topBarH + 5
	ahW := p.panelW - 20
	ahH := 220
	
	p.drawAttitudeDisplay(screen, ahX, ahY, ahW, ahH, state)

	// === HORIZONTAL GAUGE BARS (INAV style) ===
	gaugeY := ahY + ahH + 15
	p.drawHorizontalGauges(screen, gaugeY, state)

	// Panel right border
	vector.StrokeLine(screen, float32(p.panelW), 0, float32(p.panelW), float32(p.screenH), 2, color.RGBA{60, 60, 70, 255}, true)
}

// drawTopBar draws the top status section
func (p *Panel) drawTopBar(screen *ebiten.Image, state TelemetryState, homeSet bool, homeDist, homeBearing float64) {
	// Background
	vector.DrawFilledRect(screen, 0, 0, float32(p.panelW), 35, p.darkBg, true)

	// Row 1: Battery | LQ | SAT
	battStr := fmt.Sprintf("%.1fV %d%%", state.Voltage, state.Remaining)
	if state.Remaining < 20 {
		p.drawTextWithBg(screen, battStr, 8, 3, p.warningColor)
	} else {
		ebitenutil.DebugPrintAt(screen, battStr, 8, 3)
	}

	lqStr := fmt.Sprintf("LQ:%d%%", state.LinkQuality)
	if state.LinkQuality < 50 {
		p.drawTextWithBg(screen, lqStr, 95, 3, p.warningColor)
	} else {
		ebitenutil.DebugPrintAt(screen, lqStr, 95, 3)
	}

	satStr := fmt.Sprintf("SAT:%d", state.Satellites)
	if state.Satellites < 4 {
		p.drawTextWithBg(screen, satStr, 165, 3, p.warningColor)
	} else if state.Satellites >= 6 {
		p.drawTextWithBg(screen, satStr, 165, 3, p.goodColor)
	} else {
		ebitenutil.DebugPrintAt(screen, satStr, 165, 3)
	}

	// Row 2: Home info
	if homeSet && state.HasGPS {
		var homeStr string
		if homeDist >= 1000 {
			homeStr = fmt.Sprintf("HOME: %.1fkm %03.0f°", homeDist/1000, homeBearing)
		} else {
			homeStr = fmt.Sprintf("HOME: %.0fm %03.0f°", homeDist, homeBearing)
		}
		if homeDist > 5000 {
			p.drawTextWithBg(screen, homeStr, 8, 18, p.warningColor)
		} else {
			ebitenutil.DebugPrintAt(screen, homeStr, 8, 18)
		}
		// Small direction arrow
		p.drawHomeArrow(screen, p.panelW-25, 24, state.Heading, homeBearing)
	} else {
		ebitenutil.DebugPrintAt(screen, "HOME: ---", 8, 18)
	}
}

// drawTextWithBg draws text with colored background
func (p *Panel) drawTextWithBg(screen *ebiten.Image, text string, x, y int, bg color.RGBA) {
	w := len(text)*7 + 4
	vector.DrawFilledRect(screen, float32(x-2), float32(y-1), float32(w), 14, bg, true)
	ebitenutil.DebugPrintAt(screen, text, x, y)
}

// drawHomeArrow draws small arrow pointing to home
func (p *Panel) drawHomeArrow(screen *ebiten.Image, cx, cy int, heading float32, homeBearing float64) {
	relBearing := (homeBearing - float64(heading)) * math.Pi / 180
	r := float32(10)
	
	tipX := float32(cx) + r*float32(math.Sin(relBearing))
	tipY := float32(cy) - r*float32(math.Cos(relBearing))
	
	vector.StrokeLine(screen, float32(cx), float32(cy), tipX, tipY, 2, p.accentColor, true)
}

// drawAttitudeDisplay draws the main attitude display with integrated elements
func (p *Panel) drawAttitudeDisplay(screen *ebiten.Image, x, y, w, h int, state TelemetryState) {
	cx := x + w/2
	cy := y + h/2
	
	// Pitch scale: pixels per degree
	pitchScale := float32(h) / 60.0 // Show +/- 30 degrees
	pitchOffset := state.Pitch * pitchScale

	// === 1. DRAW SKY AND GROUND ===
	horizonY := float32(cy) + pitchOffset

	// Sky
	if horizonY > float32(y) {
		skyH := horizonY - float32(y)
		if skyH > float32(h) {
			skyH = float32(h)
		}
		vector.DrawFilledRect(screen, float32(x), float32(y), float32(w), skyH, p.skyColor, true)
	}

	// Ground
	if horizonY < float32(y+h) {
		groundY := horizonY
		if groundY < float32(y) {
			groundY = float32(y)
		}
		groundH := float32(y+h) - groundY
		vector.DrawFilledRect(screen, float32(x), groundY, float32(w), groundH, p.groundColor, true)
	}

	// Horizon line
	if horizonY >= float32(y) && horizonY <= float32(y+h) {
		vector.StrokeLine(screen, float32(x), horizonY, float32(x+w), horizonY, 2, p.textColor, true)
	}

	// === 2. PITCH LADDER ===
	for deg := -40; deg <= 40; deg += 10 {
		if deg == 0 {
			continue
		}
		lineY := float32(cy) + (float32(deg)-state.Pitch)*pitchScale
		if lineY < float32(y+25) || lineY > float32(y+h-30) {
			continue
		}
		
		lineW := 60
		if deg%20 != 0 {
			lineW = 35
		}
		
		lx1 := float32(cx) - float32(lineW)/2
		lx2 := float32(cx) + float32(lineW)/2
		
		vector.StrokeLine(screen, lx1, lineY, lx2, lineY, 1, p.textColor, true)
		
		if deg%20 == 0 {
			label := fmt.Sprintf("%d", -deg)
			ebitenutil.DebugPrintAt(screen, label, int(lx2)+3, int(lineY)-6)
			ebitenutil.DebugPrintAt(screen, label, int(lx1)-20, int(lineY)-6)
		}
	}

	// === 3. ROLL ARC (inside top of A/H) ===
	p.drawRollArc(screen, cx, y+35, 50, state.Roll)

	// === 4. AIRCRAFT SYMBOL ===
	wingW := float32(70)
	wingH := float32(4)
	// Left wing
	vector.DrawFilledRect(screen, float32(cx)-wingW/2, float32(cy)-wingH/2, wingW/2-8, wingH, p.yellowColor, true)
	// Right wing  
	vector.DrawFilledRect(screen, float32(cx)+8, float32(cy)-wingH/2, wingW/2-8, wingH, p.yellowColor, true)
	// Center
	vector.DrawFilledCircle(screen, float32(cx), float32(cy), 5, p.yellowColor, true)
	vector.DrawFilledCircle(screen, float32(cx), float32(cy), 2, p.darkBg, true)

	// === 5. SPEED TAPE (left side, semi-transparent overlay) ===
	tapeW := 40
	p.drawSpeedTape(screen, x, y+25, tapeW, h-55, state.GroundSpeed)

	// === 6. ALTITUDE TAPE (right side, semi-transparent overlay) ===
	p.drawAltitudeTape(screen, x+w-tapeW, y+25, tapeW, h-55, int(state.Altitude))

	// === 7. COMPASS RIBBON (bottom, semi-transparent overlay) ===
	compassH := 25
	p.drawCompassRibbon(screen, x, y+h-compassH, w, compassH, state.Heading)

	// Border
	vector.StrokeRect(screen, float32(x), float32(y), float32(w), float32(h), 2, color.RGBA{60, 60, 70, 255}, true)
}

// drawRollArc draws the roll indicator arc inside top of A/H
func (p *Panel) drawRollArc(screen *ebiten.Image, cx, cy, radius int, roll float32) {
	r := float32(radius)
	
	// Draw arc background from -60 to +60 degrees (upward arc)
	for angle := -60; angle <= 60; angle += 3 {
		rad := float64(angle-90) * math.Pi / 180
		ax := float32(cx) + r*float32(math.Cos(rad))
		ay := float32(cy) + r*float32(math.Sin(rad))
		vector.DrawFilledCircle(screen, ax, ay, 1.5, color.RGBA{150, 150, 160, 255}, true)
	}
	
	// Tick marks
	ticks := []int{-60, -45, -30, -20, -10, 0, 10, 20, 30, 45, 60}
	for _, t := range ticks {
		rad := float64(t-90) * math.Pi / 180
		innerR := r - 6
		outerR := r + 4
		if t == 0 {
			outerR = r + 8
		}
		
		x1 := float32(cx) + innerR*float32(math.Cos(rad))
		y1 := float32(cy) + innerR*float32(math.Sin(rad))
		x2 := float32(cx) + outerR*float32(math.Cos(rad))
		y2 := float32(cy) + outerR*float32(math.Sin(rad))
		
		col := p.textColor
		if t == 0 {
			col = p.yellowColor
		}
		vector.StrokeLine(screen, x1, y1, x2, y2, 1, col, true)
	}
	
	// Roll pointer (moving triangle)
	rollRad := float64(-roll-90) * math.Pi / 180
	ptrR := r - 10
	ptrX := float32(cx) + ptrR*float32(math.Cos(rollRad))
	ptrY := float32(cy) + ptrR*float32(math.Sin(rollRad))
	
	// Small filled triangle pointing outward
	size := float32(6)
	outRad := rollRad + math.Pi // Point outward
	p1x := ptrX + size*float32(math.Cos(outRad))
	p1y := ptrY + size*float32(math.Sin(outRad))
	p2x := ptrX + size*0.6*float32(math.Cos(outRad+2.3))
	p2y := ptrY + size*0.6*float32(math.Sin(outRad+2.3))
	p3x := ptrX + size*0.6*float32(math.Cos(outRad-2.3))
	p3y := ptrY + size*0.6*float32(math.Sin(outRad-2.3))
	
	vector.StrokeLine(screen, p1x, p1y, p2x, p2y, 2, p.yellowColor, true)
	vector.StrokeLine(screen, p1x, p1y, p3x, p3y, 2, p.yellowColor, true)
	vector.StrokeLine(screen, p2x, p2y, p3x, p3y, 2, p.yellowColor, true)
}

// drawSpeedTape draws speed tape overlay on left
func (p *Panel) drawSpeedTape(screen *ebiten.Image, x, y, w, h int, speed float32) {
	// Semi-transparent background
	vector.DrawFilledRect(screen, float32(x), float32(y), float32(w), float32(h), p.tapeBg, true)
	
	cy := y + h/2
	scale := float32(h) / 80.0
	
	// Tick marks and numbers
	minSpd := int(speed) - 40
	maxSpd := int(speed) + 40
	if minSpd < 0 {
		minSpd = 0
	}
	
	for spd := (minSpd / 10) * 10; spd <= maxSpd; spd += 10 {
		yPos := float32(cy) - (float32(spd)-speed)*scale
		if yPos < float32(y+5) || yPos > float32(y+h-5) {
			continue
		}
		
		vector.StrokeLine(screen, float32(x+w-10), yPos, float32(x+w-2), yPos, 1, p.textColor, true)
		
		if spd%20 == 0 && spd >= 0 {
			label := fmt.Sprintf("%d", spd)
			ebitenutil.DebugPrintAt(screen, label, x+3, int(yPos)-6)
		}
	}
	
	// Current value box
	boxH := float32(16)
	vector.DrawFilledRect(screen, float32(x), float32(cy)-boxH/2, float32(w), boxH, p.accentColor, true)
	spdStr := fmt.Sprintf("%.0f", speed)
	ebitenutil.DebugPrintAt(screen, spdStr, x+5, cy-6)
	
	// Right border
	vector.StrokeLine(screen, float32(x+w), float32(y), float32(x+w), float32(y+h), 1, color.RGBA{80, 80, 90, 255}, true)
}

// drawAltitudeTape draws altitude tape overlay on right
func (p *Panel) drawAltitudeTape(screen *ebiten.Image, x, y, w, h, alt int) {
	// Semi-transparent background
	vector.DrawFilledRect(screen, float32(x), float32(y), float32(w), float32(h), p.tapeBg, true)
	
	cy := y + h/2
	scale := float32(h) / 200.0
	
	minAlt := alt - 100
	maxAlt := alt + 100
	
	for a := (minAlt / 20) * 20; a <= maxAlt; a += 20 {
		yPos := float32(cy) - (float32(a)-float32(alt))*scale
		if yPos < float32(y+5) || yPos > float32(y+h-5) {
			continue
		}
		
		vector.StrokeLine(screen, float32(x+2), yPos, float32(x+10), yPos, 1, p.textColor, true)
		
		if a%50 == 0 {
			label := fmt.Sprintf("%d", a)
			ebitenutil.DebugPrintAt(screen, label, x+12, int(yPos)-6)
		}
	}
	
	// Current value box
	boxH := float32(16)
	vector.DrawFilledRect(screen, float32(x), float32(cy)-boxH/2, float32(w), boxH, p.accentColor, true)
	altStr := fmt.Sprintf("%d", alt)
	ebitenutil.DebugPrintAt(screen, altStr, x+5, cy-6)
	
	// Left border
	vector.StrokeLine(screen, float32(x), float32(y), float32(x), float32(y+h), 1, color.RGBA{80, 80, 90, 255}, true)
}

// drawCompassRibbon draws compass at bottom of A/H
func (p *Panel) drawCompassRibbon(screen *ebiten.Image, x, y, w, h int, heading float32) {
	// Semi-transparent background
	vector.DrawFilledRect(screen, float32(x), float32(y), float32(w), float32(h), p.tapeBg, true)
	
	cx := x + w/2
	scale := float32(w) / 140.0
	
	cardinals := []struct {
		label string
		deg   float32
	}{
		{"N", 0}, {"NE", 45}, {"E", 90}, {"SE", 135},
		{"S", 180}, {"SW", 225}, {"W", 270}, {"NW", 315},
	}
	
	// Tick marks
	for deg := 0; deg < 360; deg += 15 {
		diff := float32(deg) - heading
		for diff > 180 {
			diff -= 360
		}
		for diff < -180 {
			diff += 360
		}
		
		if diff < -70 || diff > 70 {
			continue
		}
		
		xPos := float32(cx) + diff*scale
		tickH := 4
		if deg%30 == 0 {
			tickH = 8
		}
		
		vector.StrokeLine(screen, xPos, float32(y+h-tickH), xPos, float32(y+h-1), 1, p.textColor, true)
	}
	
	// Cardinals
	for _, c := range cardinals {
		diff := c.deg - heading
		for diff > 180 {
			diff -= 360
		}
		for diff < -180 {
			diff += 360
		}
		
		if diff < -65 || diff > 65 {
			continue
		}
		
		xPos := float32(cx) + diff*scale
		col := p.textColor
		if c.label == "N" {
			col = p.warningColor
		}
		
		labelX := int(xPos) - len(c.label)*3
		if col == p.warningColor {
			p.drawTextWithBg(screen, c.label, labelX, y+2, col)
		} else {
			ebitenutil.DebugPrintAt(screen, c.label, labelX, y+2)
		}
	}
	
	// Center pointer
	vector.DrawFilledRect(screen, float32(cx-1), float32(y), 3, float32(h), color.RGBA{255, 255, 0, 150}, true)
	
	// Heading readout
	hdgStr := fmt.Sprintf("%03.0f°", heading)
	hdgW := len(hdgStr)*7 + 4
	vector.DrawFilledRect(screen, float32(cx-hdgW/2), float32(y+h-16), float32(hdgW), 14, p.darkBg, true)
	ebitenutil.DebugPrintAt(screen, hdgStr, cx-hdgW/2+2, y+h-14)
	
	// Top border
	vector.StrokeLine(screen, float32(x), float32(y), float32(x+w), float32(y), 1, color.RGBA{80, 80, 90, 255}, true)
}

// drawHorizontalGauges draws INAV-style horizontal gauge bars
func (p *Panel) drawHorizontalGauges(screen *ebiten.Image, startY int, state TelemetryState) {
	barH := 18
	barW := p.panelW - 80
	labelW := 55
	spacing := 8
	x := 10
	
	// Background for gauge area
	vector.DrawFilledRect(screen, 0, float32(startY-5), float32(p.panelW), float32(4*(barH+spacing)+10), p.darkBg, true)

	// Battery
	battPct := float32(state.Remaining) / 100.0
	p.drawHorizontalBar(screen, x, startY, labelW, barW, barH, battPct, "Batt", fmt.Sprintf("%d%%", state.Remaining))

	// Link Quality
	lqPct := float32(state.LinkQuality) / 100.0
	p.drawHorizontalBar(screen, x, startY+barH+spacing, labelW, barW, barH, lqPct, "LQ", fmt.Sprintf("%d%%", state.LinkQuality))

	// RSSI (normalize -120 to -40)
	rssiNorm := float32(state.RSSI1+120) / 80.0
	if rssiNorm < 0 {
		rssiNorm = 0
	}
	if rssiNorm > 1 {
		rssiNorm = 1
	}
	p.drawHorizontalBar(screen, x, startY+(barH+spacing)*2, labelW, barW, barH, rssiNorm, "RSSI", fmt.Sprintf("%ddB", state.RSSI1))

	// SNR (normalize -10 to 20)
	snrNorm := float32(state.SNR+10) / 30.0
	if snrNorm < 0 {
		snrNorm = 0
	}
	if snrNorm > 1 {
		snrNorm = 1
	}
	p.drawHorizontalBar(screen, x, startY+(barH+spacing)*3, labelW, barW, barH, snrNorm, "SNR", fmt.Sprintf("%ddB", state.SNR))
}

// drawHorizontalBar draws a single horizontal gauge bar (INAV style)
func (p *Panel) drawHorizontalBar(screen *ebiten.Image, x, y, labelW, barW, h int, value float32, label, valueStr string) {
	if value < 0 {
		value = 0
	}
	if value > 1 {
		value = 1
	}
	
	// Label
	ebitenutil.DebugPrintAt(screen, label, x, y+2)
	
	// Bar background
	barX := x + labelW
	vector.DrawFilledRect(screen, float32(barX), float32(y), float32(barW), float32(h), color.RGBA{40, 40, 50, 255}, true)
	
	// Value fill
	fillW := int(float32(barW-4) * value)
	fillColor := p.getGaugeColor(value)
	vector.DrawFilledRect(screen, float32(barX+2), float32(y+2), float32(fillW), float32(h-4), fillColor, true)
	
	// Border
	vector.StrokeRect(screen, float32(barX), float32(y), float32(barW), float32(h), 1, color.RGBA{80, 80, 90, 255}, true)
	
	// Value text (right side)
	ebitenutil.DebugPrintAt(screen, valueStr, barX+barW+5, y+2)
}

// getGaugeColor returns color based on value (0-1)
func (p *Panel) getGaugeColor(value float32) color.RGBA {
	if value > 0.6 {
		return p.goodColor
	} else if value > 0.3 {
		return p.yellowColor
	}
	return p.warningColor
}
