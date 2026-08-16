// Package sim: in-play football simulation (Phase 1 snap loop).
package sim

import (
	"math"
	"math/rand"

	"github.com/randerson1184/tecmo-sooper-bowl/internal/field"
	"github.com/randerson1184/tecmo-sooper-bowl/internal/playbook"
)

// Outcome of a resolved play.
type Outcome int

const (
	OutcomeNone Outcome = iota
	OutcomeTackle
	OutcomeOutOfBounds
	OutcomeIncomplete
	OutcomeTouchdown
	OutcomeSack
)

func (o Outcome) String() string {
	switch o {
	case OutcomeTackle:
		return "tackle"
	case OutcomeOutOfBounds:
		return "out_of_bounds"
	case OutcomeIncomplete:
		return "incomplete"
	case OutcomeTouchdown:
		return "touchdown"
	case OutcomeSack:
		return "sack"
	default:
		return "none"
	}
}

// Result is dead-ball info for the match layer.
type Result struct {
	Outcome     Outcome
	YardsGained float64 // from original LOS; 0 on incomplete/sack at/behind LOS handled via ball Y
	BallY       float64
	LOS         float64
	Message     string
	Thrown      bool // a pass left the QB's hand
	QBKeep      bool // pass call, QB ran it
	Carrier     Role // who had the ball at the whistle

	// Play-action film (zeroed on non-PA snaps).
	RunThreat   float64
	BiteSec     float64
	LeftoverSec float64
	ReleaseAt   float64
	Mesh        string
	BiterN      int
	Biters      string
	SepAtThrow  float64
}

// PlayState is one live snap.
type PlayState struct {
	World     *World
	Play      playbook.Play
	Def       playbook.DefenseCall
	Shell     playbook.CoverageShell
	Look      playbook.CoverageShell
	Disguised bool
	Line      LineContext
	LOS       float64

	// Player-controlled unit index into World.Units (-1 if none).
	ControlIdx int
	// Primary receiver index for pass plays.
	PrimaryIdx int
	// QB index.
	QBIdx int
	// RB index.
	RBIdx int

	// Ball in flight (pass).
	BallInAir  bool
	BallPos    field.Pos
	BallVelX   float64
	BallVelY   float64
	BallTarget int // unit index expected to catch; -1 none
	// ThrowTimer is seconds the ball has been in the air (0 at release).
	ThrowTimer float64
	// ThrowFlight is expected hang time at release (distance / ball speed).
	ThrowFlight float64
	HandoffDone bool
	HandoffAt   float64 // seconds after snap

	// Juke cooldown remaining (seconds).
	JukeCooldown float64
	// Seconds of "gather" after a hitch catch — WR is not a jet off the stop.
	HitchGather float64
	// Elapsed at catch (0 if still in the air / no catch).
	CaughtAt float64
	// Remaining pass-pro budget (seconds). Hits zero → rushers shed.
	PocketLeft float64
	Thrown     bool // a pass left the QB's hand
	QBKeep     bool // declared keep / scramble (defense plays the run)

	// Cached concept — do not rebuild playbook maps from the defender loop.
	Concept     playbook.Concept
	HasConcept  bool
	PlayAction  bool
	IsPostShot  bool
	BiteSec     float64
	LeftoverSec float64
	MeshSec     float64

	// Play-action mesh. Space during MeshLive is buffered; tryThrow aborts.
	Mesh          MeshPhase
	ThrowArmed    bool
	SnapRunThreat float64
	ReleaseAt     float64
	SepAtThrow    float64
	BiterIDs      []string
	BiterN        int

	Elapsed float64
	Alive   bool
	Result  Result

	// CarrierRole at whistle (for fatigue accounting).
	CarrierRole Role

	rng *rand.Rand
}

// Speed yards/sec by role (arcade, not real).
func roleSpeed(r Role) float64 {
	switch r {
	case RoleRB:
		return 9.5
	case RoleWR:
		return 10.0
	case RoleQB:
		return 7.5
	case RoleTE:
		return 8.0
	case RoleCB:
		return 9.8
	case RoleS:
		return 9.2
	case RoleLB:
		return 8.5
	case RoleDL:
		return 7.0
	case RoleOL:
		return 5.5
	default:
		return 7.0
	}
}

// StartPlay places the world and configures handoff / routes from the called play + defense.
// fat applies drive fatigue to offense skill speeds. Default shell is Cover 3.
func StartPlay(los float64, play playbook.Play, def playbook.DefenseCall, rng *rand.Rand, fat Fatigue) *PlayState {
	return StartSnap(los, play, def, playbook.DefaultShell(), rng, fat, LineContext{})
}

// StartSnap is StartPlay with an explicit coverage shell (front and shell are independent).
func StartSnap(los float64, play playbook.Play, def playbook.DefenseCall, shell playbook.CoverageShell, rng *rand.Rand, fat Fatigue, line LineContext) *PlayState {
	return StartSnapLook(los, play, def, shell, shell, rng, fat, line)
}

// StartSnapLook is StartSnap with a pre-snap picture that may differ from the live shell.
func StartSnapLook(los float64, play playbook.Play, def playbook.DefenseCall, shell, look playbook.CoverageShell, rng *rand.Rand, fat Fatigue, line LineContext) *PlayState {
	if rng == nil {
		rng = rand.New(rand.NewSource(1))
	}
	if shell.ID == "" {
		shell = playbook.DefaultShell()
	}
	if look.ID == "" {
		look = shell
	}
	w := PlacePreSnap(los)
	ps := &PlayState{
		World:      w,
		Play:       play,
		Def:        def,
		Shell:      shell,
		Look:       look,
		Disguised:  look.ID != shell.ID,
		Line:       line,
		LOS:        los,
		ControlIdx: -1,
		PrimaryIdx: -1,
		QBIdx:      -1,
		RBIdx:      -1,
		BallTarget: -1,
		Alive:      true,
		rng:        rng,
		HandoffAt:  0.30,
		PocketLeft: line.holdSeconds(def.ID),
		ReleaseAt:  -1,
	}
	ps.cacheConcept()
	// Toss sweep is a quick pitch
	if play.ID == "sweep" {
		ps.HandoffAt = 0.16
	}

	for i := range w.Units {
		u := &w.Units[i]
		u.HasBall = false
		u.BaseSpeed = roleSpeed(u.Role)
		u.Speed = u.BaseSpeed
		// Offense fatigue
		if u.Side == SideOffense {
			u.Speed *= fat.SpeedMult(u.Role)
			u.BaseSpeed = u.Speed
		}
		// Defense modifiers
		if u.Side == SideDefense {
			switch def.ID {
			case "run_fit":
				if u.Role == RoleLB || u.Role == RoleDL {
					u.Speed *= 1.10
					u.BaseSpeed = u.Speed
				}
			case "pass_rush", "blitz":
				if u.Role == RoleDL || u.Role == RoleLB {
					u.Speed *= 1.12
					u.BaseSpeed = u.Speed
				}
				if def.ID == "blitz" && u.Role == RoleLB {
					u.Speed *= 1.06
					u.BaseSpeed = u.Speed
				}
			case "soft_zone":
				if u.Role == RoleS || u.Role == RoleCB {
					u.Speed *= 1.03
					u.BaseSpeed = u.Speed
					// shade deep: start a bit deeper
					u.Pos.Y += 2.5
				}
			}
		}
		switch u.Role {
		case RoleQB:
			ps.QBIdx = i
		case RoleRB:
			ps.RBIdx = i
		}
	}

	// Sweep: slight outside align (not a free Super Bowl lane)
	if play.ID == "sweep" {
		if ps.RBIdx >= 0 {
			w.Units[ps.RBIdx].Pos.X += 3.5
			w.Units[ps.RBIdx].Pos = field.Clamp(w.Units[ps.RBIdx].Pos)
		}
		// Do not widen light boxes — that was the free alley. Pinch only on Run Fit.
		if def.ID == "run_fit" {
			for i := range w.Units {
				u := &w.Units[i]
				if u.Side != SideDefense || u.Pos.X <= field.HashMid {
					continue
				}
				if u.Role == RoleDL || u.Role == RoleLB || u.Role == RoleCB {
					u.Pos.X -= 0.5
					if u.Role == RoleLB || u.Role == RoleCB {
						u.Pos.Y -= 0.5
						u.Speed *= 1.08
						u.BaseSpeed = u.Speed
					}
					u.Pos = field.Clamp(u.Pos)
				}
			}
		}
	}

	// Assign primary receiver: first WR matching play side preference.
	ps.PrimaryIdx = pickPrimary(w, play)
	AssignCoverage(w, shell, los, ps.PrimaryIdx)
	// Jobs are the live shell; positions stay on the look so they rotate after the snap.
	if look.ID != shell.ID {
		shadeLookPicture(w, look, los)
	}
	if play.Type == playbook.PlayPass {
		assignPassPro(w, def)
		assignQBSpy(w, los, def, line)
		if play.ID == "slant" {
			shadeKeepHole(w, los, shell)
		}
	}

	if play.Type == playbook.PlayRun {
		// Ball starts with QB until handoff
		if ps.QBIdx >= 0 {
			w.Units[ps.QBIdx].HasBall = true
			ps.ControlIdx = ps.QBIdx // brief; switches on handoff
		}
	} else {
		if ps.QBIdx >= 0 {
			w.Units[ps.QBIdx].HasBall = true
			ps.ControlIdx = ps.QBIdx
		}
	}

	// Route targets for skill players (stored in unit.Target)
	assignRoutes(w, play, los, ps.PrimaryIdx, ps.Concept, ps.HasConcept)
	if ps.PlayAction {
		ps.armPlayAction()
	}
	if play.Type == playbook.PlayRun {
		assignRunBlocks(w, play, los)
		// Every front sets an edge on stretch. Run Fit is stronger, not unique.
		if play.ID == "sweep" {
			setSweepContain(w, def, shell)
		}
		// Run Fit vs inside zone: unblocked MLB in the A-gap.
		if play.ID == "inside_zone" && def.ID == "run_fit" {
			freeMiddleLB(w)
		}
		// Cover 2 vs inside zone: Mike fills — two-high is not a vacant alley.
		if play.ID == "inside_zone" && shell.ID == playbook.ShellCover2 && def.ID != "run_fit" {
			freeCover2Hole(w)
		}
		// Snap blockers partway toward their man (less teleport against loaded boxes)
		snapClose := 0.35
		eng := 0.35
		if play.ID == "sweep" && def.ID == "run_fit" {
			snapClose = 0.15
			eng = 0.12
		}
		if play.ID == "inside_zone" && def.ID != "run_fit" {
			snapClose = 0.48
			eng = 0.5
		}
		if play.ID == "inside_zone" && def.ID == "run_fit" {
			snapClose = 0.2
			eng = 0.18
		}
		for i := range w.Units {
			u := &w.Units[i]
			if u.BlockTarget < 0 || u.BlockTarget >= len(w.Units) {
				continue
			}
			if u.Side != SideOffense {
				continue
			}
			d := &w.Units[u.BlockTarget]
			u.Pos.X += (d.Pos.X - u.Pos.X) * snapClose
			u.Pos.Y += (d.Pos.Y - u.Pos.Y) * snapClose
			d.Engaged = eng
		}
	}
	// Quick-game: CBs start with a small cushion so slants/hitches aren't auto-jammed
	if play.Type == playbook.PlayPass {
		for i := range w.Units {
			u := &w.Units[i]
			if u.Role == RoleCB {
				u.Pos.Y += 1.5
			}
		}
	}
	return ps
}

