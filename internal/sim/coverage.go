package sim

import (
	"math"

	"github.com/randerson1184/tecmo-sooper-bowl/internal/field"
	"github.com/randerson1184/tecmo-sooper-bowl/internal/playbook"
)

// CoverJob is a defender's post-snap assignment. Arcade shells, not Madden.
type CoverJob string

const (
	CoverNone      CoverJob = ""
	CoverDeepLeft  CoverJob = "deep_left"
	CoverDeepMid   CoverJob = "deep_mid"
	CoverDeepRight CoverJob = "deep_right"
	CoverDeepHalfL CoverJob = "deep_half_l"
	CoverDeepHalfR CoverJob = "deep_half_r"
	CoverFlatL     CoverJob = "flat_l"
	CoverFlatR     CoverJob = "flat_r"
	CoverHook      CoverJob = "hook"
	CoverMan       CoverJob = "man"
)

func (j CoverJob) IsDeep() bool {
	switch j {
	case CoverDeepLeft, CoverDeepMid, CoverDeepRight, CoverDeepHalfL, CoverDeepHalfR:
		return true
	default:
		return false
	}
}

func (j CoverJob) IsFlat() bool {
	return j == CoverFlatL || j == CoverFlatR
}

// AssignCoverage writes jobs + landmarks onto defenders and shades alignment.
// primaryIdx is the offensive primary receiver (may be -1).
func AssignCoverage(w *World, shell playbook.CoverageShell, los float64, primaryIdx int) {
	if w == nil {
		return
	}
	var cbs, safeties, lbs []int
	var wrs, tes []int
	for i, u := range w.Units {
		switch {
		case u.Side == SideDefense && u.Role == RoleCB:
			cbs = append(cbs, i)
		case u.Side == SideDefense && u.Role == RoleS:
			safeties = append(safeties, i)
		case u.Side == SideDefense && u.Role == RoleLB:
			lbs = append(lbs, i)
		case u.Side == SideOffense && u.Role == RoleWR:
			wrs = append(wrs, i)
		case u.Side == SideOffense && u.Role == RoleTE:
			tes = append(tes, i)
		}
	}
	sortByX(w, cbs)
	sortByX(w, safeties)
	sortByX(w, lbs)
	sortByX(w, wrs)

	mid := field.HashMid
	clearCover(w)

	switch shell.ID {
	case playbook.ShellCover2:
		assignCover2(w, cbs, safeties, lbs, los, mid)
	case playbook.ShellManFree:
		assignManFree(w, cbs, safeties, lbs, wrs, tes, los, mid)
	default:
		assignCover3(w, cbs, safeties, lbs, los, mid)
	}

	shadeAlignment(w, shell, los)
	_ = primaryIdx
}

// AlignDefense writes coverage jobs and the pre-snap picture onto a placed world.
func AlignDefense(w *World, def playbook.DefenseCall, shell playbook.CoverageShell, los float64) {
	if w == nil {
		return
	}
	AssignCoverage(w, shell, los, -1)
	shadeFront(w, def, los)
}

func shadeFront(w *World, def playbook.DefenseCall, los float64) {
	for i := range w.Units {
		u := &w.Units[i]
		if u.Side != SideDefense {
			continue
		}
		switch def.ID {
		case "run_fit":
			if u.Role == RoleLB || u.Role == RoleDL {
				u.Pos.Y -= 0.8
			}
		case "soft_zone":
			if u.Role == RoleLB {
				u.Pos.Y += 1.2
			}
			if u.Role == RoleS || u.Role == RoleCB {
				u.Pos.Y += 1.6
			}
		case "pass_rush":
			if u.Role == RoleDL {
				u.Pos.Y -= 0.35
			}
		case "blitz":
			if u.Role == RoleLB && math.Abs(u.Pos.X-field.HashMid) < 5 {
				u.Pos.Y -= 1.4
			}
		}
		u.Pos = field.Clamp(u.Pos)
	}
	_ = los
}

func clearCover(w *World) {
	for i := range w.Units {
		if w.Units[i].Side != SideDefense {
			continue
		}
		w.Units[i].CoverJob = CoverNone
		w.Units[i].CoverMan = -1
	}
}

func assignCover3(w *World, cbs, safeties, lbs []int, los, mid float64) {
	if len(cbs) >= 1 {
		setZone(w, cbs[0], CoverDeepLeft, field.Pos{X: mid - 17, Y: los + 13})
	}
	if len(cbs) >= 2 {
		setZone(w, cbs[len(cbs)-1], CoverDeepRight, field.Pos{X: mid + 17, Y: los + 13})
	}
	if len(safeties) >= 1 {
		setZone(w, safeties[0], CoverDeepMid, field.Pos{X: mid, Y: los + 15.5})
	}
	if len(safeties) >= 2 {
		// Strong safety: curl/flat helper — the YAC cop, not a second deep.
		setZone(w, safeties[1], CoverHook, field.Pos{X: mid + 7, Y: los + 8})
	}
	for i, lb := range lbs {
		x := mid
		if i == 0 {
			x = mid - 8
		} else if i == len(lbs)-1 {
			x = mid + 8
		}
		job := CoverHook
		if i == 0 {
			job = CoverFlatL
			x = mid - 12
		} else if i == len(lbs)-1 {
			job = CoverFlatR
			x = mid + 12
		}
		setZone(w, lb, job, field.Pos{X: x, Y: los + 6})
	}
}

