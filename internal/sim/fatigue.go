package sim

// Fatigue tracks drive-long wear on featured skill players (Tecmo soul).
// Values are 0 (fresh) .. 1 (gassed).
type Fatigue struct {
	RB float64
	QB float64
	WR float64
}

// Effective multiplies base speed for a role given fatigue.
func (f Fatigue) SpeedMult(role Role) float64 {
	var t float64
	switch role {
	case RoleRB:
		t = f.RB
	case RoleQB:
		t = f.QB
	case RoleWR, RoleTE:
		t = f.WR
	default:
		return 1
	}
	// Up to ~40% speed loss when fully gassed
	m := 1 - 0.40*t
	if m < 0.55 {
		m = 0.55
	}
	return m
}

// OnPlayEnd applies wear from a resolved snap and recovers everyone a bit.
func (f *Fatigue) OnPlayEnd(carrier Role, yards float64, wasPass bool, incomplete bool) {
	// Mild recovery each play for everyone
	f.RB = recover(f.RB, 0.06)
	f.QB = recover(f.QB, 0.05)
	f.WR = recover(f.WR, 0.05)

	// Workload cost
	work := 0.08 + 0.01*abs(yards)
	if work > 0.22 {
		work = 0.22
	}
	switch carrier {
	case RoleRB:
		// Edge diet wears the back out faster (anti-Bo cheese)
		mult := 1.15
		if yards >= 8 {
			mult = 1.45
		}
		f.RB = clamp01(f.RB + work*mult)
	case RoleQB:
		if wasPass {
			f.QB = clamp01(f.QB + 0.04)
		} else {
			f.QB = clamp01(f.QB + work)
		}
	case RoleWR, RoleTE:
		if incomplete {
			f.WR = clamp01(f.WR + 0.05)
		} else {
			f.WR = clamp01(f.WR + work*0.9)
		}
	}
}

// BetweenDrives soft reset (kickoff).
func (f *Fatigue) BetweenDrives() {
	f.RB *= 0.45
	f.QB *= 0.5
	f.WR *= 0.5
}

// Display returns the "featured" stamina for HUD (mostly RB on this game).
func (f Fatigue) Display(role Role) float64 {
	switch role {
	case RoleRB:
		return 1 - f.RB
	case RoleQB:
		return 1 - f.QB
	case RoleWR, RoleTE:
		return 1 - f.WR
	default:
		return 1 - f.RB
	}
}

func recover(v, amt float64) float64 {
	v -= amt
	if v < 0 {
		return 0
	}
	return v
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