// assignRunBlocks pairs each OL (and TE) with a defender so runs can create a lane.
func assignRunBlocks(w *World, play playbook.Play, los float64) {
	// Collect OL left-to-right and DL/LB candidates.
	var oline []int
	var front []int // DL first, then LBs
	var lbs []int
	for i, u := range w.Units {
		if u.Side == SideOffense && (u.Role == RoleOL || u.Role == RoleTE) {
			oline = append(oline, i)
		}
		if u.Side == SideDefense {
			if u.Role == RoleDL {
				front = append(front, i)
			}
			if u.Role == RoleLB {
				lbs = append(lbs, i)
			}
		}
	}
	// Sort oline by X
	sortByX(w, oline)
	sortByX(w, front)
	sortByX(w, lbs)

	// 1:1 OL to DL where possible
	for i, oi := range oline {
		w.Units[oi].BlockTarget = -1
		if i < len(front) {
			w.Units[oi].BlockTarget = front[i]
		} else if i-len(front) < len(lbs) {
			// Extra OL/TE climb to LBs
			w.Units[oi].BlockTarget = lbs[i-len(front)]
		}
	}

	// Sweep: overload the right edge — seal DE, kick out LB, crack support
	if play.ID == "sweep" {
		if len(front) > 0 && len(oline) > 0 {
			// Right DE sealed by right tackle
			w.Units[oline[len(oline)-1]].BlockTarget = front[len(front)-1]
		}
		if len(front) > 1 && len(oline) > 1 {
			// Next DL washed inside by right guard
			w.Units[oline[len(oline)-2]].BlockTarget = front[len(front)-2]
		}
		if len(lbs) > 0 && len(oline) > 2 {
			// Center/TE climb to right LB
			w.Units[oline[len(oline)-3]].BlockTarget = lbs[len(lbs)-1]
		}
		// TE (often last in oline if included) already assigned; right WR cracks CB below
		for i, u := range w.Units {
			if u.Side == SideOffense && u.Role == RoleWR && u.Pos.X > field.HashMid {
				// Crack the right CB / OLB
				best := -1
				bestD := 1e9
				for j, d := range w.Units {
					if d.Side != SideDefense {
						continue
					}
					if d.Role != RoleCB && d.Role != RoleLB {
						continue
					}
					if d.Pos.X < field.HashMid {
						continue
					}
					dd := math.Hypot(d.Pos.X-u.Pos.X, d.Pos.Y-u.Pos.Y)
					if dd < bestD {
						bestD = dd
						best = j
					}
				}
				if best >= 0 {
					w.Units[i].BlockTarget = best
				}
			}
		}
	}

	// Inside zone: combo the interior DTs so the A-gap isn't two free 1-techs.
	if play.ID == "inside_zone" && len(oline) >= 5 && len(front) >= 4 {
		// LT, LG, C, RG, RT — LG+C on LDT, RG on RDT (RT/LT keep ends)
		w.Units[oline[1]].BlockTarget = front[1]
		w.Units[oline[2]].BlockTarget = front[1]
		w.Units[oline[3]].BlockTarget = front[2]
	} else if play.ID == "inside_zone" && len(oline) >= 3 && len(front) >= 2 {
		mid := len(oline) / 2
		w.Units[oline[mid]].BlockTarget = front[len(front)/2]
		if mid > 0 {
			w.Units[oline[mid-1]].BlockTarget = front[len(front)/2]
		}
	}

	_ = los
}

// setSweepContain puts one unblocked defender in the right alley on every front.
// Run Fit is a loaded edge; Base/Blitz still set it; light boxes set it later/wider.
func setSweepContain(w *World, def playbook.DefenseCall, shell playbook.CoverageShell) {
	idx := -1
	if shell.ID == playbook.ShellCover2 {
		idx = pickCover2EdgeForce(w)
	}
	if idx < 0 && (shell.ID == playbook.ShellCover3 || shell.ID == playbook.ShellManFree) {
		idx = pickCover3EdgeForce(w)
	}
	if idx < 0 {
		idx = pickSweepContain(w, def, shell)
	}
	if idx < 0 {
		return
	}
	for i := range w.Units {
		if w.Units[i].BlockTarget == idx {
			w.Units[i].BlockTarget = -1
		}
		w.Units[i].ContainEdge = false
		w.Units[i].ContainForceX = 0
		w.Units[i].ContainSpd = 0
	}
	u := &w.Units[idx]
	u.ContainEdge = true
	u.Engaged = 0

	alley := 17.5
	step := 0.15
	spd := 1.02
	switch def.ID {
	case "run_fit":
		alley, step, spd = 12.0, 1.0, 1.14
	case "blitz":
		// Extra rusher, not a vacant alley — force the stretch inside.
		alley, step, spd = 14.0, -0.25, 1.10
	case "pass_rush", "soft_zone":
		// Light box, not a vacant alley: wider than Run Fit, still forces inside.
		alley, step, spd = 15.5, -0.2, 1.07
	}
	// Cover 2 squat corners must still force the stretch inside, even in a light box.
	if shell.ID == playbook.ShellCover2 {
		alley = 13.5
		step = -0.4 // step up toward the LOS and force inside
		if spd < 1.12 {
			spd = 1.12
		}
		if def.ID == "run_fit" {
			alley = 12.0
		}
	}
	// Cover 3 / Man Free on a light or empty box: force the edge.
	// Base + Cover 3 still gives the stretch a chance to bounce.
	light := def.ID == "pass_rush" || def.ID == "soft_zone" || def.ID == "blitz"
	if light && (shell.ID == playbook.ShellCover3 || shell.ID == playbook.ShellManFree) {
		alley = 13.0
		spd = 1.14
		// Hook depth is +10. Walk the edge down to the LOS — the +55 leak
		// was a contain standing in the hook while the stretch turned up.
		u.Pos.Y = w.Ball.Y + 2.2
		step = 0
	}
	if u.CoverJob.IsDeep() {
		step = -8.0
		if spd < 1.12 {
			spd = 1.12
		}
	}
	u.Pos.X = field.HashMid + alley
	if step != 0 {
		u.Pos.Y += step
	}
	u.Speed *= spd
	u.BaseSpeed = u.Speed
	u.ContainForceX = field.HashMid + alley
	u.ContainSpd = spd
	u.Pos = field.Clamp(u.Pos)
}