func assignCover2(w *World, cbs, safeties, lbs []int, los, mid float64) {
	if len(cbs) >= 1 {
		setZone(w, cbs[0], CoverFlatL, field.Pos{X: mid - 18, Y: los + 3.6})
	}
	if len(cbs) >= 2 {
		setZone(w, cbs[len(cbs)-1], CoverFlatR, field.Pos{X: mid + 18, Y: los + 3.6})
	}
	if len(safeties) >= 1 {
		setZone(w, safeties[0], CoverDeepHalfL, field.Pos{X: mid - 11, Y: los + 14})
	}
	if len(safeties) >= 2 {
		setZone(w, safeties[1], CoverDeepHalfR, field.Pos{X: mid + 11, Y: los + 14})
	} else if len(safeties) == 1 {
		// One safety: still a deep half so the invariant holds.
		setZone(w, safeties[0], CoverDeepHalfL, field.Pos{X: mid, Y: los + 14})
	}
	for i, lb := range lbs {
		x := mid
		job := CoverHook
		if i == 0 {
			x = mid - 7
		} else if i == len(lbs)-1 {
			x = mid + 7
		} else {
			// Mike drops the hole — that's the Cover 2 / Tampa tell.
			x = mid
			job = CoverHook
		}
		y := los + 7
		if i > 0 && i < len(lbs)-1 {
			y = los + 9.5
		}
		setZone(w, lb, job, field.Pos{X: x, Y: y})
	}
}

func assignManFree(w *World, cbs, safeties, lbs, wrs, tes []int, los, mid float64) {
	// Match CBs to outside WRs by X.
	usedOff := map[int]bool{}
	for i, cb := range cbs {
		man := -1
		if i < len(wrs) {
			if i == 0 {
				man = wrs[0]
			} else {
				man = wrs[len(wrs)-1]
			}
		}
		if man >= 0 {
			usedOff[man] = true
			setMan(w, cb, man)
		} else {
			setZone(w, cb, CoverHook, field.Pos{X: w.Units[cb].Pos.X, Y: los + 6})
		}
	}
	// Remaining WRs + TE get LBs.
	var leftovers []int
	for _, wr := range wrs {
		if !usedOff[wr] {
			leftovers = append(leftovers, wr)
		}
	}
	leftovers = append(leftovers, tes...)
	lbUsed := 0
	for _, off := range leftovers {
		if lbUsed >= len(lbs) {
			break
		}
		setMan(w, lbs[lbUsed], off)
		lbUsed++
	}
	for ; lbUsed < len(lbs); lbUsed++ {
		setZone(w, lbs[lbUsed], CoverHook, field.Pos{X: w.Units[lbs[lbUsed]].Pos.X, Y: los + 6})
	}
	// Free safety: deep middle. Extra safety is a robber.
	if len(safeties) >= 1 {
		setZone(w, safeties[0], CoverDeepMid, field.Pos{X: mid, Y: los + 15})
	}
	if len(safeties) >= 2 {
		setZone(w, safeties[1], CoverHook, field.Pos{X: mid + 4, Y: los + 8})
	}
}

func setZone(w *World, i int, job CoverJob, land field.Pos) {
	w.Units[i].CoverJob = job
	w.Units[i].CoverLand = field.Clamp(land)
	w.Units[i].CoverMan = -1
}

func setMan(w *World, i, man int) {
	w.Units[i].CoverJob = CoverMan
	w.Units[i].CoverMan = man
	w.Units[i].CoverLand = w.Units[man].Pos
}

