package sim

import (
	"fmt"
	"math"
	"strings"

	"github.com/randerson1184/tecmo-sooper-bowl/internal/field"
	"github.com/randerson1184/tecmo-sooper-bowl/internal/playbook"
)

// MeshPhase is the play-action fake. PA is not "post with a bite flag."
type MeshPhase int

const (
	MeshNone MeshPhase = iota
	MeshLive
	MeshDone
	MeshAbort
)

func (m MeshPhase) String() string {
	switch m {
	case MeshLive:
		return "live"
	case MeshDone:
		return "complete"
	case MeshAbort:
		return "aborted"
	default:
		return "none"
	}
}

const (
	paMeshSec       = 0.22
	paMeshBehind    = 3.6
	paPocketTax     = 0.10
	paWarmLeft      = 0.48 // leftover after mesh when RunThreat >= 1.5 (bite ~0.70)
	paHotLeft       = 0.58 // leftover when RunThreat >= 2.5 (bite ~0.80)
	paFitLeft       = 0.08
	paSellLeftMul   = 0.35 // unearned leftover: pass-sell may flatten
	paSellLiveMul   = 0.75 // live run: pass-sell only shaves
	paLiveLeftMin   = 0.36 // live RunThreat keeps a hittable leftover under pass-sell
	paHoldDrain     = 0.45 // extra pocket drain after the fake
	glanceStemMult  = 1.80 // plant by leftover, not after it
	glanceStemUntil = 0.48
)

func (ps *PlayState) cacheConcept() {
	c, ok := playbook.ConceptFor(ps.Play.ID)
	ps.Concept = c
	ps.HasConcept = ok
	ps.PlayAction = ok && c.PlayAction
	ps.IsPostShot = ps.Play.ID == "post" || (ok && c.PrimaryBreak == "post")
	ps.SnapRunThreat = ps.Line.RunThreat
	if !ps.PlayAction {
		ps.Mesh = MeshNone
		ps.MeshSec = 0
		ps.BiteSec = 0
		return
	}
	ps.Mesh = MeshLive
	mesh, left, bite := computePAWindow(ps.Line, ps.Def.ID)
	ps.MeshSec = mesh
	ps.LeftoverSec = left
	ps.BiteSec = bite
	ps.PocketLeft -= paPocketTax
	if ps.PocketLeft < 0.45 {
		ps.PocketLeft = 0.45
	}
}

// computePAWindow is mesh + leftover. Cold leftover is 0 — they sell during
// the fake and recover when it ends. A live run (or Run Fit) buys time after.
// Pass-sell may flatten unearned leftover; live RunThreat vetoes the crush
// the same way it vetoes lighting the box.
func computePAWindow(line LineContext, defID string) (mesh, leftover, bite float64) {
	mesh = paMeshSec
	if line.RunThreat >= 2.5 {
		leftover = paHotLeft
	} else if line.RunThreat >= 1.5 {
		leftover = paWarmLeft
	}
	if defID == "run_fit" {
		leftover += paFitLeft
	}
	if line.PassSell() {
		if line.RunThreat >= 1.5 {
			leftover *= paSellLiveMul
			if leftover < paLiveLeftMin {
				leftover = paLiveLeftMin
			}
		} else {
			leftover *= paSellLeftMul
		}
	}
	if leftover < 0 {
		leftover = 0
	}
	return mesh, leftover, mesh + leftover
}

func computeBiteSec(line LineContext, defID string) float64 {
	_, _, b := computePAWindow(line, defID)
	return b
}

func (ps *PlayState) meshPoint() field.Pos {
	return field.Pos{X: field.HashMid, Y: ps.LOS - paMeshBehind}
}