func pickCover2EdgeForce(w *World) int {
	// Playside (right) flat corner — already in the alley if C2 is aligned.
	best := -1
	bestX := field.HashMid
	for i, u := range w.Units {
		if u.Side != SideDefense || u.Role != RoleCB {
			continue
		}
		if u.CoverJob != CoverFlatR && u.CoverJob != CoverFlatL {
			continue
		}
		if u.Pos.X > bestX {
			bestX = u.Pos.X
			best = i
		}
	}
	return best
}

func pickCover3EdgeForce(w *World) int {
	// Playside flat / hook — already near the LOS. Never a deep-third CB.
	best, bestX := -1, field.HashMid
	for i, u := range w.Units {
		if u.Side != SideDefense || u.CoverJob.IsDeep() {
			continue
		}
		if u.CoverJob != CoverFlatR && u.CoverJob != CoverFlatL && u.CoverJob != CoverHook {
			continue
		}
		if u.Pos.X <= bestX {
			continue
		}
		bestX = u.Pos.X
		best = i
	}
	return best
}

func pickSweepContain(w *World, def playbook.DefenseCall, shell playbook.CoverageShell) int {
	// Prefer playside LB, then playside DE, then safety, then CB.
	// Light boxes used to prefer the playside CB — on Cover 3 that is a bailed third.
	prefer := []Role{RoleLB, RoleDL, RoleS, RoleCB}
	skipDeep := shell.ID == playbook.ShellCover3 || shell.ID == playbook.ShellManFree ||
		def.ID == "pass_rush" || def.ID == "soft_zone"
	for _, role := range prefer {
		best := -1
		bestX := field.HashMid
		for i, u := range w.Units {
			if u.Side != SideDefense || u.Role != role {
				continue
			}
			if skipDeep && u.CoverJob.IsDeep() {
				continue
			}
			if u.Pos.X <= bestX {
				continue
			}
			bestX = u.Pos.X
			best = i
		}
		if best >= 0 {
			return best
		}
	}
	return -1
}

func SweepContainIndex(w *World) int {
	if w == nil {
		return -1
	}
	for i, u := range w.Units {
		if u.ContainEdge {
			return i
		}
	}
	return -1
}

// freeMiddleLB leaves the Mike unblocked so Run Fit can stuff the A-gap.
func freeMiddleLB(w *World) {
	midLB := -1
	best := 1e9
	for i, u := range w.Units {
		if u.Side != SideDefense || u.Role != RoleLB {
			continue
		}
		d := math.Abs(u.Pos.X - field.HashMid)
		if d < best {
			best = d
			midLB = i
		}
	}
	if midLB < 0 {
		return
	}
	for i := range w.Units {
		if w.Units[i].BlockTarget == midLB {
			w.Units[i].BlockTarget = -1
		}
	}
	w.Units[midLB].Pos.X = field.HashMid
	w.Units[midLB].Pos.Y += 0.4
	w.Units[midLB].Engaged = 0
	w.Units[midLB].Speed *= 1.14
	w.Units[midLB].BaseSpeed = w.Units[midLB].Speed
}

// freeCover2Hole leaves the Cover 2 Mike unblocked so inside zone is a crease, not a house.
func freeCover2Hole(w *World) {
	idx := -1
	best := 1e9
	for i, u := range w.Units {
		if u.Side != SideDefense || u.Role != RoleLB || u.CoverJob != CoverHook {
			continue
		}
		d := math.Abs(u.Pos.X - field.HashMid)
		if d < best {
			best = d
			idx = i
		}
	}
	if idx < 0 {
		return
	}
	for i := range w.Units {
		if w.Units[i].BlockTarget == idx {
			w.Units[i].BlockTarget = -1
		}
	}
	w.Units[idx].Engaged = 0
}

func sortByX(w *World, idxs []int) {
	for i := 0; i < len(idxs); i++ {
		for j := i + 1; j < len(idxs); j++ {
			if w.Units[idxs[j]].Pos.X < w.Units[idxs[i]].Pos.X {
				idxs[i], idxs[j] = idxs[j], idxs[i]
			}
		}
	}
}

func pickPrimary(w *World, play playbook.Play) int {
	best := -1
	bestScore := -1e9
	mid := field.HashMid
	for i, u := range w.Units {
		if u.Side != SideOffense || (u.Role != RoleWR && u.Role != RoleTE) {
			continue
		}
		score := 0.0
		switch play.Side {
		case playbook.SideLeft:
			score = mid - u.Pos.X // more left = higher
		case playbook.SideRight:
			score = u.Pos.X - mid
		default:
			score = -math.Abs(u.Pos.X - mid)
		}
		if u.Role == RoleWR {
			score += 2
		}
		if score > bestScore {
			bestScore = score
			best = i
		}
	}
	return best
}

func conceptRoute(u *Unit, isPrimary bool, los float64, c playbook.Concept) (tx, ty float64) {
	tx, ty = u.Pos.X, los+c.PrimaryDepth
	if !isPrimary {
		// Clear-out vertical so the primary has air.
		clear := c.PrimaryDepth + 6
		if c.PrimaryBreak == "glance" {
			clear = 16
		}
		return u.Pos.X, los + clear
	}
	switch c.PrimaryBreak {
	case "post":
		return field.HashMid, los + c.PrimaryDepth
	case "glance":
		// Hook sit beside the A-gap — behind the crashing Mike, not through him.
		x := u.Pos.X
		if math.Abs(x-field.HashMid) < 4 {
			if x >= field.HashMid {
				x = field.HashMid + 6
			} else {
				x = field.HashMid - 6
			}
		}
		return x, los + c.PrimaryDepth
	default:
		return tx, ty
	}
}

func assignRoutes(w *World, play playbook.Play, los float64, primaryIdx int, c playbook.Concept, hasC bool) {
	for i := range w.Units {
		u := &w.Units[i]
		if u.Side != SideOffense {
			continue
		}
		isPrimary := i == primaryIdx
		switch u.Role {
		case RoleWR, RoleTE:
			tx, ty := u.Pos.X, los+8
			if play.Type == playbook.PlayPass {
				if hasC {
					tx, ty = conceptRoute(u, isPrimary, los, c)
				} else {
					switch play.ID {
					case "slant":
						// Primary: clear inside stem — 4 yards up, then cut hard to hash
						if isPrimary {
							if u.Pos.X < field.HashMid {
								tx = field.HashMid - 2
							} else {
								tx = field.HashMid + 2
							}
							ty = los + 7
						} else {
							// Clear-out go / opposite fade
							tx = u.Pos.X
							ty = los + 16
						}
					case "hitch":
						if isPrimary {
							tx = u.Pos.X
							ty = los + 5.5
						} else {
							tx = u.Pos.X
							ty = los + 12 // clear out
						}
					default:
						ty = los + 10
					}
				}
			} else if play.ID == "sweep" && u.Pos.X > field.HashMid {
				// Crack / seal down block path
				tx = u.Pos.X - 2
				ty = los + 2
			} else {
				// Run blocking: release downfield lightly or seal
				ty = los + 3
				if play.Side == playbook.SideRight {
					tx = u.Pos.X + 2
				} else if play.Side == playbook.SideLeft {
					tx = u.Pos.X - 2
				}
			}
			u.Target = field.Pos{X: tx, Y: ty}
			u.HasTarget = true
		case RoleRB:
			if play.Type == playbook.PlayRun {
				tx := field.HashMid
				ty := los + 15
				switch play.ID {
				case "sweep":
					// Stretch outside, then turn up inside the numbers (not the sideline)
					tx = field.HashMid + 14
					ty = los + 14
				case "inside_zone":
					tx = field.HashMid + (playSideJitter(play) * 2)
					ty = los + 12
				}
				u.Target = field.Pos{X: tx, Y: ty}
				u.HasTarget = true
			} else {
				if hasC && c.PlayAction {
					// Sell inside zone — dive, don't flare. Mesh retargets at snap.
					u.Target = field.Pos{X: field.HashMid, Y: los + 10}
				} else {
					u.Target = field.Pos{X: u.Pos.X + 4, Y: los + 4}
				}
				u.HasTarget = true
			}
		case RoleOL:
			// Drive block north a bit
			u.Target = field.Pos{X: u.Pos.X, Y: los + 2}
			u.HasTarget = true
		case RoleQB:
			if play.Type == playbook.PlayPass {
				drop := 2.5
				if hasC && c.DropYards > 0 {
					drop = c.DropYards
				} else if play.ID == "slant" || play.ID == "hitch" {
					drop = 1.5
				}
				u.Target = field.Pos{X: u.Pos.X, Y: los - drop}
				u.HasTarget = true
			}
		}
	}
}