func shadeAlignment(w *World, shell playbook.CoverageShell, los float64) {
	for i := range w.Units {
		u := &w.Units[i]
		if u.Side != SideDefense || u.CoverJob == CoverNone {
			continue
		}
		switch shell.ID {
		case playbook.ShellCover2:
			// Two high, corners squat the flats.
			if u.CoverJob.IsDeep() {
				u.Pos.X = u.CoverLand.X
				u.Pos.Y = los + 13.5
			}
			if u.CoverJob.IsFlat() {
				u.Pos.Y = los + 4.0
			}
			if u.CoverJob == CoverHook && math.Abs(u.Pos.X-field.HashMid) < 4 {
				u.Pos.Y = los + 8.5
			}
		case playbook.ShellManFree:
			// One high, corners pressed.
			if u.CoverJob == CoverDeepMid {
				u.Pos.X = field.HashMid
				u.Pos.Y = los + 14.5
			}
			if u.CoverJob == CoverHook {
				u.Pos.X = u.CoverLand.X
				u.Pos.Y = los + 8.0
			}
			if u.CoverJob == CoverMan && u.CoverMan >= 0 {
				m := w.Units[u.CoverMan].Pos
				u.Pos.X = m.X
				u.Pos.Y = m.Y + 1.3
			}
		default: // Cover 3: one high, corners off.
			if u.CoverJob == CoverDeepMid {
				u.Pos.X = field.HashMid
				u.Pos.Y = los + 14.5
			}
			if u.CoverJob == CoverDeepLeft || u.CoverJob == CoverDeepRight {
				u.Pos.Y = los + 8.2
			}
			if u.CoverJob == CoverHook {
				u.Pos.X = u.CoverLand.X
				u.Pos.Y = los + 10.5 // in the box, not a second high safety
			}
			if u.CoverJob.IsFlat() {
				u.Pos.Y = los + 5.5
			}
		}
		u.Pos = field.Clamp(u.Pos)
	}
}

// DeepOwners returns defender indices whose job is a deep zone.
func DeepOwners(w *World) []int {
	var out []int
	if w == nil {
		return out
	}
	for i, u := range w.Units {
		if u.Side == SideDefense && u.CoverJob.IsDeep() {
			out = append(out, i)
		}
	}
	return out
}

func jobHasDeepMid(w *World) bool {
	for _, u := range w.Units {
		if u.CoverJob == CoverDeepMid {
			return true
		}
	}
	return false
}

func jobHasDeepHalves(w *World) (left, right bool) {
	for _, u := range w.Units {
		if u.CoverJob == CoverDeepHalfL {
			left = true
		}
		if u.CoverJob == CoverDeepHalfR {
			right = true
		}
	}
	return
}

// coverTarget is where this defender should work during a pass (pre-throw).
func (ps *PlayState) coverTarget(i int) (field.Pos, float64, bool) {
	u := ps.World.Units[i]
	if u.CoverJob == CoverNone {
		return field.Pos{}, 0, false
	}
	if u.CoverJob == CoverMan && u.CoverMan >= 0 && u.CoverMan < len(ps.World.Units) {
		m := ps.World.Units[u.CoverMan].Pos
		// Trail: sit a yard inside/under, not on their toes.
		m.Y -= 0.8
		return m, u.Speed * 0.98, true
	}
	land := u.CoverLand
	recv := ps.receiverInZone(u)
	dest := land
	spd := u.Speed * 0.86
	if recv >= 0 {
		rp := ps.World.Units[recv].Pos
		switch {
		case u.CoverJob.IsDeep():
			// Stay over the top. Never squat a hitch.
			dest.X = land.X*0.55 + rp.X*0.45
			dest.Y = math.Max(land.Y, rp.Y+3)
			spd *= 0.92
		case u.CoverJob.IsFlat():
			// Cover 2 squat: play the receiver, not a deep landmark.
			dest.X = rp.X*0.7 + land.X*0.3
			dest.Y = rp.Y*0.55 + land.Y*0.45
			spd *= 1.02
		default:
			dest.X = land.X*0.5 + rp.X*0.5
			dest.Y = land.Y*0.65 + rp.Y*0.35
		}
	}
	if u.CoverJob.IsDeep() && dest.Y < ps.LOS+9 {
		dest.Y = ps.LOS + 9
	}
	return dest, spd, true
}

// receiverInZone picks the most relevant WR/TE for this defender's job.
func (ps *PlayState) receiverInZone(u Unit) int {
	best := -1
	bestD := 1e9
	for i, o := range ps.World.Units {
		if o.Side != SideOffense || (o.Role != RoleWR && o.Role != RoleTE) {
			continue
		}
		if !inCoverZone(u.CoverJob, o.Pos, ps.LOS) {
			continue
		}
		d := math.Hypot(o.Pos.X-u.Pos.X, o.Pos.Y-u.Pos.Y)
		// Prefer the primary if they're in this zone.
		if i == ps.PrimaryIdx {
			d *= 0.7
		}
		if d < bestD {
			bestD = d
			best = i
		}
	}
	return best
}

func inCoverZone(job CoverJob, p field.Pos, los float64) bool {
	mid := field.HashMid
	switch job {
	case CoverDeepLeft, CoverFlatL, CoverDeepHalfL:
		return p.X <= mid+2
	case CoverDeepRight, CoverFlatR, CoverDeepHalfR:
		return p.X >= mid-2
	case CoverDeepMid, CoverHook:
		return true
	case CoverMan:
		return true
	default:
		return math.Abs(p.Y-los) < 20
	}
}
