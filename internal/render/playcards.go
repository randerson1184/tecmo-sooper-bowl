package render

import (
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"github.com/randerson1184/tecmo-sooper-bowl/internal/field"
	"github.com/randerson1184/tecmo-sooper-bowl/internal/playbook"
)

// DrawPlayCards paints the four slots in the empty sideline gutters.
// 1 Inside / 3 Quick on the left; 2 Outside / 4 Shot on the right.
func DrawPlayCards(dst *ebiten.Image, l Layout, cam *Camera, slots []playbook.Slot, slotI int, selected playbook.Play) {
	if dst == nil || cam == nil || len(slots) == 0 {
		return
	}
	fx, fy, fw, fh := l.FieldRect()
	leftSideline, _ := cam.YardToScreen(field.Pos{X: 0, Y: cam.Y})
	rightSideline, _ := cam.YardToScreen(field.Pos{X: field.WidthYards, Y: cam.Y})

	leftW := leftSideline - fx - 10
	rightW := fx + fw - rightSideline - 10
	if leftW < 72 && rightW < 72 {
		return // live zoom — field owns the screen
	}

	gap := float32(8)
	cardH := (fh - gap*3) / 2
	if cardH < 80 {
		return
	}

	playFor := func(si int) playbook.Play {
		s := slots[si]
		if si == slotI {
			return selected
		}
		if len(s.Plays) == 0 {
			return playbook.Play{}
		}
		return s.Plays[0]
	}

	if leftW >= 72 {
		drawPlayCard(dst, fx+4, fy+gap, leftW, cardH, slots[0], playFor(0), slotI == 0)
		drawPlayCard(dst, fx+4, fy+gap*2+cardH, leftW, cardH, slots[2], playFor(2), slotI == 2)
	}
	if rightW >= 72 {
		rx := rightSideline + 6
		drawPlayCard(dst, rx, fy+gap, rightW, cardH, slots[1], playFor(1), slotI == 1)
		drawPlayCard(dst, rx, fy+gap*2+cardH, rightW, cardH, slots[3], playFor(3), slotI == 3)
	}
}

func drawPlayCard(dst *ebiten.Image, x, y, w, h float32, slot playbook.Slot, play playbook.Play, on bool) {
	bg := color.RGBA{16, 20, 28, 230}
	border := color.RGBA{90, 100, 115, 220}
	if on {
		bg = color.RGBA{28, 36, 22, 240}
		border = color.RGBA{255, 210, 70, 255}
	}
	vector.DrawFilledRect(dst, x, y, w, h, bg, false)
	vector.StrokeRect(dst, x, y, w, h, 1.5, border, false)

	title := fmt.Sprintf("%d  %s", slot.Key, play.Name)
	if play.Name == "" {
		title = fmt.Sprintf("%d  %s", slot.Key, slot.Name)
	}
	ebitenutil.DebugPrintAt(dst, title, int(x+6), int(y+4))

	foot := float32(0)
	if slot.Key == 3 {
		ebitenutil.DebugPrintAt(dst, "Shift+3 cycle", int(x+6), int(y+h-14))
		foot = 14
	} else if slot.Key == 4 {
		ebitenutil.DebugPrintAt(dst, "Shift+4 cycle", int(x+6), int(y+h-14))
		foot = 14
	}

	// Mini field inside the card
	mx := x + 8
	my := y + 20
	mw := w - 16
	mh := h - 28 - foot
	if mw < 40 || mh < 40 {
		return
	}
	vector.DrawFilledRect(dst, mx, my, mw, mh, color.RGBA{22, 70, 36, 255}, false)
	// LOS ~30% up from the bottom of the mini field
	losY := my + mh*0.68
	vector.StrokeLine(dst, mx, losY, mx+mw, losY, 1.2, color.RGBA{80, 170, 255, 220}, false)

	drawDiagram(dst, mx, my, mw, mh, losY, play.ID, on)
}

