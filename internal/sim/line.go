package sim

import (
	"math"

	"github.com/randerson1184/tecmo-sooper-bowl/internal/field"
	"github.com/randerson1184/tecmo-sooper-bowl/internal/playbook"
)

// LineContext is the defense-first picture of the LOS this snap.
// Threats are effectiveness (success / explosives), not call frequency.
type LineContext struct {
	ObviousPass bool    // 3rd or 4th & 7+
	RunThreat   float64 // recent successful/explosive runs (decayed, capped)
	PassThreat  float64 // recent successful/explosive passes
	RunPct      float64
	PassPct     float64
	KeepThreat  float64 // QB keep / scramble success (decayed)
	Samples     int
}

// PassHappy is extreme pass frequency plus a working pass game — not "more pass than run."
func (c LineContext) PassHappy() bool {
	return c.Samples >= 5 && c.PassPct >= 0.78 && c.PassThreat >= 1.5
}

// PassSell: defense should shade pass (lighter box / faster ears).
func (c LineContext) PassSell() bool {
	return c.PassThreat >= 2.0 || c.PassHappy() || c.ObviousPass
}

// WantsSpy is a dedicated hole player — a coverage cost, not the default.
func (c LineContext) WantsSpy() bool {
	return c.KeepThreat >= 1.5
}

func (c LineContext) holdSeconds(front string) float64 {
	hold := 1.15
	if c.RunThreat >= 2 && !c.ObviousPass {
		hold += 0.28
	} else if c.RunThreat >= 1 && !c.ObviousPass {
		hold += 0.12
	}
	if c.ObviousPass {
		hold -= 0.30
	}
	if c.PassHappy() {
		hold -= 0.18
	}
	if front == "blitz" || front == "pass_rush" {
		hold -= 0.10
	}
	if hold < 0.55 {
		hold = 0.55
	}
	if hold > 1.45 {
		hold = 1.45
	}
	return hold
}

func (c LineContext) rushDelay(front string) float64 {
	if front == "blitz" || c.ObviousPass || c.PassHappy() {
		return 0
	}
	if c.RunThreat >= 2 {
		return 0.32
	}
	if c.RunThreat >= 1 {
		return 0.18
	}
	return 0.10
}

func (c LineContext) runPushBonus(front string) float64 {
	if front == "run_fit" {
		return 0
	}
	if c.PassThreat >= 2 || c.PassHappy() {
		return 0.7
	}
	if c.PassThreat >= 1.2 {
		return 0.35
	}
	return 0
}

// assignPassPro is 1:1 OL → DL by X. Extra OL has no one. A blitz LB is never claimed.
func assignPassPro(w *World, def playbook.DefenseCall) {
	var oline, dl, lbs []int
	for i, u := range w.Units {
		if u.Side == SideOffense && u.Role == RoleOL {
			oline = append(oline, i)
		}
		if u.Side == SideDefense && u.Role == RoleDL {
			dl = append(dl, i)
		}
		if u.Side == SideDefense && u.Role == RoleLB {
			lbs = append(lbs, i)
		}
	}
	sortByX(w, oline)
	sortByX(w, dl)
	sortByX(w, lbs)

	for _, oi := range oline {
		w.Units[oi].BlockTarget = -1
	}
	claimed := map[int]bool{}
	n := len(oline)
	if len(dl) < n {
		n = len(dl)
	}
	for i := 0; i < n; i++ {
		w.Units[oline[i]].BlockTarget = dl[i]
		claimed[dl[i]] = true
	}

	// Designate the Mike as the free rusher on blitz — no OL may claim him.
	if def.ID == "blitz" && len(lbs) > 0 {
		mike := lbs[len(lbs)/2]
		for i := range w.Units {
			if w.Units[i].BlockTarget == mike {
				w.Units[i].BlockTarget = -1
			}
		}
		w.Units[mike].RushFree = true
	}
	_ = claimed
}

