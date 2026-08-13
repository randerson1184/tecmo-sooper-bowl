package render

import (
	"math"

	"github.com/randerson1184/tecmo-sooper-bowl/internal/field"
)

// Camera maps a zoomed window of the field onto the play rectangle.
// Focus is in field yards; Scale is pixels per yard.
type Camera struct {
	// Smoothed focus (yards).
	X, Y float64
	// Target focus (yards).
	TX, TY float64
	// Pixels per yard (higher = more zoomed in).
	Scale  float64
	TScale float64

	// Screen playfield rect (pixels) — set each frame from Layout.
	OriginX, OriginY float32
	PixelW, PixelH   float32
}

// Default camera: full-ish field view for play select.
func NewCamera() Camera {
	// Fit ~full width with a bit of margin.
	return Camera{
		X: field.HashMid, Y: 50,
		TX: field.HashMid, TY: 50,
		Scale: 6, TScale: 6,
	}
}

// SetTarget aims the camera (does not snap).
func (c *Camera) SetTarget(p field.Pos, scale float64) {
	c.TX = p.X
	c.TY = p.Y
	if scale > 0 {
		c.TScale = scale
	}
}

// Snap jumps immediately (reset / first frame).
func (c *Camera) Snap(p field.Pos, scale float64) {
	c.X, c.Y = p.X, p.Y
	c.TX, c.TY = p.X, p.Y
	c.Scale, c.TScale = scale, scale
}

// Update eases toward target.
func (c *Camera) Update(dt float64) {
	// Critically-damped-ish lerp
	k := 1 - math.Exp(-8*dt)
	c.X += (c.TX - c.X) * k
	c.Y += (c.TY - c.Y) * k
	c.Scale += (c.TScale - c.Scale) * (1 - math.Exp(-6*dt))
}

// BindPlayRect tells the camera where on screen the field is drawn.
func (c *Camera) BindPlayRect(ox, oy, w, h float32) {
	c.OriginX, c.OriginY = ox, oy
	c.PixelW, c.PixelH = w, h
}

// visible yards
func (c *Camera) viewYards() (vw, vh float64) {
	vh = float64(c.PixelH) / c.Scale
	vw = float64(c.PixelW) / c.Scale
	return
}

// YardToScreen converts field yards → pixel center using the camera.
func (c *Camera) YardToScreen(p field.Pos) (sx, sy float32) {
	vw, vh := c.viewYards()
	// Clamp focus so we don't show huge empty outside (soft)
	// Screen: +X right, +Y down. Field +Y is north (toward top of screen).
	relX := p.X - (c.X - vw/2)
	relY := (c.Y + vh/2) - p.Y // invert Y
	sx = c.OriginX + float32(relX*c.Scale)
	sy = c.OriginY + float32(relY*c.Scale)
	return
}

// PixelsPerYard for sizing sprites.
func (c *Camera) PixelsPerYard() float32 {
	return float32(c.Scale)
}

// Suggested scales
const (
	ScaleSelect = 7.5  // pre-snap: see formation
	ScaleLive   = 14.0 // in play: tight on ball
	ScaleScore  = 11.0
)
