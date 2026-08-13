// Package render draws the field and units with Ebitengine.
package render

import (
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"github.com/randerson1184/tecmo-sooper-bowl/internal/field"
	"github.com/randerson1184/tecmo-sooper-bowl/internal/game"
	"github.com/randerson1184/tecmo-sooper-bowl/internal/playbook"
	"github.com/randerson1184/tecmo-sooper-bowl/internal/sim"
)

// Layout is the screen chrome around the play surface.
type Layout struct {
	ScreenW   int
	ScreenH   int
	PadTop    float32
	PadBottom float32
	PadX      float32
}

func (l Layout) FieldRect() (x, y, w, h float32) {
	x = l.PadX
	y = l.PadTop
	w = float32(l.ScreenW) - l.PadX*2
	h = float32(l.ScreenH) - l.PadTop - l.PadBottom
	return
}

// DrawField paints grass, yard lines, and end zones in the camera window.
func DrawField(dst *ebiten.Image, l Layout, cam *Camera, m *game.Match) {
	fx, fy, fw, fh := l.FieldRect()
	cam.BindPlayRect(fx, fy, fw, fh)

	// Clip-ish: fill play rect with dark, then field pieces
	vector.DrawFilledRect(dst, fx, fy, fw, fh, color.RGBA{12, 40, 22, 255}, false)

	// Draw field as a grid of yard bands within camera view
	// End zones 0 and 100
	drawYardBand(dst, cam, -field.EndZone, 0, color.RGBA{18, 85, 38, 255})
	drawYardBand(dst, cam, 100, 100+field.EndZone, color.RGBA{18, 85, 38, 255})
	// Main field in 5-yard stripes for depth
	for y := 0; y < 100; y += 5 {
		c := color.RGBA{34, 120, 50, 255}
		if (y/5)%2 == 0 {
			c = color.RGBA{32, 112, 48, 255}
		}
		drawYardBand(dst, cam, float64(y), float64(y+5), c)
	}

	// Yard lines every 10
	for yard := 0; yard <= 100; yard += 10 {
		x0, y0 := cam.YardToScreen(field.Pos{X: 0, Y: float64(yard)})
		x1, y1 := cam.YardToScreen(field.Pos{X: field.WidthYards, Y: float64(yard)})
		vector.StrokeLine(dst, x0, y0, x1, y1, 1.5, color.RGBA{230, 230, 230, 180}, false)
		if yard > 0 && yard < 100 {
			label := fmt.Sprintf("%d", yard)
			if yard > 50 {
				label = fmt.Sprintf("%d", 100-yard)
			}
			ebitenutil.DebugPrintAt(dst, label, int(x0+6), int(y0-7))
		}
	}

	// Sidelines
	s0x, s0y := cam.YardToScreen(field.Pos{X: 0, Y: -field.EndZone})
	s1x, s1y := cam.YardToScreen(field.Pos{X: 0, Y: 100 + field.EndZone})
	vector.StrokeLine(dst, s0x, s0y, s1x, s1y, 2, color.RGBA{250, 250, 250, 220}, false)
	e0x, e0y := cam.YardToScreen(field.Pos{X: field.WidthYards, Y: -field.EndZone})
	e1x, e1y := cam.YardToScreen(field.Pos{X: field.WidthYards, Y: 100 + field.EndZone})
	vector.StrokeLine(dst, e0x, e0y, e1x, e1y, 2, color.RGBA{250, 250, 250, 220}, false)

	// LOS + first down
	lx0, ly0 := cam.YardToScreen(field.Pos{X: 0, Y: m.LineOfScrimmage})
	lx1, ly1 := cam.YardToScreen(field.Pos{X: field.WidthYards, Y: m.LineOfScrimmage})
	vector.StrokeLine(dst, lx0, ly0, lx1, ly1, 3, color.RGBA{80, 170, 255, 255}, false)
	fx0, fy0 := cam.YardToScreen(field.Pos{X: 0, Y: m.FirstDownMarker})
	fx1, fy1 := cam.YardToScreen(field.Pos{X: field.WidthYards, Y: m.FirstDownMarker})
	vector.StrokeLine(dst, fx0, fy0, fx1, fy1, 3, color.RGBA{255, 220, 40, 255}, false)

	// Frame
	vector.StrokeRect(dst, fx, fy, fw, fh, 2, color.RGBA{240, 240, 240, 255}, false)
}

func drawYardBand(dst *ebiten.Image, cam *Camera, y0, y1 float64, c color.RGBA) {
	// Quad as two triangles via filled rect in screen space (axis-aligned approx)
	// Corners of band
	tlx, tly := cam.YardToScreen(field.Pos{X: 0, Y: y1})
	brx, bry := cam.YardToScreen(field.Pos{X: field.WidthYards, Y: y0})
	x := tlx
	y := tly
	w := brx - tlx
	h := bry - tly
	if w < 0 {
		x = brx
		w = -w
	}
	if h < 0 {
		y = bry
		h = -h
	}
	vector.DrawFilledRect(dst, x, y, w, h, c, false)
}