// assignQBSpy puts a dedicated hole player only when keeps have been working.
// Ordinary snaps rely on rush lanes + react-on-commit (QBKeep).
func assignQBSpy(w *World, los float64, def playbook.DefenseCall, line LineContext) {
	for i := range w.Units {
		w.Units[i].Spy = false
	}
	if !line.WantsSpy() {
		return
	}
	idx := pickSpy(w)
	if idx < 0 {
		return
	}
	u := &w.Units[idx]
	u.Spy = true
	// Don't also ask him to cover a man / deep.
	if u.CoverJob == CoverMan || u.CoverJob.IsDeep() {
		u.CoverJob = CoverHook
		u.CoverMan = -1
	}
	depth := 5.0
	if line.KeepThreat >= 2 || line.ObviousPass {
		depth = 3.5
	}
	if def.ID == "run_fit" {
		depth = 3.2
	}
	u.Pos.X = field.HashMid
	u.Pos.Y = los + depth
	u.Pos = field.Clamp(u.Pos)
	u.Engaged = 0
}

func pickSpy(w *World) int {
	// Prefer a non-blitzing Mike, then any LB, then a safety.
	best, bestD := -1, 1e9
	for i, u := range w.Units {
		if u.Side != SideDefense || u.Role != RoleLB || u.RushFree {
			continue
		}
		d := math.Abs(u.Pos.X - field.HashMid)
		if d < bestD {
			bestD, best = d, i
		}
	}
	if best >= 0 {
		return best
	}
	for i, u := range w.Units {
		if u.Side == SideDefense && u.Role == RoleS && !u.CoverJob.IsDeep() {
			return i
		}
	}
	for i, u := range w.Units {
		if u.Side == SideDefense && u.Role == RoleS {
			return i
		}
	}
	return -1
}

func SpyIndex(w *World) int {
	if w == nil {
		return -1
	}
	for i, u := range w.Units {
		if u.Spy {
			return i
		}
	}
	return -1
}

func (ps *PlayState) declareKeep() {
	if ps.Thrown || ps.QBKeep || ps.Play.Type != playbook.PlayPass {
		return
	}
	if ps.QBIdx < 0 || !ps.World.Units[ps.QBIdx].HasBall {
		return
	}
	qb := ps.World.Units[ps.QBIdx]
	// A dropback sits *behind* the LOS. A keep is north through it.
	if qb.Pos.Y >= ps.LOS+0.05 {
		ps.commitKeep()
		return
	}
	if ps.Elapsed >= 0.5 && qb.Pos.Y >= ps.LOS-0.35 && qb.VY > 2.5 {
		ps.commitKeep()
		return
	}
	// Slant keep declares a beat earlier — mash-up was crossing before the scrape.
	if ps.Play.ID == "slant" && ps.Elapsed >= 0.30 && qb.Pos.Y >= ps.LOS-1.1 && qb.VY > 2.2 {
		ps.commitKeep()
	}
}

func (ps *PlayState) commitKeep() {
	ps.QBKeep = true
	ps.freeSpyToTackle()
	ps.scrapeKeepHole()
}

// shadeKeepHole sits the Mike in the A-gap on Cover 2 slants so a keep
// is not a vacant draw. Not a dedicated Spy — first-down Cover 3 stays clean.
func shadeKeepHole(w *World, los float64, shell playbook.CoverageShell) {
	if w == nil || shell.ID != playbook.ShellCover2 {
		return
	}
	idx := closestHookLB(w)
	if idx < 0 {
		return
	}
	u := &w.Units[idx]
	if u.Spy || u.RushFree {
		return
	}
	u.HoleFill = true
	u.CoverJob = CoverHook
	u.CoverMan = -1
	u.CoverLand = field.Pos{X: field.HashMid, Y: los + 5.0}
	u.Pos = u.CoverLand
	u.Pos = field.Clamp(u.Pos)
	u.Engaged = 0
}

func closestHookLB(w *World) int {
	best, bestD := -1, 1e9
	for i, u := range w.Units {
		if u.Side != SideDefense || u.Role != RoleLB {
			continue
		}
		if u.CoverJob != CoverHook && u.CoverJob != CoverNone {
			// Prefer hook; allow a box LB with no job.
			if u.CoverJob.IsDeep() || u.CoverJob.IsFlat() || u.CoverJob == CoverMan {
				continue
			}
		}
		d := math.Abs(u.Pos.X - field.HashMid)
		if d < bestD {
			bestD, best = d, i
		}
	}
	return best
}

