// Package ai owns CPU play calling, pursuit hooks, and tendency tracking.
// Phase 0: data structures + simple selection. Phase 3: real coverage brains.
package ai

import (
	"math/rand"

	"github.com/randerson1184/tecmo-sooper-bowl/internal/game"
	"github.com/randerson1184/tecmo-sooper-bowl/internal/playbook"
)

// PlayObservation is one offensive snap worth of tags.
type PlayObservation struct {
	Type playbook.PlayType
	Side playbook.Side
	// Depth 0=short/none, 1=intermediate, 2=deep
	Depth     int
	Situation game.SituationClass
}

// Tracker remembers recent offensive tendencies.
type Tracker struct {
	// Window is max plays kept (oldest dropped).
	Window int
	Plays  []PlayObservation
}

func NewTracker(window int) *Tracker {
	if window <= 0 {
		window = 16
	}
	return &Tracker{Window: window}
}

func (t *Tracker) Observe(o PlayObservation) {
	t.Plays = append(t.Plays, o)
	if len(t.Plays) > t.Window {
		t.Plays = t.Plays[len(t.Plays)-t.Window:]
	}
}

// Snapshot is a summary used by ChooseDefense.
type Snapshot struct {
	Samples int
	RunPct  float64
	PassPct float64
	LeftPct float64
	RightPct float64
	MidPct  float64
}

func (t *Tracker) Snapshot() Snapshot {
	n := len(t.Plays)
	if n == 0 {
		return Snapshot{}
	}
	var runs, passes, left, right, mid int
	for _, p := range t.Plays {
		if p.Type == playbook.PlayRun {
			runs++
		} else {
			passes++
		}
		switch p.Side {
		case playbook.SideLeft:
			left++
		case playbook.SideRight:
			right++
		default:
			mid++
		}
	}
	fn := float64(n)
	return Snapshot{
		Samples:  n,
		RunPct:   float64(runs) / fn,
		PassPct:  float64(passes) / fn,
		LeftPct:  float64(left) / fn,
		RightPct: float64(right) / fn,
		MidPct:   float64(mid) / fn,
	}
}

// ChooseDefense picks a defensive game plan from situation + tendencies.
// Intentionally simple for MVP; weights get smarter in Phase 3.
func ChooseDefense(sit game.SituationClass, snap Snapshot, rng *rand.Rand) playbook.DefenseCall {
	calls := playbook.DefaultDefenseCalls()
	byID := map[string]playbook.DefenseCall{}
	for _, c := range calls {
		byID[c.ID] = c
	}

	// Base weights
	w := map[string]float64{
		"base":      1.0,
		"run_fit":   0.4,
		"soft_zone": 0.4,
		"pass_rush": 0.3,
		"blitz":     0.2,
	}

	// Situation priors
	switch sit {
	case game.SitShortYardage, game.SitGoalLine:
		w["run_fit"] += 1.2
		w["blitz"] += 0.3
		w["soft_zone"] -= 0.2
	case game.SitThirdLong:
		w["pass_rush"] += 0.8
		w["soft_zone"] += 0.6
		w["blitz"] += 0.5
		w["run_fit"] -= 0.2
	case game.SitRedZone:
		w["run_fit"] += 0.5
		w["base"] += 0.3
	}

	// Tendency adjustments (need a few samples)
	if snap.Samples >= 4 {
		if snap.RunPct >= 0.65 {
			w["run_fit"] += 1.2
			w["soft_zone"] -= 0.35
			w["base"] -= 0.15
		}
		if snap.PassPct >= 0.65 {
			w["soft_zone"] += 0.7
			w["pass_rush"] += 0.5
			w["run_fit"] -= 0.3
		}
		// Bo-Jackson-right diet: load the edge
		if snap.RunPct >= 0.5 && snap.RightPct >= 0.45 {
			w["run_fit"] += 1.0
			w["blitz"] += 0.25
		}
	}
	// Even earlier signal if they're clearly pounding edge runs
	if snap.Samples >= 3 && snap.RunPct >= 0.7 && snap.RightPct >= 0.5 {
		w["run_fit"] += 0.8
	}

	// Weighted pick with floor so nothing goes negative.
	type pair struct {
		id string
		w  float64
	}
	var list []pair
	var total float64
	for id, weight := range w {
		if weight < 0.05 {
			weight = 0.05
		}
		list = append(list, pair{id, weight})
		total += weight
	}
	r := rng.Float64() * total
	var acc float64
	for _, p := range list {
		acc += p.w
		if r <= acc {
			return byID[p.id]
		}
	}
	return byID["base"]
}