// Mini-field local: x 0..1 left→right, y 0 at LOS toward offense backfield,
// y 1 at the top of the card (downfield). Screen +Y is down, so we invert.
func drawDiagram(dst *ebiten.Image, mx, my, mw, mh, losY float32, id string, on bool) {
	to := func(px, py float32) (float32, float32) {
		return mx + px*mw, losY - py*(losY-my-4)
	}
	pass := color.RGBA{220, 230, 240, 255}
	run := color.RGBA{255, 200, 70, 255}
	fake := color.RGBA{255, 230, 120, 200}
	if !on {
		pass = color.RGBA{170, 180, 190, 220}
		run = color.RGBA{200, 170, 80, 220}
	}

	dot := func(px, py float32, c color.RGBA, r float32) {
		sx, sy := to(px, py)
		vector.DrawFilledCircle(dst, sx, sy, r, c, false)
	}
	arrow := func(pts [][2]float32, c color.RGBA) {
		if len(pts) < 2 {
			return
		}
		for i := 0; i < len(pts)-1; i++ {
			x0, y0 := to(pts[i][0], pts[i][1])
			x1, y1 := to(pts[i+1][0], pts[i+1][1])
			vector.StrokeLine(dst, x0, y0, x1, y1, 2, c, false)
		}
		// Head
		a := pts[len(pts)-2]
		b := pts[len(pts)-1]
		x0, y0 := to(a[0], a[1])
		x1, y1 := to(b[0], b[1])
		dx, dy := x1-x0, y1-y0
		n := float32(math.Hypot(float64(dx), float64(dy)))
		if n < 1 {
			return
		}
		ux, uy := dx/n, dy/n
		vector.StrokeLine(dst, x1, y1, x1-ux*7-uy*4, y1-uy*7+ux*4, 2, c, false)
		vector.StrokeLine(dst, x1, y1, x1-ux*7+uy*4, y1-uy*7-ux*4, 2, c, false)
	}

	ol := color.RGBA{70, 130, 230, 255}
	for i := 0; i < 5; i++ {
		dot(0.28+float32(i)*0.11, 0.04, ol, 3)
	}
	dot(0.50, -0.16, ol, 3.4)                            // QB
	dot(0.50, -0.06, color.RGBA{255, 210, 60, 255}, 3.2) // RB

	switch id {
	case "inside_zone":
		arrow([][2]float32{{0.50, -0.06}, {0.52, 0.18}, {0.54, 0.55}}, run)
	case "sweep":
		arrow([][2]float32{{0.50, -0.06}, {0.68, 0.02}, {0.84, 0.12}, {0.90, 0.42}}, run)
	case "slant":
		dot(0.16, 0.10, color.RGBA{80, 160, 255, 255}, 3.2)
		arrow([][2]float32{{0.16, 0.10}, {0.20, 0.32}, {0.48, 0.52}}, pass)
	case "hitch":
		dot(0.84, 0.10, color.RGBA{80, 160, 255, 255}, 3.2)
		arrow([][2]float32{{0.84, 0.10}, {0.86, 0.38}, {0.82, 0.38}}, pass)
	case "post":
		dot(0.62, 0.10, color.RGBA{80, 160, 255, 255}, 3.2)
		arrow([][2]float32{{0.62, 0.10}, {0.62, 0.48}, {0.42, 0.78}}, pass)
	case "pa_post":
		// Same fake + TE stem as glance; this one keeps climbing to the post.
		arrow([][2]float32{{0.50, -0.06}, {0.50, 0.16}}, fake)
		dot(0.18, 0.10, color.RGBA{80, 160, 255, 255}, 2.6)
		arrow([][2]float32{{0.18, 0.10}, {0.18, 0.72}}, pass)
		dot(0.62, 0.10, color.RGBA{80, 160, 255, 255}, 3.2)
		arrow([][2]float32{{0.62, 0.10}, {0.62, 0.48}, {0.42, 0.78}}, pass)
	case "pa_glance":
		// Same picture as PA Post: mesh + TE stem. He sits at 11 beside the A-gap.
		arrow([][2]float32{{0.50, -0.06}, {0.50, 0.16}}, fake)
		dot(0.18, 0.10, color.RGBA{80, 160, 255, 255}, 2.6)
		arrow([][2]float32{{0.18, 0.10}, {0.18, 0.72}}, pass)
		dot(0.62, 0.10, color.RGBA{80, 160, 255, 255}, 3.2)
		arrow([][2]float32{{0.62, 0.10}, {0.62, 0.48}, {0.54, 0.50}}, pass)
	default:
		arrow([][2]float32{{0.50, -0.06}, {0.50, 0.45}}, run)
	}
}
