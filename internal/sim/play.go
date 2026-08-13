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
}

// PlayState is one live snap.
type PlayState struct {
	World *World
	Play  playbook.Play
	Def   playbook.DefenseCall
	LOS   float64

	// Player-controlled unit index into World.Units (-1 if none).
	ControlIdx int
	// Primary receiver index for pass plays.
	PrimaryIdx int
	// QB index.
	QBIdx int
	// RB index.
	RBIdx int

	// Ball in flight (pass).
	BallInAir   bool
	BallPos     field.Pos
	BallVelX    float64
	BallVelY    float64
	BallTarget  int // unit index expected to catch; -1 none
	ThrowTimer  float64
	HandoffDone bool
	HandoffAt   float64 // seconds after snap

	// Juke cooldown remaining (seconds).
	JukeCooldown float64

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
// fat applies drive fatigue to offense skill speeds.
func StartPlay(los float64, play playbook.Play, def playbook.DefenseCall, rng *rand.Rand, fat Fatigue) *PlayState {
	if rng == nil {
		rng = rand.New(rand.NewSource(1))
	}
	w := PlacePreSnap(los)
	ps := &PlayState{
		World:      w,
		Play:       play,
		Def:        def,
		LOS:        los,
		ControlIdx: -1,
		PrimaryIdx: -1,
		QBIdx:      -1,
		RBIdx:      -1,
		BallTarget: -1,
		Alive:      true,
		rng:        rng,
		HandoffAt:  0.30,
	}
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
		// Run Fit: stack the edge — no free outside
		edge := 1.0
		if def.ID == "run_fit" {
			edge = -0.5 // actually pinching
		} else if def.ID == "soft_zone" || def.ID == "pass_rush" {
			edge = 1.8 // still punish light boxes
		}
		for i := range w.Units {
			u := &w.Units[i]
			if u.Side != SideDefense || u.Pos.X <= field.HashMid {
				continue
			}
			if u.Role == RoleDL || u.Role == RoleLB || u.Role == RoleCB {
				u.Pos.X += edge
				if def.ID == "run_fit" && (u.Role == RoleLB || u.Role == RoleCB) {
					u.Pos.Y -= 0.5 // step up
					u.Speed *= 1.08
					u.BaseSpeed = u.Speed
				}
				u.Pos = field.Clamp(u.Pos)
			}
		}
	}

	// Assign primary receiver: first WR matching play side preference.
	ps.PrimaryIdx = pickPrimary(w, play)

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
	assignRoutes(w, play, los, ps.PrimaryIdx)
	if play.Type == playbook.PlayRun {
		assignRunBlocks(w, play, los)
		// Run Fit vs stretch: keep a free playside LB in the alley (classic "load the edge")
		if play.ID == "sweep" && def.ID == "run_fit" {
			freePlaysideLB(w)
		}
		// Snap blockers partway toward their man (less teleport against loaded boxes)
		snapClose := 0.35
		eng := 0.35
		if play.ID == "sweep" && def.ID == "run_fit" {
			snapClose = 0.15
			eng = 0.12
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

	// Inside zone: center + guards double A-gap DTs (first two DL)
	if play.ID == "inside_zone" && len(oline) >= 3 && len(front) >= 2 {
		// Center (middle) and right guard both on first DT; left guard on second — crude doubles
		mid := len(oline) / 2
		w.Units[oline[mid]].BlockTarget = front[len(front)/2]
		if mid > 0 {
			w.Units[oline[mid-1]].BlockTarget = front[len(front)/2]
		}
	}

	_ = los
}

// freePlaysideLB clears blocks on the rightmost LB so Run Fit has a free hitter on sweeps.
func freePlaysideLB(w *World) {
	rightLB := -1
	bestX := -1.0
	for i, u := range w.Units {
		if u.Side == SideDefense && u.Role == RoleLB && u.Pos.X > bestX {
			bestX = u.Pos.X
			rightLB = i
		}
	}
	if rightLB < 0 {
		return
	}
	for i := range w.Units {
		if w.Units[i].BlockTarget == rightLB {
			w.Units[i].BlockTarget = -1
		}
	}
	// Align free hitter in the alley
	w.Units[rightLB].Pos.X = field.HashMid + 12
	w.Units[rightLB].Pos.Y += 1
	w.Units[rightLB].Engaged = 0
	w.Units[rightLB].Speed *= 1.12
	w.Units[rightLB].BaseSpeed = w.Units[rightLB].Speed
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

func assignRoutes(w *World, play playbook.Play, los float64, primaryIdx int) {
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
				// Check release
				u.Target = field.Pos{X: u.Pos.X + 4, Y: los + 4}
				u.HasTarget = true
			}
		case RoleOL:
			// Drive block north a bit
			u.Target = field.Pos{X: u.Pos.X, Y: los + 2}
			u.HasTarget = true
		case RoleQB:
			if play.Type == playbook.PlayPass {
				// Quick game: shallow drop (slant/hitch), not a 7-step
				drop := 2.5
				if play.ID == "slant" || play.ID == "hitch" {
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

	// Throw request
	if in.Throw && ps.Play.Type == playbook.PlayPass && !ps.BallInAir && ps.ControlIdx == ps.QBIdx {
		ps.tryThrow()
	}

	// Move ball in air
	if ps.BallInAir {
		ps.BallPos.X += ps.BallVelX * dt
		ps.BallPos.Y += ps.BallVelY * dt
		w.Ball = ps.BallPos
		// Catch / incomplete check near target
		ps.checkPassArrival()
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
	return ps.Alive
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
	ps.ThrowTimer = dist / ballSpeed
}

func (ps *PlayState) checkPassArrival() {
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
			const dt = 1.0 / 60.0
			pull := 28.0 // yards/sec² toward receiver
			ps.BallVelX += (hx / hd) * pull * dt * 60 // per-tick accel scaled
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
	if dist < catchR {
		pCatch := 0.94
		if contested {
			pCatch = 0.58
		}
		// Soft zone gives underneath completions
		if ps.Def.ID == "soft_zone" && !contested {
			pCatch = 0.98
		}
		if ps.Def.ID == "soft_zone" && contested {
			pCatch = 0.7
		}
		if ps.rng.Float64() < pCatch {
			recv.HasBall = true
			ps.BallInAir = false
			ps.ControlIdx = ps.BallTarget
			ps.BallTarget = -1
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
	if ps.ThrowTimer > 0 && ps.Elapsed > ps.ThrowTimer+0.85 && dist > 8 {
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
				if tpos.X >= field.HashMid {
					tpos.X += 1.5
				} else {
					tpos.X -= 1.5
				}
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
		moveToward(u, u.Target, u.Speed*0.85)
		// Hitch: stop near target
		if ps.Play.ID == "hitch" && u.Role == RoleWR {
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
		// Pass pro: light chip only
		for i := range ps.World.Units {
			u := &ps.World.Units[i]
			if u.Side != SideOffense || u.Role != RoleOL {
				continue
			}
			nearest := ps.nearestDefender(u.Pos, RoleDL)
			if nearest < 0 {
				continue
			}
			d := &ps.World.Units[nearest]
			if math.Hypot(d.Pos.X-u.Pos.X, d.Pos.Y-u.Pos.Y) < 2.2 {
				d.VX *= 0.4
				d.VY *= 0.4
				d.Engaged = 0.15
			}
		}
		return
	}

	// How hard the line pushes (defense call matters)
	push := 4.2 // yards/sec drive when locked
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
			if d.Pos.X >= field.HashMid {
				driveX = push * 0.45
			} else {
				driveX = -push * 0.45
			}
			driveY = push * 0.9
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

	if ps.Play.Type == playbook.PlayPass && !ps.BallInAir && ps.ballCarrierIdx() == ps.QBIdx {
		// Coverage vs rush
		switch u.Role {
		case RoleDL:
			// Slightly delayed get-off so quick game can beat pure pressure
			spd := u.Speed
			if ps.Elapsed < 0.35 && (ps.Play.ID == "slant" || ps.Play.ID == "hitch") {
				spd *= 0.7
			}
			moveToward(u, qbPos, spd)
		case RoleLB:
			if ps.Def.ID == "blitz" || ps.Def.ID == "pass_rush" {
				moveToward(u, qbPos, u.Speed*0.95)
			} else if ps.PrimaryIdx >= 0 {
				// hook zone toward primary
				t := ps.World.Units[ps.PrimaryIdx].Pos
				t.Y = (t.Y + ps.LOS + 5) * 0.5
				moveToward(u, t, u.Speed*0.8)
			}
		case RoleCB, RoleS:
			// Cover nearest WR in their half; CBs give a cushion early on short routes
			target := ps.nearestOffense(u.Pos, RoleWR, RoleTE)
			// Prefer matching primary if this CB is on that side
			if ps.PrimaryIdx >= 0 {
				pr := ps.World.Units[ps.PrimaryIdx]
				if (u.Pos.X < field.HashMid) == (pr.Pos.X < field.HashMid) || u.Role == RoleS {
					target = ps.PrimaryIdx
				}
			}
			if target >= 0 {
				rp := ps.World.Units[target].Pos
				spd := u.Speed * 0.88
				if ps.Def.ID == "soft_zone" {
					rp.Y += 4
					spd *= 0.9
				} else if ps.Play.DepthLabel == "short" && ps.Elapsed < 0.7 {
					// Off coverage for a beat — slant/hitch window
					rp.Y += 2.0
					spd *= 0.75
				}
				// Don't teleport onto the receiver
				if math.Hypot(rp.X-u.Pos.X, rp.Y-u.Pos.Y) < 1.2 {
					u.VX, u.VY = 0, 0
					return
				}
				moveToward(u, rp, spd)
			}
		}
		return
	}

	// Ball in air: DBs play ball / receiver
	if ps.BallInAir && (u.Role == RoleCB || u.Role == RoleS || u.Role == RoleLB) {
		moveToward(u, ps.BallPos, u.Speed)
		return
	}

	// Pursue ball carrier
	if ballCarrier >= 0 {
		bp := ps.World.Units[ballCarrier].Pos
		spd := u.Speed

		// Engaged on a block: heavily slowed / sealed
		if u.Engaged > 0 {
			spd *= 0.2
			// Try to shed slowly toward ball but stay washed
			moveToward(u, bp, spd)
			return
		}

		// Run fit: LBs/DL crash harder once free
		if ps.Def.ID == "run_fit" && (u.Role == RoleLB || u.Role == RoleDL) {
			spd *= 1.12
		}
		// Soft zone / pass rush: second-level slower to scrape on early run
		if ps.Play.Type == playbook.PlayRun && ps.Elapsed < 1.1 {
			if ps.Def.ID == "soft_zone" || ps.Def.ID == "pass_rush" {
				if u.Role == RoleLB {
					spd *= 0.75
				}
				if u.Role == RoleS {
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
		// Sweep contain: Run Fit crashes the edge; other calls flow more
		if ps.Play.ID == "sweep" {
			if u.Role == RoleS {
				if ps.Def.ID == "run_fit" {
					bp.X += 2
					spd *= 1.1 // force
				} else {
					bp.X += 3
				}
			}
			if u.Role == RoleCB && u.Pos.X > field.HashMid {
				if ps.Def.ID == "run_fit" {
					spd *= 1.05 // support edge
				} else if ps.Elapsed < 0.55 {
					spd *= 0.8
					bp.X += 1.5
				}
			}
			if u.Role == RoleLB && u.Pos.X > field.HashMid {
				if ps.Def.ID == "run_fit" {
					spd *= 1.15 // scrape hard
				}
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
	if ps.Play.Type == playbook.PlayPass && !ps.BallInAir {
		if ps.QBIdx >= 0 && ps.World.Units[ps.QBIdx].HasBall {
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
				if ps.Play.DepthLabel == "short" && ps.Elapsed < 0.9 {
					sackR = 1.1
				}
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
		for _, u := range ps.World.Units {
			if u.Side != SideDefense {
				continue
			}
			// Engaged defenders shed into tackles as the timer runs out
			if u.Engaged > 0.2 {
				continue
			}
			r := tackleR
			if u.Engaged > 0 {
				r *= 0.85 // partial shed
			}
			// Backside defenders need to close fully
			if ps.Play.ID == "sweep" && u.Pos.X < field.HashMid {
				r = 1.2
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