func playSideJitter(play playbook.Play) float64 {
	switch play.Side {
	case playbook.SideLeft:
		return -1
	case playbook.SideRight:
		return 1
	default:
		return 0
	}
}

// Input from the player this frame.
type Input struct {
	DX, DY float64 // -1..1 desired move for controlled unit
	Throw  bool    // attempt pass to primary
	Juke   bool    // spin / juke (Shift)
}

// Tick advances the play by dt seconds. Returns true if still live.
func (ps *PlayState) Tick(dt float64, in Input) bool {
	if !ps.Alive {
		return false
	}
	ps.Elapsed += dt
	w := ps.World

	if ps.JukeCooldown > 0 {
		ps.JukeCooldown -= dt
		if ps.JukeCooldown < 0 {
			ps.JukeCooldown = 0
		}
	}
	if ps.HitchGather > 0 {
		ps.HitchGather -= dt
		if ps.HitchGather < 0 {
			ps.HitchGather = 0
		}
	}
	for i := range w.Units {
		if w.Units[i].JukeTimer > 0 {
			w.Units[i].JukeTimer -= dt
			if w.Units[i].JukeTimer < 0 {
				w.Units[i].JukeTimer = 0
			}
		}
	}

	// Handoff on runs
	if ps.Play.Type == playbook.PlayRun && !ps.HandoffDone && ps.Elapsed >= ps.HandoffAt {
		ps.doHandoff()
	}

	// Juke: lateral burst + brief tackle invuln
	if in.Juke && ps.ControlIdx >= 0 && ps.JukeCooldown <= 0 {
		ctrl := &w.Units[ps.ControlIdx]
		if ctrl.HasBall {
			ps.JukeCooldown = 1.35
			ctrl.JukeTimer = 0.32
			// Burst sideways relative to input, or default right
			jx := in.DX
			if math.Abs(jx) < 0.2 {
				if math.Abs(in.DY) < 0.2 {
					jx = 1
				} else {
					jx = 0
				}
			}
			jy := in.DY * 0.35
			mag := math.Hypot(jx, jy)
			if mag < 0.1 {
				jx, jy, mag = 1, 0, 1
			}
			burst := ctrl.BaseSpeed * 1.8
			ctrl.VX = jx / mag * burst
			ctrl.VY = jy / mag * burst
		}
	}

	// Auto-assist: RB drifts toward play side target if little input on runs after handoff
	if ps.Alive && ps.ControlIdx >= 0 {
		ctrl := &w.Units[ps.ControlIdx]
		if ctrl.HasBall {
			// Don't overwrite juke burst on the juke frame
			if ctrl.JukeTimer <= 0.28 || (ctrl.VX == 0 && ctrl.VY == 0) {
				speed := ctrl.Speed
				if ctrl.JukeTimer > 0 {
					speed *= 1.15
				}
				if ps.HitchGather > 0 && (ctrl.Role == RoleWR || ps.isGlance()) {
					speed *= 0.68 // plant and turn, not a free burst
				}
				ix, iy := in.DX, in.DY
				if mag := math.Hypot(ix, iy); mag > 1 {
					ix /= mag
					iy /= mag
				}
				if ps.Play.Type == playbook.PlayRun && math.Abs(iy) < 0.2 {
					iy = 0.55
				}
				// Early run: ease toward the designed hole if you're not steering hard
				if ps.Play.Type == playbook.PlayRun && ps.Elapsed < 1.4 && math.Abs(ix) < 0.45 {
					if ps.Play.ID == "inside_zone" {
						gapX := field.HashMid - ctrl.Pos.X
						ix += clamp(gapX*0.15, -0.35, 0.35)
					} else if ps.Play.ID == "sweep" {
						// Mild stretch assist — player still steers
						ix += 0.35
						if iy < 0.25 {
							iy = 0.35
						}
					}
				}
				// Only set from input if not mid-juke burst (first 0.12s pure burst)
				if ctrl.JukeTimer < 0.20 {
					ctrl.VX = ix * speed
					ctrl.VY = iy * speed
					if ps.Play.ID == "sweep" {
						// Help turn up before the sideline (not a free jet)
						sideline := field.WidthYards - ctrl.Pos.X
						if sideline > 12 {
							ctrl.VX += speed * 0.12
						} else if sideline > 6 {
							ctrl.VX *= 0.65
							ctrl.VY += speed * 0.25
						} else {
							ctrl.VX *= 0.25
							if ctrl.VY < speed*0.55 {
								ctrl.VY = speed * 0.55
							}
						}
					}
				}
			}
		}
	}

	// Play-action mesh first so Space buffers instead of killing the fake.
	ps.tickPlayAction(&in)

	// Throw request — once he crosses and it's a keep, the ball is live as a run.
	if in.Throw && ps.Play.Type == playbook.PlayPass && !ps.BallInAir &&
		ps.ControlIdx == ps.QBIdx && !ps.QBKeep {
		ps.tryThrow()
	}
	ps.declareKeep()

	// Move ball in air
	if ps.BallInAir {
		ps.ThrowTimer += dt
		ps.BallPos.X += ps.BallVelX * dt
		ps.BallPos.Y += ps.BallVelY * dt
		w.Ball = ps.BallPos
		// Catch / incomplete check near target
		ps.checkPassArrival(dt)
	}

	// AI for all non-controlled units
	ballCarrier := ps.ballCarrierIdx()
	for i := range w.Units {
		if i == ps.ControlIdx && !ps.BallInAir && (w.Units[i].HasBall || i == ps.QBIdx) {
			// Player-steered unit: velocity already set from input
			continue
		}
		ps.aiUnit(i, ballCarrier, dt)
	}

	// OL engagement after AI so blocks stick this frame
	ps.applyBlocks(dt)

	// Integrate positions
	for i := range w.Units {
		u := &w.Units[i]
		u.Pos.X += u.VX * dt
		u.Pos.Y += u.VY * dt
		if u.Pos.X < 0 {
			u.Pos.X = 0
		}
		if u.Pos.X > field.WidthYards {
			u.Pos.X = field.WidthYards
		}
	}

	// Ball follows carrier
	if !ps.BallInAir {
		if idx := ps.ballCarrierIdx(); idx >= 0 {
			w.Ball = w.Units[idx].Pos
		}
	}

	// Tackle / OOB / TD
	ps.checkDeadBall()
	if !ps.Alive {
		ps.sealResult()
	}
	return ps.Alive
}