// scrapeKeepHole unblocks the hole player so react-on-commit can finish.
func (ps *PlayState) scrapeKeepHole() {
	idx := SpyIndex(ps.World)
	if idx < 0 {
		for i, u := range ps.World.Units {
			if u.HoleFill {
				idx = i
				break
			}
		}
	}
	if idx < 0 {
		idx = closestHookLB(ps.World)
	}
	if idx < 0 {
		return
	}
	ps.World.Units[idx].Engaged = 0
	for i := range ps.World.Units {
		if ps.World.Units[i].BlockTarget == idx {
			ps.World.Units[i].BlockTarget = -1
		}
	}
}

// freeSpyToTackle drops blocks on the spy so a paid look can actually finish the play.
func (ps *PlayState) freeSpyToTackle() {
	idx := SpyIndex(ps.World)
	if idx < 0 {
		return
	}
	ps.World.Units[idx].Engaged = 0
	for i := range ps.World.Units {
		if ps.World.Units[i].BlockTarget == idx {
			ps.World.Units[i].BlockTarget = -1
		}
	}
}

// UnblockedRushers is defenders rushing with no OL BlockTarget on them.
func UnblockedRushers(w *World) []int {
	var rush []int
	if w == nil {
		return rush
	}
	claimed := map[int]bool{}
	for _, u := range w.Units {
		if u.Side == SideOffense && u.BlockTarget >= 0 {
			claimed[u.BlockTarget] = true
		}
	}
	for i, u := range w.Units {
		if u.Side != SideDefense {
			continue
		}
		if u.Role != RoleDL && !u.RushFree {
			continue
		}
		if !claimed[i] {
			rush = append(rush, i)
		}
	}
	return rush
}

func (ps *PlayState) applyPassPro(dt float64) {
	if ps.Play.Type != playbook.PlayPass || ps.BallInAir {
		return
	}
	ps.PocketLeft -= dt
	// After the fake the pocket is on a clock — leftover bite is not extra protection.
	if ps.PlayAction && ps.Mesh != MeshLive && !ps.Thrown && !ps.QBKeep {
		ps.PocketLeft -= dt * paHoldDrain
	}
	qb := field.Pos{}
	if ps.QBIdx >= 0 {
		qb = ps.World.Units[ps.QBIdx].Pos
	}

	// One OL per claimed rusher — never two on one man.
	seen := map[int]bool{}
	for i := range ps.World.Units {
		u := &ps.World.Units[i]
		if u.Side != SideOffense || u.Role != RoleOL || u.BlockTarget < 0 {
			continue
		}
		ti := u.BlockTarget
		if ti >= len(ps.World.Units) || seen[ti] {
			u.BlockTarget = -1
			continue
		}
		seen[ti] = true
		d := &ps.World.Units[ti]
		if d.Side != SideDefense {
			continue
		}

		dist := math.Hypot(d.Pos.X-u.Pos.X, d.Pos.Y-u.Pos.Y)
		if ps.PocketLeft <= 0 {
			// Beaten: stop picking, let the rusher go.
			continue
		}
		// Stay between the rusher and the QB (a pocket face, not a teleport).
		anchor := field.Pos{
			X: d.Pos.X*0.55 + qb.X*0.45,
			Y: d.Pos.Y*0.4 + qb.Y*0.6,
		}
		moveToward(u, anchor, u.Speed*0.95)

		if dist < 2.4 {
			d.Engaged = 0.16
			d.VX *= 0.12
			d.VY *= 0.12
			// Soft reset: don't walk through the OL.
			if dist < 0.85 {
				dx := d.Pos.X - u.Pos.X
				dy := d.Pos.Y - u.Pos.Y
				if mag := math.Hypot(dx, dy); mag > 0.05 {
					d.Pos.X = u.Pos.X + dx/mag*0.9
					d.Pos.Y = u.Pos.Y + dy/mag*0.9
				}
			}
		}
	}
	_ = dt
}

func claimedRusher(w *World, defIdx int) bool {
	for _, u := range w.Units {
		if u.Side == SideOffense && u.BlockTarget == defIdx {
			return true
		}
	}
	return false
}