func (ps *PlayState) armPlayAction() {
	if !ps.PlayAction {
		return
	}
	mesh := ps.meshPoint()
	if ps.QBIdx >= 0 {
		u := &ps.World.Units[ps.QBIdx]
		u.Target = mesh
		u.HasTarget = true
	}
	if ps.RBIdx >= 0 {
		u := &ps.World.Units[ps.RBIdx]
		u.Target = mesh
		u.HasTarget = true
	}
	// Glance: on the stem, not already at the sit — plant during leftover.
	if ps.isGlance() && ps.PrimaryIdx >= 0 {
		u := &ps.World.Units[ps.PrimaryIdx]
		if u.Pos.Y < ps.LOS+4.2 {
			u.Pos.Y = ps.LOS + 4.2
			u.Pos = field.Clamp(u.Pos)
		}
	}
}

// tickPlayAction runs before the throw request. Space during the mesh is
// buffered; an explicit tryThrow while the mesh is live aborts the fake.
func (ps *PlayState) tickPlayAction(in *Input) {
	if !ps.PlayAction || ps.Thrown || ps.QBKeep {
		return
	}
	if ps.Mesh == MeshLive {
		if in.Juke {
			ps.abortMesh()
		}
		if in.Throw {
			ps.ThrowArmed = true
			in.Throw = false
		}
		if ps.Mesh == MeshLive {
			ps.driveMesh()
			if ps.Elapsed >= ps.MeshSec {
				ps.completeMesh()
			}
		}
	}
	if ps.ThrowArmed && ps.Mesh == MeshDone && !ps.Thrown {
		ps.ThrowArmed = false
		ps.tryThrow()
	}
}

func (ps *PlayState) driveMesh() {
	mesh := ps.meshPoint()
	if ps.QBIdx >= 0 {
		qb := &ps.World.Units[ps.QBIdx]
		moveToward(qb, mesh, qb.Speed*0.9)
		// Stay in the backfield — the fake is not a keep.
		if qb.Pos.Y > ps.LOS-1.4 {
			qb.Pos.Y = ps.LOS - 1.4
			if qb.VY > 0 {
				qb.VY = 0
			}
		}
	}
	if ps.RBIdx >= 0 {
		rb := &ps.World.Units[ps.RBIdx]
		moveToward(rb, mesh, rb.Speed*0.95)
	}
	ps.showMeshBall()
}

func (ps *PlayState) showMeshBall() {
	if ps.World == nil || ps.QBIdx < 0 || ps.RBIdx < 0 {
		return
	}
	qb := ps.World.Units[ps.QBIdx].Pos
	rb := ps.World.Units[ps.RBIdx].Pos
	ps.World.Ball = field.Pos{X: (qb.X + rb.X) * 0.5, Y: (qb.Y + rb.Y) * 0.5}
}

func (ps *PlayState) completeMesh() {
	if ps.Mesh != MeshLive {
		return
	}
	ps.Mesh = MeshDone
	ps.restorePARoutes()
}

func (ps *PlayState) abortMesh() {
	if ps.Mesh != MeshLive {
		return
	}
	ps.Mesh = MeshAbort
	ps.ThrowArmed = false
	ps.restorePARoutes()
}

func (ps *PlayState) restorePARoutes() {
	if ps.QBIdx >= 0 {
		drop := 3.8
		if ps.HasConcept && ps.Concept.DropYards > 0 {
			drop = ps.Concept.DropYards
		}
		u := &ps.World.Units[ps.QBIdx]
		u.Target = field.Pos{X: u.Pos.X, Y: ps.LOS - drop}
		u.HasTarget = true
	}
	if ps.RBIdx >= 0 {
		u := &ps.World.Units[ps.RBIdx]
		u.Target = field.Pos{X: field.HashMid, Y: ps.LOS + 10}
		u.HasTarget = true
	}
}

func (ps *PlayState) isGlance() bool {
	return ps.HasConcept && ps.Concept.PrimaryBreak == "glance"
}

func (ps *PlayState) glanceStem(i int) bool {
	if !ps.isGlance() || i != ps.PrimaryIdx || ps.Thrown {
		return false
	}
	u := ps.World.Units[i]
	if ps.Elapsed >= glanceStemUntil {
		return false
	}
	if !u.HasTarget {
		return true
	}
	return math.Hypot(u.Pos.X-u.Target.X, u.Pos.Y-u.Target.Y) > 1.6
}

