// Package sim owns placement, movement, collision, tackle, and throw resolution.
package sim

import "github.com/randerson1184/tecmo-sooper-bowl/internal/field"

// Team side for a unit.
type Side int

const (
	SideOffense Side = iota
	SideDefense
)

// Role is a coarse position label for MVP placement.
type Role string

const (
	RoleQB Role = "QB"
	RoleRB Role = "RB"
	RoleWR Role = "WR"
	RoleTE Role = "TE"
	RoleOL Role = "OL"
	RoleDL Role = "DL"
	RoleLB Role = "LB"
	RoleCB Role = "CB"
	RoleS  Role = "S"
)

// Unit is one player on the field.
type Unit struct {
	ID   int
	Side Side
	Role Role
	Pos  field.Pos
	// Vel in yards per second.
	VX, VY float64
	// BaseSpeed is fresh speed; Speed is current (fatigue / juke modified).
	BaseSpeed float64
	Speed     float64
	HasBall   bool

	// AI route / assignment target.
	Target    field.Pos
	HasTarget bool

	// JukeTimer > 0: brief evade window (not easily tackled).
	JukeTimer float64

	// BlockTarget is the defender unit ID this OL/TE is assigned to (-1 = none).
	BlockTarget int
	// Engaged > 0: this defender is currently sealed / driven (seconds remaining feel via decay).
	Engaged float64
	// RushFree: blitz LB who must not be claimed by an OL.
	RushFree bool
	// ContainEdge: this defender sets the alley on stretch (every front gets one).
	ContainEdge bool
	// ContainForceX / ContainSpd are written once in setSweepContain.
	// Live pursuit must read these — do not re-derive a second table.
	ContainForceX float64
	ContainSpd    float64
	// Spy: hole player vs QB keep. Does not drop into coverage.
	Spy bool

	// Coverage assignment (defenders only). Landmark + optional man.
	CoverJob  CoverJob
	CoverLand field.Pos
	CoverMan  int // unit index; -1 if zone
}

// World is the in-play simulation snapshot.
type World struct {
	Units []Unit
	Ball  field.Pos
}

// PlacePreSnap builds a crude I-formation vs base 4-3 look at the given LOS (ball Y).
func PlacePreSnap(losY float64) *World {
	mid := field.HashMid
	w := &World{Ball: field.Pos{X: mid, Y: losY}}

	off := []struct {
		role   Role
		dx, dy float64
	}{
		{RoleQB, 0, -5},
		{RoleRB, 0, -8},
		{RoleWR, -18, -1},
		{RoleWR, 18, -1},
		{RoleTE, 6, -1},
		{RoleOL, -4, -1},
		{RoleOL, -2, -1},
		{RoleOL, 0, -1},
		{RoleOL, 2, -1},
		{RoleOL, 4, -1},
		{RoleWR, -12, -3},
	}
	id := 1
	for _, o := range off {
		sp := roleSpeed(o.role)
		w.Units = append(w.Units, Unit{
			ID:          id,
			Side:        SideOffense,
			Role:        o.role,
			Pos:         field.Clamp(field.Pos{X: mid + o.dx, Y: losY + o.dy}),
			HasBall:     o.role == RoleQB,
			BaseSpeed:   sp,
			Speed:       sp,
			BlockTarget: -1,
			CoverMan:    -1,
		})
		id++
	}

	def := []struct {
		role   Role
		dx, dy float64
	}{
		{RoleDL, -6, 1},
		{RoleDL, -2, 1},
		{RoleDL, 2, 1},
		{RoleDL, 6, 1},
		{RoleLB, -8, 4},
		{RoleLB, 0, 5},
		{RoleLB, 8, 4},
		{RoleCB, -18, 2},
		{RoleCB, 18, 2},
		{RoleS, -6, 12},
		{RoleS, 6, 12},
	}
	for _, d := range def {
		sp := roleSpeed(d.role)
		w.Units = append(w.Units, Unit{
			ID:          id,
			Side:        SideDefense,
			Role:        d.role,
			Pos:         field.Clamp(field.Pos{X: mid + d.dx, Y: losY + d.dy}),
			BaseSpeed:   sp,
			Speed:       sp,
			BlockTarget: -1,
			CoverMan:    -1,
		})
		id++
	}
	return w
}