// DrawUnits paints larger players with role tags. primaryIdx = pass target ring.
func DrawUnits(dst *ebiten.Image, cam *Camera, w *sim.World, primaryIdx int) {
	if w == nil {
		return
	}
	ppy := cam.PixelsPerYard()
	// ~0.9 yard body radius in pixels
	baseR := ppy * 0.55
	if baseR < 6 {
		baseR = 6
	}
	if baseR > 16 {
		baseR = 16
	}

	for i, u := range w.Units {
		sx, sy := cam.YardToScreen(u.Pos)
		var body color.RGBA
		if u.Side == sim.SideOffense {
			body = color.RGBA{50, 110, 230, 255}
		} else {
			body = color.RGBA{210, 70, 55, 255}
		}
		r := baseR
		if u.HasBall {
			r = baseR * 1.25
			body = color.RGBA{255, 210, 60, 255}
		}
		// Juke flash
		if u.JukeTimer > 0 {
			vector.StrokeCircle(dst, sx, sy, r+4, 2, color.RGBA{255, 255, 255, 220}, false)
		}
		if i == primaryIdx && !u.HasBall {
			vector.StrokeCircle(dst, sx, sy, r+5, 2, color.RGBA{120, 255, 140, 255}, false)
		}
		// Shadow
		vector.DrawFilledCircle(dst, sx+1.5, sy+2, r*0.9, color.RGBA{0, 0, 0, 60}, false)
		// Body
		vector.DrawFilledCircle(dst, sx, sy, r, body, false)
		// Helmet highlight
		vector.DrawFilledCircle(dst, sx-r*0.25, sy-r*0.3, r*0.35, color.RGBA{255, 255, 255, 70}, false)
		// Role label
		label := string(u.Role)
		if u.HasBall {
			label = "*" + label
		}
		ebitenutil.DebugPrintAt(dst, label, int(sx-6), int(sy-4))
	}
}

// DrawBall draws the football in flight.
func DrawBall(dst *ebiten.Image, cam *Camera, p field.Pos) {
	sx, sy := cam.YardToScreen(p)
	ppy := cam.PixelsPerYard()
	r := ppy * 0.28
	if r < 3 {
		r = 3
	}
	vector.DrawFilledCircle(dst, sx, sy, r, color.RGBA{255, 190, 70, 255}, false)
}

// DrawHUD prints score, down/distance, stamina, tips.
func DrawHUD(dst *ebiten.Image, m *game.Match, selected playbook.Play, def playbook.DefenseCall, tip, phase string, stamina float64, jukeCD float64) {
	line1 := fmt.Sprintf("TECMO SOOPER BOWL  |  HOME %d  AWAY %d  |  Q%d  %d:%02d  |  %s",
		m.HomeScore, m.AwayScore, m.Quarter, m.ClockSec/60, m.ClockSec%60, phase)
	line2 := fmt.Sprintf("Ball on %.0f  |  %d & %.0f  |  PlayClock %d",
		m.BallY, m.Down, m.Distance, m.PlayClock)
	line3 := fmt.Sprintf("OFF: [%s]  DEF: [%s]  |  STAMINA %s  JUKE %s",
		selected.Name, def.Name, staminaBar(stamina), jukeReady(jukeCD))
	line4 := "1-4 play  SPACE snap/throw  Arrows move  SHIFT juke  R reset"
	if tip != "" {
		line4 = tip
	}
	ebitenutil.DebugPrint(dst, line1+"\n"+line2+"\n"+line3+"\n"+line4)

	// Stamina bar graphic under HUD text area
	drawMeter(dst, 8, 58, 160, 8, stamina, color.RGBA{80, 200, 120, 255}, color.RGBA{40, 40, 40, 200})
}

func staminaBar(s float64) string {
	if s < 0 {
		s = 0
	}
	if s > 1 {
		s = 1
	}
	return fmt.Sprintf("%d%%", int(s*100))
}

func jukeReady(cd float64) string {
	if cd <= 0 {
		return "READY"
	}
	return fmt.Sprintf("%.1fs", cd)
}

func drawMeter(dst *ebiten.Image, x, y, w, h float32, fill float64, fg, bg color.RGBA) {
	if fill < 0 {
		fill = 0
	}
	if fill > 1 {
		fill = 1
	}
	vector.DrawFilledRect(dst, x, y, w, h, bg, false)
	vector.DrawFilledRect(dst, x, y, w*float32(fill), h, fg, false)
	vector.StrokeRect(dst, x, y, w, h, 1, color.RGBA{220, 220, 220, 180}, false)
}