// sealResult writes throw / keep / carrier onto the result so logs don't guess.
func (ps *PlayState) sealResult() {
	if ps.CarrierRole == "" && ps.QBIdx >= 0 && ps.World != nil &&
		ps.QBIdx < len(ps.World.Units) && ps.World.Units[ps.QBIdx].HasBall {
		ps.CarrierRole = RoleQB
	}
	// Pass call, never threw, QB still the carrier → it's a keep (sacks stay sacks).
	if ps.Play.Type == playbook.PlayPass && !ps.Thrown && ps.CarrierRole == RoleQB &&
		ps.Result.Outcome != OutcomeSack && ps.Result.Outcome != OutcomeIncomplete &&
		ps.Result.Outcome != OutcomeNone {
		ps.QBKeep = true
	}
	ps.Result.Thrown = ps.Thrown
	ps.Result.QBKeep = ps.QBKeep
	ps.Result.Carrier = ps.CarrierRole
	ps.fillPAResult(&ps.Result)
	if !ps.QBKeep {
		return
	}
	switch ps.Result.Outcome {
	case OutcomeTouchdown:
		ps.Result.Message = "QB keep — TOUCHDOWN!"
	case OutcomeOutOfBounds:
		ps.Result.Message = "QB keep — out of bounds"
	default:
		if ps.Result.Message == "Tackled" || ps.Result.Message == "Whistle (play clock)" || ps.Result.Message == "" {
			ps.Result.Message = "QB keep"
		}
	}
}

func (ps *PlayState) doHandoff() {
	ps.HandoffDone = true
	if ps.QBIdx < 0 || ps.RBIdx < 0 {
		return
	}
	ps.World.Units[ps.QBIdx].HasBall = false
	ps.World.Units[ps.RBIdx].HasBall = true
	ps.ControlIdx = ps.RBIdx
	// Toss: some outside momentum (not a turbo boost)
	if ps.Play.ID == "sweep" {
		rb := &ps.World.Units[ps.RBIdx]
		rb.VX = rb.BaseSpeed * 0.55
		rb.VY = rb.BaseSpeed * 0.28
	}
}

func (ps *PlayState) ballCarrierIdx() int {
	if ps.BallInAir {
		return -1
	}
	for i, u := range ps.World.Units {
		if u.HasBall {
			return i
		}
	}
	return -1
}

func (ps *PlayState) tryThrow() {
	if ps.PrimaryIdx < 0 || ps.QBIdx < 0 {
		return
	}
	// Releasing during the mesh is an aborted fake — no defensive bite.
	if ps.PlayAction && ps.Mesh == MeshLive {
		ps.abortMesh()
	}
	qb := ps.World.Units[ps.QBIdx]
	recv := ps.World.Units[ps.PrimaryIdx]

	// Lead the receiver toward their route target (arcade timing)
	aim := recv.Pos
	if recv.HasTarget {
		tdx := recv.Target.X - recv.Pos.X
		tdy := recv.Target.Y - recv.Pos.Y
		tdist := math.Hypot(tdx, tdy)
		if tdist > 0.3 {
			lead := 0.45 // seconds of lead
			if ps.Play.ID == "slant" {
				lead = 0.55
			}
			if ps.Play.DepthLabel == "intermediate" {
				lead = 0.7
			}
			if ps.HasConcept && ps.Concept.PrimaryBreak == "glance" {
				lead = 0.16 // sit throw — don't lead it into a 12-yard shot
			}
			aim.X = recv.Pos.X + (tdx/tdist)*recv.Speed*lead
			aim.Y = recv.Pos.Y + (tdy/tdist)*recv.Speed*lead
		}
	}
	dx := aim.X - qb.Pos.X
	dy := aim.Y - qb.Pos.Y
	dist := math.Hypot(dx, dy)
	if dist < 0.5 {
		dist = 0.5
	}
	// Short game: faster ball so slant window is hittable
	ballSpeed := 32.0
	if ps.Play.DepthLabel == "short" {
		ballSpeed = 38.0
	}
	ps.BallInAir = true
	ps.BallPos = qb.Pos
	ps.BallVelX = dx / dist * ballSpeed
	ps.BallVelY = dy / dist * ballSpeed
	ps.BallTarget = ps.PrimaryIdx
	ps.World.Units[ps.QBIdx].HasBall = false
	ps.ControlIdx = -1 // watching ball
	ps.ThrowTimer = 0
	ps.ThrowFlight = dist / ballSpeed
	ps.Thrown = true
	ps.markRelease()
}

func (ps *PlayState) checkPassArrival(dt float64) {
	if ps.BallTarget < 0 {
		ps.endIncomplete("Throw away / no target")
		return
	}
	recv := &ps.World.Units[ps.BallTarget]

	// Soft homing on short throws so timing is Tecmo-forgiving
	if ps.Play.DepthLabel == "short" {
		hx := recv.Pos.X - ps.BallPos.X
		hy := recv.Pos.Y - ps.BallPos.Y
		hd := math.Hypot(hx, hy)
		if hd > 0.2 && hd < 14 {
			if dt < 0 {
				dt = 0
			}
			// pull is tuned as "per 60 Hz tick"; scale by real dt so 30/120 FPS
			// match the same arcade curve instead of a hardcoded 1/60 step.
			pull := 28.0
			ps.BallVelX += (hx / hd) * pull * dt * 60
			ps.BallVelY += (hy / hd) * pull * dt * 60
			spd := math.Hypot(ps.BallVelX, ps.BallVelY)
			maxSpd := 42.0
			if spd > maxSpd {
				ps.BallVelX = ps.BallVelX / spd * maxSpd
				ps.BallVelY = ps.BallVelY / spd * maxSpd
			}
		}
	}

	dx := recv.Pos.X - ps.BallPos.X
	dy := recv.Pos.Y - ps.BallPos.Y
	dist := math.Hypot(dx, dy)

	// Contest: must be truly draped (tighter than before)
	contested := false
	contestR := 1.35
	for _, u := range ps.World.Units {
		if u.Side != SideDefense {
			continue
		}
		if math.Hypot(u.Pos.X-recv.Pos.X, u.Pos.Y-recv.Pos.Y) < contestR {
			contested = true
			break
		}
	}

	catchR := 2.1
	if ps.Play.ID == "slant" {
		catchR = 2.4
	}
	if ps.Play.DepthLabel == "intermediate" {
		catchR = 2.25
	}
	if dist < catchR {
		pCatch := 0.94
		if contested {
			pCatch = 0.58
		}
		switch ps.Shell.ID {
		case playbook.ShellCover2:
			if ps.Play.ID == "hitch" {
				pCatch -= 0.16 // squat corners contest the out
			}
			if ps.Play.ID == "slant" && !contested {
				pCatch += 0.04 // hole vs two-deep
			}
		case playbook.ShellCover3:
			if ps.Play.ID == "hitch" && !contested {
				pCatch += 0.03 // CBs bailed; hitch is the give
			}
		case playbook.ShellManFree:
			if contested {
				pCatch -= 0.08
			} else if ps.Play.ID == "slant" {
				pCatch -= 0.04 // tighter window if on time
			}
		}
		if ps.Def.ID == "soft_zone" && !contested {
			pCatch += 0.03
		}
		if pCatch < 0.35 {
			pCatch = 0.35
		}
		if pCatch > 0.98 {
			pCatch = 0.98
		}
		if ps.rng.Float64() < pCatch {
			recv.HasBall = true
			ps.BallInAir = false
			ps.ControlIdx = ps.BallTarget
			ps.BallTarget = -1
			ps.CaughtAt = ps.Elapsed
			if ps.Play.ID == "hitch" || ps.isGlance() {
				ps.HitchGather = 0.32
			}
		} else {
			ps.endIncomplete("Broken up / incomplete")
		}
		return
	}

	if ps.BallPos.Y > field.LengthYards+5 || ps.BallPos.Y < -5 ||
		ps.BallPos.X < -2 || ps.BallPos.X > field.WidthYards+2 {
		ps.endIncomplete("Incomplete (out of bounds)")
		return
	}
	// Hang-time budget is flight duration from release, not time since snap.
	if ps.ThrowFlight > 0 && ps.ThrowTimer > ps.ThrowFlight+0.85 && dist > 8 {
		ps.endIncomplete("Incomplete (overthrown)")
	}
}

func (ps *PlayState) endIncomplete(msg string) {
	ps.Alive = false
	ps.BallInAir = false
	ps.Result = Result{
		Outcome:     OutcomeIncomplete,
		YardsGained: 0,
		BallY:       ps.LOS,
		LOS:         ps.LOS,
		Message:     msg,
	}
}

