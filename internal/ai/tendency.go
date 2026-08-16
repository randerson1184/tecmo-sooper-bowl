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
	Yards     float64
	Outcome   string // tackle, incomplete, sack, touchdown, ...
	QBKeep    bool   // pass call, never threw, QB ran it
}

// Success is effectiveness, not "we called a run."
func (o PlayObservation) Success() bool {
	if o.Type == playbook.PlayRun {
		return o.Yards >= 4 || o.Outcome == "touchdown"
	}
	if o.Outcome == "sack" || o.Outcome == "incomplete" || o.Outcome == "none" || o.Outcome == "" {
		return false
	}
	return o.Yards >= 0
}

func (o PlayObservation) Explosive() bool {
	return o.Yards >= 12 || o.Outcome == "touchdown"
}

// Tracker remembers recent offensive tendencies.
type Tracker struct {
	// Window is max plays kept (oldest dropped).
	Window int
	Plays  []PlayObservation
}

func NewTracker(window int) *Tracker {
	if window <= 0 {
		window = 12
	}
	if window > 12 {
		window = 12 // small rolling window — early calls must not haunt the game
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
	Samples  int
	RunPct   float64
	PassPct  float64
	LeftPct  float64
	RightPct float64
	MidPct   float64

	RunN, PassN           int
	RunSuccessPct         float64
	PassSuccessPct        float64
	RunYds, PassYds       float64
	RunThreat, PassThreat float64 // decayed success+explosives, capped
	KeepThreat            float64
	KeepN                 int
	KeepYds               float64
	ThrowN                int // actual throws (called pass, not a keep)
}

func (t *Tracker) Snapshot() Snapshot {
	n := len(t.Plays)
	if n == 0 {
		return Snapshot{}
	}
	var runs, passes, left, right, mid int
	var runOK, passOK int
	var runYds, passYds, keepYds float64
	var runThreat, passThreat, keepThreat, w float64
	var keepN, throwN int
	w = 1
	for i := n - 1; i >= 0; i-- {
		p := t.Plays[i]
		score := 0.0
		if p.Success() {
			score += 1
		}
		if p.Explosive() {
			score += 1
		}
		if p.Type == playbook.PlayRun {
			runs++
			runYds += p.Yards
			if p.Success() {
				runOK++
			}
			runThreat += score * w
		} else {
			// Called pass — frequency. Effectiveness splits throw vs keep.
			passes++
			if p.QBKeep {
				keepN++
				keepYds += p.Yards
				if p.Success() {
					ks := 1.0
					if p.Explosive() {
						ks += 1
					}
					keepThreat += ks * w
				}
			} else {
				throwN++
				passYds += p.Yards
				if p.Success() {
					passOK++
				}
				passThreat += score * w
			}
		}
		switch p.Side {
		case playbook.SideLeft:
			left++
		case playbook.SideRight:
			right++
		default:
			mid++
		}
		w *= 0.85
	}
	if runThreat > 4 {
		runThreat = 4
	}
	if passThreat > 4 {
		passThreat = 4
	}
	if keepThreat > 4 {
		keepThreat = 4
	}
	fn := float64(n)
	s := Snapshot{
		Samples:    n,
		RunPct:     float64(runs) / fn,
		PassPct:    float64(passes) / fn,
		LeftPct:    float64(left) / fn,
		RightPct:   float64(right) / fn,
		MidPct:     float64(mid) / fn,
		RunN:       runs,
		PassN:      passes,
		RunYds:     runYds,
		PassYds:    passYds,
		RunThreat:  runThreat,
		PassThreat: passThreat,
		KeepThreat: keepThreat,
		KeepN:      keepN,
		KeepYds:    keepYds,
		ThrowN:     throwN,
	}
	if runs > 0 {
		s.RunSuccessPct = float64(runOK) / float64(runs)
	}
	if throwN > 0 {
		s.PassSuccessPct = float64(passOK) / float64(throwN)
	}
	return s
}

// ChoosePackage picks an independent front and coverage shell.
func ChoosePackage(sit game.SituationClass, snap Snapshot, rng *rand.Rand) playbook.Package {
	return ChooseStaffPackage(sit, snap, DefaultStaff(), rng)
}

// ChooseShell picks Cover 3 / Cover 2 / Man Free from situation + tendencies.
// Arcade: right-side pass diet (the hitch) → Cover 2 squat; pass-heavy → more man.
func ChooseShell(sit game.SituationClass, snap Snapshot, rng *rand.Rand) playbook.CoverageShell {
	w := map[string]float64{
		playbook.ShellCover3:  1.0,
		playbook.ShellCover2:  0.45,
		playbook.ShellManFree: 0.35,
	}
	switch sit {
	case game.SitShortYardage, game.SitGoalLine:
		w[playbook.ShellCover3] += 0.5
		w[playbook.ShellManFree] -= 0.1
	case game.SitThirdLong:
		w[playbook.ShellCover2] += 0.45
		w[playbook.ShellManFree] += 0.5
	case game.SitRedZone:
		w[playbook.ShellCover2] += 0.25
		w[playbook.ShellManFree] += 0.2
	}
	if snap.Samples >= 4 {
		if snap.PassPct >= 0.55 && snap.RightPct >= 0.45 {
			w[playbook.ShellCover2] += 1.1 // hitch diet: squat the flat
			w[playbook.ShellManFree] += 0.35
		}
		if snap.PassPct >= 0.65 {
			w[playbook.ShellManFree] += 0.55
			w[playbook.ShellCover2] += 0.25
		}
		if snap.RunPct >= 0.65 {
			w[playbook.ShellCover3] += 0.8
			w[playbook.ShellCover2] -= 0.15
		}
	}
	id := weightedPick(w, rng)
	return playbook.ShellByID(id)
}

func weightedPick(w map[string]float64, rng *rand.Rand) string {
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
	var last string
	for _, p := range list {
		acc += p.w
		last = p.id
		if r <= acc {
			return p.id
		}
	}
	return last
}

// ChooseDefense picks a defensive *front* from situation + tendencies.
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
		if snap.RunThreat >= 2 {
			w["run_fit"] += 0.9 // they actually gained yards
		}
		if snap.PassPct >= 0.65 {
			w["pass_rush"] += 0.5
			if snap.RunThreat >= 1.5 || snap.KeepThreat >= 1.5 {
				// Throwing a lot, but a run (designed or QB) already hurt us.
				w["base"] += 0.45
			} else {
				w["soft_zone"] += 0.7
				w["run_fit"] -= 0.3
			}
		}
		if snap.PassThreat >= 2 {
			w["pass_rush"] += 0.45
			if snap.RunThreat < 1.5 && snap.KeepThreat < 1.5 {
				w["soft_zone"] += 0.25
			}
		}
		if snap.KeepThreat >= 1.5 {
			// Distinct tools — not one magic counter.
			w["pass_rush"] += 0.4 // squeeze lanes
			w["run_fit"] += 0.45
			w["blitz"] -= 0.15 // don't empty the middle more
			w["soft_zone"] -= 0.45
		}
		// Right-side run that is actually gaining — shade the box, don't flip it.
		if snap.RightPct >= 0.4 && snap.RunThreat >= 1.5 {
			w["run_fit"] += 0.55
		}
		// Frequency without success: a nudge, not a lock.
		if snap.RunPct >= 0.5 && snap.RightPct >= 0.45 {
			w["run_fit"] += 0.25
		}
	}

	id := weightedPick(w, rng)
	if c, ok := byID[id]; ok {
		return c
	}
	return byID["base"]
}