// glanceWrapRadius is YAC close-out. Cold leftover: hook/LB are already there.
// Live leftover: they bit to the LOS, so the first yards after the catch exist.
func (ps *PlayState) glanceWrapRadius() float64 {
	if !ps.isGlance() || ps.CaughtAt <= 0 || ps.Elapsed-ps.CaughtAt > 1.25 {
		return 0
	}
	if ps.glanceEarnedThrow() {
		if ps.Shell.ID == playbook.ShellCover2 {
			return 1.48
		}
		return 1.40
	}
	// Missed leftover after a live run: sit in traffic harder than a cold sit.
	missed := ps.glanceMissedLeftover()
	late := ps.glanceHeldLate()
	switch ps.Shell.ID {
	case playbook.ShellManFree:
		if missed {
			return 1.88
		}
		if late {
			return 1.78
		}
		return 1.62
	case playbook.ShellCover2:
		if missed {
			return 2.05
		}
		if late {
			return 1.95
		}
		return 1.82
	default:
		if missed {
			return 2.02
		}
		if late {
			return 1.92
		}
		return 1.78
	}
}

func (ps *PlayState) glanceWrapSpeed(u *Unit) float64 {
	if !ps.isGlance() || ps.CaughtAt <= 0 {
		return 1
	}
	if u.CoverJob.IsDeep() {
		return 1
	}
	if u.Role != RoleLB && u.CoverJob != CoverHook && !u.CoverJob.IsFlat() && u.CoverJob != CoverMan {
		return 1
	}
	if ps.glanceEarnedThrow() {
		return 1.08
	}
	if ps.glanceMissedLeftover() {
		return 1.46
	}
	if ps.glanceHeldLate() {
		return 1.38
	}
	return 1.28
}

func (ps *PlayState) glanceSit(i int) bool {
	return ps.isGlance() && i == ps.PrimaryIdx
}

// glanceWindowClosed: leftover is over (or never earned). Coverage sits on
// the landmark instead of letting the stem become a delayed post.
func (ps *PlayState) glanceWindowClosed() bool {
	if !ps.isGlance() || ps.Thrown {
		return false
	}
	if ps.Mesh == MeshAbort || ps.Mesh == MeshNone {
		return true
	}
	if ps.Mesh == MeshLive {
		return false
	}
	return ps.Elapsed >= ps.BiteSec
}

// glanceEarnedThrow is a leftover release — the sit while they are still down.
func (ps *PlayState) glanceEarnedThrow() bool {
	return ps.isGlance() && ps.LeftoverSec >= 0.18 && ps.ReleaseAt > 0 &&
		ps.ReleaseAt <= ps.BiteSec+0.08
}

// glanceMissedLeftover: they had the window and threw after it closed.
func (ps *PlayState) glanceMissedLeftover() bool {
	return ps.isGlance() && ps.LeftoverSec >= 0.18 && ps.ReleaseAt > ps.BiteSec+0.08
}

// glanceHeldLate is a sit throw after leftover (or a cold mesh).
func (ps *PlayState) glanceHeldLate() bool {
	if !ps.isGlance() || ps.ReleaseAt <= 0 {
		return false
	}
	return ps.ReleaseAt > ps.BiteSec+0.08
}

func (ps *PlayState) isPlayAction() bool {
	return ps.PlayAction
}

func (ps *PlayState) isPostShot() bool {
	return ps.IsPostShot
}

func (ps *PlayState) paBiteSec() float64 {
	return ps.BiteSec
}

func (ps *PlayState) paRushDelay(base float64) float64 {
	if !ps.PlayAction {
		return base
	}
	// After the fake they know it's a pass — no more courtesy delay.
	if ps.Mesh != MeshLive {
		return 0
	}
	return base * 0.5
}