func (ps *PlayState) aiUnit(i int, ballCarrier int, dt float64) {
	u := &ps.World.Units[i]
	// Clear vel; set anew
	u.VX, u.VY = 0, 0

	if u.Side == SideOffense {
		ps.aiOffense(i, dt)
		return
	}
	ps.aiDefense(i, ballCarrier, dt)
}

func (ps *PlayState) aiOffense(i int, dt float64) {
	u := &ps.World.Units[i]
	if u.HasBall {
		return // controlled or will be
	}

	// Run blockers: climb to assigned defender (OL / TE / crack WR)
	if ps.Play.Type == playbook.PlayRun && u.BlockTarget >= 0 &&
		(u.Role == RoleOL || u.Role == RoleTE || u.Role == RoleWR) {
		ti := u.BlockTarget
		if ti >= 0 && ti < len(ps.World.Units) {
			tpos := ps.World.Units[ti].Pos
			tpos.Y += 1.5
			if ps.Play.ID == "sweep" {
				tpos.X += 1.5
				tpos.Y += 0.5
			} else if ps.Play.ID == "inside_zone" {
				// Climb to the hole, not a lateral wash that fills it.
				if tpos.X >= field.HashMid {
					tpos.X += 0.6
				} else {
					tpos.X -= 0.6
				}
				tpos.Y += 0.8
			}
			spd := u.Speed * 1.05
			if u.Role == RoleWR {
				spd = u.Speed * 0.95
			}
			moveToward(u, tpos, spd)
			return
		}
	}

	// Move toward route target
	if u.HasTarget {
		spd := u.Speed * 0.85
		if ps.glanceStem(i) {
			spd = u.Speed * glanceStemMult
		}
		moveToward(u, u.Target, spd)
		// Hitch / glance: sit near the landmark
		if (ps.Play.ID == "hitch" && u.Role == RoleWR) || ps.glanceSit(i) {
			if math.Hypot(u.Pos.X-u.Target.X, u.Pos.Y-u.Target.Y) < 1.2 {
				u.VX, u.VY = 0, 0
			}
		}
	}
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// applyBlocks drives / seals defenders on run plays so the RB has a lane.
func (ps *PlayState) applyBlocks(dt float64) {
	// Decay engage state
	for i := range ps.World.Units {
		if ps.World.Units[i].Engaged > 0 {
			ps.World.Units[i].Engaged -= dt
			if ps.World.Units[i].Engaged < 0 {
				ps.World.Units[i].Engaged = 0
			}
		}
	}

	if ps.Play.Type != playbook.PlayRun {
		ps.applyPassPro(dt)
		return
	}

	// How hard the line pushes (defense call matters)
	push := 4.2  // yards/sec drive when locked
	hold := 0.10 // residual velocity factor for defender
	engageTime := 0.65
	switch ps.Def.ID {
	case "run_fit":
		push = 2.8
		hold = 0.25
		engageTime = 0.4
	case "soft_zone", "pass_rush":
		push = 5.2 // light box = better push
		hold = 0.06
		engageTime = 0.85
	case "blitz":
		push = 3.4
		hold = 0.18
	}
	if ps.Play.ID == "sweep" {
		if ps.Def.ID == "run_fit" {
			push = 1.6
			engageTime = 0.22
			hold = 0.4
		} else {
			push += 0.35
			engageTime += 0.05
		}
	}
	if ps.Play.ID == "inside_zone" {
		if ps.Def.ID == "run_fit" {
			push = 2.2
			engageTime = 0.32
			hold = 0.32
		} else {
			push += 1.15
			engageTime += 0.16
			hold *= 0.7
		}
	}
	push += ps.Line.runPushBonus(ps.Def.ID)

	for i := range ps.World.Units {
		u := &ps.World.Units[i]
		if u.Side != SideOffense {
			continue
		}
		if u.Role != RoleOL && u.Role != RoleTE && !(u.Role == RoleWR && u.BlockTarget >= 0) {
			continue
		}
		ti := u.BlockTarget
		if ti < 0 || ti >= len(ps.World.Units) {
			// Fallback: nearest DL (OL only)
			if u.Role != RoleOL && u.Role != RoleTE {
				continue
			}
			ti = ps.nearestDefender(u.Pos, RoleDL, RoleLB)
			if ti < 0 {
				continue
			}
		}
		d := &ps.World.Units[ti]
		if d.Side != SideDefense {
			continue
		}
		dist := math.Hypot(d.Pos.X-u.Pos.X, d.Pos.Y-u.Pos.Y)
		reach := 3.2
		if u.Role == RoleWR {
			reach = 2.8
		}
		if dist > reach {
			continue
		}

		// Locked up: kill pursuit, drive them in play direction
		d.Engaged = engageTime
		if ps.Play.ID == "sweep" {
			d.Engaged = engageTime + 0.2
		}
		d.VX *= hold
		d.VY *= hold

		// Drive vector: north + wash to open lane
		driveX := 0.0
		driveY := push
		if ps.Play.ID == "sweep" {
			// Kick out / seal to the right — open the edge
			driveX = push * 0.85
			driveY = push * 0.55
			if u.Role == RoleWR {
				driveX = push * 0.4
				driveY = push * 0.3 // crack down
			}
		} else if ps.Play.ID == "inside_zone" {
			// Vertical displacement first — open a crease, don't clog it.
			if d.Pos.X >= field.HashMid {
				driveX = push * 0.18
			} else {
				driveX = -push * 0.18
			}
			driveY = push * 1.15
		}
		// Double-team bonus: another blocker also close
		for j := range ps.World.Units {
			if j == i {
				continue
			}
			o := &ps.World.Units[j]
			if o.Side != SideOffense || (o.Role != RoleOL && o.Role != RoleTE) {
				continue
			}
			if math.Hypot(o.Pos.X-d.Pos.X, o.Pos.Y-d.Pos.Y) < 2.8 {
				driveY *= 1.25
				driveX *= 1.15
				d.VX *= 0.5
				d.VY *= 0.5
				break
			}
		}
		d.Pos.X += driveX * dt
		d.Pos.Y += driveY * dt
		// Stick OL on the block (ride the defender)
		if dist > 0.9 {
			u.Pos.X += (d.Pos.X - u.Pos.X) * 0.35
			u.Pos.Y += (d.Pos.Y - u.Pos.Y) * 0.35
		}
	}
}

func (ps *PlayState) aiDefense(i int, ballCarrier int, dt float64) {
	u := &ps.World.Units[i]
	// Pass rush: DL always attack QB until throw
	qbPos := field.Pos{}
	if ps.QBIdx >= 0 {
		qbPos = ps.World.Units[ps.QBIdx].Pos
	}

	if ps.Play.Type == playbook.PlayPass && !ps.BallInAir && ps.ballCarrierIdx() == ps.QBIdx && !ps.QBKeep {
		if ps.sellPlayAction(u, qbPos) {
			return
		}
		// Coverage vs rush
		switch u.Role {
		case RoleDL:
			spd := u.Speed
			delay := ps.paRushDelay(ps.Line.rushDelay(ps.Def.ID))
			if ps.Elapsed < delay {
				spd *= 0.22
			}
			if claimedRusher(ps.World, i) && u.Engaged > 0 {
				spd *= 0.15
			}
			if ps.Elapsed > 0.95 && u.Engaged <= 0 {
				spd *= 1.08
			}
			// Interior rushers squeeze the A-gap; they do not loop and vacate it.
			aim := qbPos
			if math.Abs(u.Pos.X-field.HashMid) < 7.5 {
				aim.X = field.HashMid*0.55 + qbPos.X*0.45
			}
			moveToward(u, aim, spd)
		case RoleLB:
			if u.Spy {
				sit := field.Pos{X: field.HashMid, Y: ps.LOS + 4}
				if ps.Line.KeepThreat >= 2 {
					sit.Y = ps.LOS + 2.5
				}
				// Shadow the QB's hash; don't chase him into the backfield.
				sit.X = qbPos.X*0.4 + field.HashMid*0.6
				moveToward(u, sit, u.Speed*0.9)
				return
			}
			if u.RushFree || (ps.Def.ID == "blitz" && u.RushFree) {
				moveToward(u, qbPos, u.Speed*1.05)
			} else if ps.Def.ID == "pass_rush" && ps.Elapsed > 0.55 {
				// Delayed scrape, not a free extra rusher on every snap.
				moveToward(u, qbPos, u.Speed*0.85)
			} else if dest, spd, ok := ps.coverTarget(i); ok {
				moveToward(u, dest, spd)
			} else if ps.PrimaryIdx >= 0 {
				t := ps.World.Units[ps.PrimaryIdx].Pos
				t.Y = (t.Y + ps.LOS + 5) * 0.5
				moveToward(u, t, u.Speed*0.8)
			}
		case RoleCB, RoleS:
			if dest, spd, ok := ps.coverTarget(i); ok {
				closeR := 1.15
				if u.CoverJob.IsFlat() && ps.Play.ID == "hitch" {
					closeR = 0.7 // Cover 2 squat: hip pocket
				}
				if u.CoverJob.IsDeep() {
					closeR = 1.6 // don't collapse onto a stop route
				}
				if math.Hypot(dest.X-u.Pos.X, dest.Y-u.Pos.Y) < closeR {
					u.VX, u.VY = 0, 0
					return
				}
				moveToward(u, dest, spd)
			}
		}
		return
	}

	// Ball in air: play the assignment, not "everyone chase the ball."
	if ps.BallInAir && (u.Role == RoleCB || u.Role == RoleS || u.Role == RoleLB) {
		if u.CoverJob.IsDeep() {
			// Stay over the top of the throw.
			sit := field.Pos{X: u.CoverLand.X, Y: math.Max(u.CoverLand.Y, ps.BallPos.Y+2)}
			if ps.PrimaryIdx >= 0 {
				pr := ps.World.Units[ps.PrimaryIdx].Pos
				sit.X = sit.X*0.45 + pr.X*0.55
			}
			moveToward(u, sit, u.Speed)
			return
		}
		if u.CoverJob.IsFlat() || u.CoverJob == CoverMan {
			moveToward(u, ps.BallPos, u.Speed*1.05)
			return
		}
		// Hook/robber: drive the catch point, don't fly past it.
		sit := ps.BallPos
		if sit.Y < ps.LOS+5 {
			sit.Y = ps.LOS + 5
		}
		moveToward(u, sit, u.Speed)
		return
	}

	// Pursue ball carrier
	if ballCarrier >= 0 {
		bp := ps.World.Units[ballCarrier].Pos
		spd := u.Speed

		// Engaged on a block: heavily slowed / sealed
		if u.Engaged > 0 && !u.ContainEdge {
			spd *= 0.2
			// Try to shed slowly toward ball but stay washed
			moveToward(u, bp, spd)
			return
		}

		if ps.QBKeep && u.Spy {
			moveToward(u, bp, spd*1.38)
			return
		}
		if ps.QBKeep && (u.HoleFill || (u.Role == RoleLB && (u.CoverJob == CoverHook || math.Abs(u.Pos.X-field.HashMid) < 5))) {
			burst := 1.28
			if u.HoleFill {
				burst = 1.42
			}
			moveToward(u, bp, spd*burst)
			return
		}

		// Sweep contain first — chase the stored alley, not a second table.
		// Speed was already applied on the unit in setSweepContain.
		if ps.Play.ID == "sweep" && u.ContainEdge {
			forceX := u.ContainForceX
			if forceX == 0 {
				forceX = field.HashMid + 14.2
			}
			if ps.Def.ID == "run_fit" && bp.X > forceX {
				forceX = bp.X + 1.6
			}
			// Cover 3 / light: squeeze if they bounce outside the stored alley.
			light := ps.Def.ID == "pass_rush" || ps.Def.ID == "soft_zone" || ps.Def.ID == "blitz"
			if light && (ps.Shell.ID == playbook.ShellCover3 || ps.Shell.ID == playbook.ShellManFree) && bp.X > forceX {
				forceX = bp.X + 1.3
			}
			// Set the edge at the LOS — do not chase into the backfield and stuff every stretch.
			ty := bp.Y + 0.2
			if ty < ps.LOS+0.6 {
				ty = ps.LOS + 0.6
			}
			moveToward(u, field.Pos{X: forceX, Y: ty}, spd)
			return
		}

		// Run fit: LBs/DL crash harder once free
		if ps.Def.ID == "run_fit" && (u.Role == RoleLB || u.Role == RoleDL) {
			spd *= 1.12
		}
		// Hitch YAC: zone shells wrap; man-free only if you're the beaten man.
		if ps.Play.ID == "hitch" && ps.CaughtAt > 0 {
			switch ps.Shell.ID {
			case playbook.ShellManFree:
				if u.CoverJob == CoverMan && ballCarrier == u.CoverMan {
					spd *= 1.08
				}
			default:
				if u.Role == RoleCB || u.Role == RoleS || u.Role == RoleLB {
					spd *= 1.18
				}
			}
		}
		if wr := ps.glanceWrapSpeed(u); wr != 1 {
			spd *= wr
		}
		// Slant YAC: man and hook wrap; deep thirds stay home.
		if ps.Play.ID == "slant" && ps.CaughtAt > 0 && !u.CoverJob.IsDeep() {
			if u.CoverJob == CoverMan || u.CoverJob == CoverHook || u.CoverJob.IsFlat() {
				spd *= 1.16
			}
		}
		// Post YAC: deep halves / hook close after the catch (12–16, not 20).
		if ps.isPostShot() && ps.CaughtAt > 0 {
			if u.CoverJob.IsDeep() {
				spd *= 1.38
			}
			if u.CoverJob == CoverHook {
				spd *= 1.18
			}
		}
		// Inside zone: second level reads so the crease exists vs base.
		// Cover 2 is the exception — the Mike fills or the A-gap is a 60-yard alley.
		c2hole := ps.Play.ID == "inside_zone" && ps.Shell.ID == playbook.ShellCover2
		if ps.Play.ID == "inside_zone" && ps.Def.ID != "run_fit" && ps.Def.ID != "blitz" {
			if u.Role == RoleLB && ps.Elapsed < 0.85 {
				if c2hole && u.CoverJob == CoverHook && math.Abs(u.Pos.X-field.HashMid) < 5 {
					if ps.Elapsed < 0.95 {
						spd *= 0.22 // crease exists
					} else {
						spd *= 1.12 // then the alley closes
					}
				} else if !(c2hole && u.CoverJob == CoverHook) {
					spd *= 0.42
				}
			}
			if u.Role == RoleS && ps.Elapsed < 1.0 {
				if c2hole {
					spd *= 0.88 // rotate down; do not vanish
				} else {
					spd *= 0.55
				}
			}
		}
		// Soft zone / pass rush: second-level slower to scrape on early run
		if ps.Play.Type == playbook.PlayRun && ps.Elapsed < 1.1 {
			if ps.Def.ID == "soft_zone" || ps.Def.ID == "pass_rush" {
				if u.Role == RoleLB && !(c2hole && u.CoverJob == CoverHook) {
					spd *= 0.75
				}
				if u.Role == RoleS && !c2hole {
					spd *= 0.65 // stay deep a beat
				}
			}
		}
		// Early down: LBs read a half-second (unless blitz/run_fit)
		if ps.Play.Type == playbook.PlayRun && ps.Elapsed < 0.45 && u.Role == RoleLB {
			if ps.Def.ID != "run_fit" && ps.Def.ID != "blitz" {
				spd *= 0.55
			}
		}
		if ps.Play.ID == "sweep" {
			if u.Role == RoleS && !u.CoverJob.IsDeep() {
				bp.X += 2
				if ps.Elapsed < 0.7 {
					spd *= 0.5 // read the stretch; don't crash the LOS on every snap
				} else {
					spd *= 1.05
				}
			}
			if u.Role == RoleCB && u.Pos.X > field.HashMid && !u.ContainEdge {
				spd *= 1.02
			}
			if u.Role == RoleLB && u.Pos.X < field.HashMid {
				spd *= 0.85
			}
		}
		// DL without engagement still crash but not warp-speed through gaps
		if u.Role == RoleDL && ps.Play.Type == playbook.PlayRun {
			spd *= 0.85
		}
		moveToward(u, bp, spd)
		return
	}
}

func (ps *PlayState) nearestDefender(from field.Pos, roles ...Role) int {
	best := -1
	bestD := 1e9
	roleOK := func(r Role) bool {
		if len(roles) == 0 {
			return true
		}
		for _, x := range roles {
			if x == r {
				return true
			}
		}
		return false
	}
	for i, u := range ps.World.Units {
		if u.Side != SideDefense || !roleOK(u.Role) {
			continue
		}
		d := math.Hypot(u.Pos.X-from.X, u.Pos.Y-from.Y)
		if d < bestD {
			bestD = d
			best = i
		}
	}
	return best
}

func (ps *PlayState) nearestOffense(from field.Pos, roles ...Role) int {
	best := -1
	bestD := 1e9
	roleOK := func(r Role) bool {
		if len(roles) == 0 {
			return true
		}
		for _, x := range roles {
			if x == r {
				return true
			}
		}
		return false
	}
	for i, u := range ps.World.Units {
		if u.Side != SideOffense || !roleOK(u.Role) {
			continue
		}
		d := math.Hypot(u.Pos.X-from.X, u.Pos.Y-from.Y)
		if d < bestD {
			bestD = d
			best = i
		}
	}
	return best
}

func moveToward(u *Unit, target field.Pos, speed float64) {
	dx := target.X - u.Pos.X
	dy := target.Y - u.Pos.Y
	dist := math.Hypot(dx, dy)
	if dist < 0.15 {
		u.VX, u.VY = 0, 0
		return
	}
	u.VX = dx / dist * speed
	u.VY = dy / dist * speed
}

func (ps *PlayState) checkDeadBall() {
	// Sack: defender hits QB with ball on pass before throw
	if ps.Play.Type == playbook.PlayPass && !ps.BallInAir && !ps.QBKeep {
		if ps.QBIdx >= 0 && ps.World.Units[ps.QBIdx].HasBall &&
			ps.World.Units[ps.QBIdx].Pos.Y < ps.LOS+0.4 {
			qb := ps.World.Units[ps.QBIdx]
			for _, u := range ps.World.Units {
				if u.Side != SideDefense {
					continue
				}
				if qb.JukeTimer > 0 {
					continue
				}
				// Slightly tighter sack radius; quick game gets the ball out
				sackR := 1.25
				if ps.Play.DepthLabel == "short" && ps.Elapsed < 0.75 {
					sackR = 1.08
				}
				if ps.Elapsed > 0.95 {
					sackR = 1.45
				}
				if ps.Def.ID == "blitz" || ps.Def.ID == "pass_rush" {
					sackR += 0.12
				}
				sackR += ps.paSackBonus()
				if math.Hypot(u.Pos.X-qb.Pos.X, u.Pos.Y-qb.Pos.Y) < sackR {
					ps.Alive = false
					ballY := qb.Pos.Y
					gained := ballY - ps.LOS
					ps.CarrierRole = RoleQB
					ps.Result = Result{
						Outcome:     OutcomeSack,
						YardsGained: gained,
						BallY:       ballY,
						LOS:         ps.LOS,
						Message:     "SACK!",
					}
					return
				}
			}
		}
	}

	idx := ps.ballCarrierIdx()
	if idx < 0 {
		return
	}
	bc := ps.World.Units[idx]

	ps.CarrierRole = bc.Role

	// Touchdown
	if bc.Pos.Y >= field.LengthYards {
		ps.Alive = false
		ps.Result = Result{
			Outcome:     OutcomeTouchdown,
			YardsGained: field.LengthYards - ps.LOS,
			BallY:       field.LengthYards,
			LOS:         ps.LOS,
			Message:     "TOUCHDOWN!",
		}
		return
	}

	// Out of bounds
	if bc.Pos.X <= 0.15 || bc.Pos.X >= field.WidthYards-0.15 {
		ps.Alive = false
		ballY := bc.Pos.Y
		if ballY < 0 {
			ballY = 0
		}
		if ballY > field.LengthYards {
			ballY = field.LengthYards
		}
		ps.Result = Result{
			Outcome:     OutcomeOutOfBounds,
			YardsGained: ballY - ps.LOS,
			BallY:       ballY,
			LOS:         ps.LOS,
			Message:     "Out of bounds",
		}
		return
	}

	// Juke: temporary evade
	if bc.JukeTimer > 0 {
		// still can score / OOB, but not tackled this window
	} else {
		// Tackle radius — engaged defenders and early-stretch grace for sweeps
		tackleR := 1.35
		if ps.Play.ID == "sweep" && ps.Elapsed < 0.9 {
			tackleR = 1.15
		}
		if ps.Play.ID == "inside_zone" && ps.Elapsed < 0.9 && ps.Def.ID != "run_fit" {
			if ps.Shell.ID == playbook.ShellCover2 {
				tackleR = 1.18
			} else {
				tackleR = 1.12
			}
		}
		if ps.Play.ID == "hitch" && ps.CaughtAt > 0 && ps.Elapsed-ps.CaughtAt < 1.35 {
			switch ps.Shell.ID {
			case playbook.ShellManFree:
				tackleR = 1.38 // beat man → YAC
			case playbook.ShellCover2:
				tackleR = 1.78 // squat + wrap
			default:
				tackleR = 1.68 // C3: SS/LB wrap the alley
			}
		}
		if ps.Play.ID == "slant" && ps.CaughtAt > 0 && ps.Elapsed-ps.CaughtAt < 1.2 {
			switch ps.Shell.ID {
			case playbook.ShellCover2:
				tackleR = 1.42 // hole vs two-deep — still the give
			case playbook.ShellManFree:
				tackleR = 1.62 // beaten man + hook wrap
			default:
				tackleR = 1.55
			}
		}
		if wr := ps.glanceWrapRadius(); wr > 0 {
			if wr > tackleR {
				tackleR = wr
			}
		}
		if ps.isPostShot() && ps.CaughtAt > 0 && ps.Elapsed-ps.CaughtAt < 1.4 {
			switch ps.Shell.ID {
			case playbook.ShellCover2:
				tackleR = 1.82 // deep halves must finish
			case playbook.ShellManFree:
				tackleR = 1.58
			default:
				tackleR = 1.62
			}
		}
		for _, u := range ps.World.Units {
			if u.Side != SideDefense {
				continue
			}
			// Engaged defenders shed into tackles as the timer runs out
			if u.Engaged > 0.2 && !u.Spy && !u.HoleFill {
				continue
			}
			r := tackleR
			if u.Engaged > 0 && !u.Spy {
				r *= 0.85 // partial shed
			}
			if ps.QBKeep {
				if u.Spy || u.HoleFill {
					r = 1.74
				} else if u.Role == RoleLB || u.Role == RoleDL {
					if r < 1.50 {
						r = 1.50
					}
				}
				if (ps.Shell.ID == playbook.ShellCover2 || ps.Def.ID == "base") &&
					(u.Role == RoleLB || u.CoverJob == CoverHook) {
					if r < 1.64 {
						r = 1.64
					}
				}
			}
			// Backside defenders need to close fully
			if ps.Play.ID == "sweep" && u.Pos.X < field.HashMid {
				r = 1.2
			}
			if ps.Play.ID == "sweep" && u.ContainEdge {
				r = 1.45
				if ps.Def.ID == "run_fit" {
					r = 1.62
				}
				if ps.Def.ID == "blitz" {
					r = 1.55
				}
				if ps.Def.ID == "pass_rush" || ps.Def.ID == "soft_zone" {
					r = 1.58
				}
				if ps.Shell.ID == playbook.ShellCover2 {
					r = 1.58
				}
				if (ps.Def.ID == "pass_rush" || ps.Def.ID == "soft_zone") &&
					ps.Shell.ID == playbook.ShellCover3 {
					r = 1.64
				}
			}
			if ps.isPostShot() && ps.CaughtAt > 0 && (u.CoverJob.IsDeep() || u.CoverJob == CoverHook) {
				if r < 1.82 {
					r = 1.82
				}
			}
			if math.Hypot(u.Pos.X-bc.Pos.X, u.Pos.Y-bc.Pos.Y) < r {
				ps.Alive = false
				ballY := bc.Pos.Y
				gained := ballY - ps.LOS
				ps.CarrierRole = bc.Role
				ps.Result = Result{
					Outcome:     OutcomeTackle,
					YardsGained: gained,
					BallY:       ballY,
					LOS:         ps.LOS,
					Message:     "Tackled",
				}
				return
			}
		}
	}

	// Safety valve: play too long
	if ps.Elapsed > 18 {
		ps.Alive = false
		ballY := bc.Pos.Y
		ps.Result = Result{
			Outcome:     OutcomeTackle,
			YardsGained: ballY - ps.LOS,
			BallY:       ballY,
			LOS:         ps.LOS,
			Message:     "Whistle (play clock)",
		}
	}
}
