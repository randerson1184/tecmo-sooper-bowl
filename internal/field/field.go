// Package field holds pure geometry for a 100-yard American football field.
// No rendering here — only coordinates and clamps.
package field

// Yards along the length of the field (offense typically moves +Y toward opponent end zone
// in our model: own goal line = 0, opponent goal = 100). Sideline width is in yards too.
const (
	LengthYards = 100.0
	WidthYards  = 53.3 // official ~53.333
	EndZone     = 10.0 // drawn beyond 0 and 100; playable ball is 0..100
)

// Pos is a position on the field in yards.
// X: 0 = left sideline, WidthYards = right sideline.
// Y: 0 = south goal line, 100 = north goal line.
type Pos struct {
	X, Y float64
}

// Clamp keeps a position in bounds (sidelines + goal lines).
func Clamp(p Pos) Pos {
	if p.X < 0 {
		p.X = 0
	}
	if p.X > WidthYards {
		p.X = WidthYards
	}
	if p.Y < 0 {
		p.Y = 0
	}
	if p.Y > LengthYards {
		p.Y = LengthYards
	}
	return p
}

// InEndZone returns -1 (south), +1 (north), or 0 (field of play).
// Touchdown logic uses ball Y crossing 0 or 100 with possession direction later.
func InEndZone(y float64) int {
	if y <= 0 {
		return -1
	}
	if y >= LengthYards {
		return 1
	}
	return 0
}

// Hash marks as fractions of width (approx).
const (
	HashLeft  = WidthYards * 0.35
	HashRight = WidthYards * 0.65
	HashMid   = WidthYards * 0.5
)