// paSackBonus is extra sack radius once the mesh is over. Holding 2s is not free.
func (ps *PlayState) paSackBonus() float64 {
	if !ps.PlayAction || ps.Mesh == MeshLive || ps.Thrown || ps.QBKeep {
		return 0
	}
	extra := 0.10
	if ps.Elapsed > 1.15 {
		extra += 0.16
	}
	if ps.Elapsed > 1.8 {
		extra += 0.14
	}
	return extra
}

func (ps *PlayState) paInLeftover() bool {
	return ps.PlayAction && ps.Mesh == MeshDone && !ps.Thrown &&
		ps.LeftoverSec > 0.02 && ps.Elapsed < ps.BiteSec
}

// InLeftoverWindow is the HUD hook: leftover bite is live after a completed mesh.
func (ps *PlayState) InLeftoverWindow() bool {
	return ps != nil && ps.paInLeftover()
}

func (ps *PlayState) paCrashMult() float64 {
	// Duration is the main knob; a live run also crashes a little harder.
	if ps.SnapRunThreat >= 2.5 {
		return 1.22
	}
	if ps.SnapRunThreat >= 1.5 {
		return 1.16
	}
	return 1.12
}

// sellPlayAction crashes LBs / hook toward the run fake.
// An aborted mesh (immediate throw) buys no bite. Rushers and the spy never sell.
func (ps *PlayState) sellPlayAction(u *Unit, qbPos field.Pos) bool {
	if !ps.PlayAction || ps.Thrown || ps.Mesh == MeshAbort || ps.Mesh == MeshNone {
		return false
	}
	if ps.Elapsed >= ps.BiteSec {
		return false
	}
	switch {
	case u.Role == RoleDL, u.Spy, u.RushFree:
		return false
	case u.Role == RoleLB:
		if u.Engaged > 0 {
			u.Engaged *= 0.35
		}
		aim := field.Pos{X: field.HashMid*0.55 + qbPos.X*0.45, Y: ps.LOS + 1.2}
		moveToward(u, aim, u.Speed*ps.paCrashMult())
		ps.noteBite(u)
		return true
	case u.CoverJob == CoverHook:
		moveToward(u, field.Pos{X: u.Pos.X, Y: ps.LOS + 3.2}, u.Speed*0.8)
		ps.noteBite(u)
		return true
	case u.CoverJob.IsDeep() && ps.SnapRunThreat >= 1.5:
		sit := u.CoverLand
		sit.Y -= 2.2
		moveToward(u, sit, u.Speed*0.5)
		ps.noteBite(u)
		return true
	default:
		return false
	}
}

func (ps *PlayState) noteBite(u *Unit) {
	id := fmt.Sprintf("%s#%d", u.Role, u.ID)
	for _, x := range ps.BiterIDs {
		if x == id {
			return
		}
	}
	ps.BiterIDs = append(ps.BiterIDs, id)
	ps.BiterN = len(ps.BiterIDs)
}

func (ps *PlayState) primarySep() float64 {
	if ps.PrimaryIdx < 0 || ps.World == nil {
		return 0
	}
	pr := ps.World.Units[ps.PrimaryIdx].Pos
	best := 1e9
	for _, u := range ps.World.Units {
		if u.Side != SideDefense {
			continue
		}
		d := math.Hypot(u.Pos.X-pr.X, u.Pos.Y-pr.Y)
		if d < best {
			best = d
		}
	}
	if best > 1e8 {
		return 0
	}
	return best
}

func (ps *PlayState) markRelease() {
	ps.ReleaseAt = ps.Elapsed
	ps.SepAtThrow = ps.primarySep()
}

func (ps *PlayState) fillPAResult(r *Result) {
	r.RunThreat = ps.SnapRunThreat
	r.BiteSec = ps.BiteSec
	r.LeftoverSec = ps.LeftoverSec
	r.ReleaseAt = ps.ReleaseAt
	r.BiterN = ps.BiterN
	r.Biters = strings.Join(ps.BiterIDs, ",")
	r.SepAtThrow = ps.SepAtThrow
	if ps.PlayAction {
		r.Mesh = ps.Mesh.String()
	}
}
