// Package ratings will hold player attributes (speed, power, hands, coverage).
// Phase 0: stub types only.
package ratings

// Attributes are 1..100 arcade ratings (Tecmo soul, not NFL Combine).
type Attributes struct {
	Speed    int
	Power    int
	Hands    int
	Coverage int
	Stamina  int
}

// DefaultSkillPosition is a generic solid starter.
func DefaultSkillPosition() Attributes {
	return Attributes{Speed: 70, Power: 70, Hands: 70, Coverage: 70, Stamina: 80}
}
