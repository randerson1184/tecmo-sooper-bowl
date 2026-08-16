package sim

import (
	"math"
	"math/rand"
	"testing"

	"github.com/randerson1184/tecmo-sooper-bowl/internal/field"
	"github.com/randerson1184/tecmo-sooper-bowl/internal/playbook"
)

func TestDisguiseKeepsLiveJobsAndLookPicture(t *testing.T) {
	const los = 30.0
	live := playbook.ShellByID(playbook.ShellCover2)
	look := playbook.ShellByID(playbook.ShellCover3)
	w := PlacePreSnap(los)
	AlignDefense(w, defByID("base"), look, los)
	if HighSafetyCount(w, los) != 1 {
		t.Fatalf("Cover 3 look should be 1-high, got %d", HighSafetyCount(w, los))
	}
	ps := StartSnapLook(los, playByID("slant"), defByID("base"), live, look,
		rand.New(rand.NewSource(2)), Fatigue{}, LineContext{})
	assertShellInvariants(t, ps.World, live, los)
	if HighSafetyCount(ps.World, los) != 1 {
		t.Fatalf("at snap the picture should still be the look (1-high), got %d", HighSafetyCount(ps.World, los))
	}
	if !ps.Disguised {
		t.Fatal("expected disguised snap")
	}
}

func TestHonestSnapMatchesTheShellPicture(t *testing.T) {
	const los = 30.0
	c2 := playbook.ShellByID(playbook.ShellCover2)
	ps := StartSnap(los, playByID("slant"), defByID("base"), c2,
		rand.New(rand.NewSource(2)), Fatigue{}, LineContext{})
	if HighSafetyCount(ps.World, los) < 2 {
		t.Fatalf("honest Cover 2 should be 2-high, got %d", HighSafetyCount(ps.World, los))
	}
	if ps.Disguised {
		t.Fatal("StartSnap without a look should not disguise")
	}
}

func TestCoverageInvariantsEveryShellAndPlay(t *testing.T) {
	shells := playbook.DefaultShells()
	plays := playbook.DefaultOffense()
	def := defByID("base")
	const los = 30.0

	for _, sh := range shells {
		for _, p := range plays {
			t.Run(sh.ID+"/"+p.ID, func(t *testing.T) {
				rng := rand.New(rand.NewSource(1))
				ps := StartSnap(los, p, def, sh, rng, Fatigue{}, LineContext{})
				assertShellInvariants(t, ps.World, sh, los)
			})
		}
	}
}

func assertShellInvariants(t *testing.T, w *World, sh playbook.CoverageShell, los float64) {
	t.Helper()
	deeps := DeepOwners(w)
	if len(deeps) < 1 {
		t.Fatalf("no deep owner in %s", sh.ID)
	}
	for _, i := range deeps {
		u := w.Units[i]
		if u.CoverLand.Y < los+10 {
			t.Errorf("deep job %s landmark Y=%.1f; want >= LOS+10", u.CoverJob, u.CoverLand.Y)
		}
	}

	switch sh.ID {
	case playbook.ShellCover3:
		if !jobHasDeepMid(w) {
			t.Fatal("Cover 3: deep middle has no owner")
		}
		var left, right bool
		for _, u := range w.Units {
			if u.CoverJob == CoverDeepLeft {
				left = true
			}
			if u.CoverJob == CoverDeepRight {
				right = true
			}
		}
		if !left || !right {
			t.Fatalf("Cover 3 missing a deep third (left=%v right=%v)", left, right)
		}
		if len(deeps) < 3 {
			t.Fatalf("Cover 3 want 3 deep owners, got %d", len(deeps))
		}
	case playbook.ShellCover2:
		l, r := jobHasDeepHalves(w)
		if !l || !r {
			t.Fatalf("Cover 2 missing a deep half (left=%v right=%v)", l, r)
		}
		var flatL, flatR bool
		for _, u := range w.Units {
			if u.CoverJob == CoverFlatL {
				flatL = true
			}
			if u.CoverJob == CoverFlatR {
				flatR = true
			}
		}
		if !flatL || !flatR {
			t.Fatalf("Cover 2 missing a flat (L=%v R=%v)", flatL, flatR)
		}
	case playbook.ShellManFree:
		if !jobHasDeepMid(w) {
			t.Fatal("Man Free: no free safety / deep middle")
		}
		// Every WR must have a man.
		for i, o := range w.Units {
			if o.Side != SideOffense || o.Role != RoleWR {
				continue
			}
			if !hasMan(w, i) {
				t.Errorf("WR idx %d at x=%.1f has no man", i, o.Pos.X)
			}
		}
	}

	// No two deep jobs should share the exact same landmark.
	seen := map[CoverJob]field.Pos{}
	for _, u := range w.Units {
		if !u.CoverJob.IsDeep() {
			continue
		}
		if prev, ok := seen[u.CoverJob]; ok {
			if prev == u.CoverLand {
				t.Errorf("duplicate deep job %s at %+v", u.CoverJob, prev)
			}
		}
		seen[u.CoverJob] = u.CoverLand
	}
}

func hasMan(w *World, offIdx int) bool {
	for _, u := range w.Units {
		if u.CoverJob == CoverMan && u.CoverMan == offIdx {
			return true
		}
	}
	return false
}

func TestCover3DoesNotSquatTheHitchLandmark(t *testing.T) {
	ps := StartSnap(30, playByID("hitch"), defByID("base"), playbook.ShellByID(playbook.ShellCover3), rand.New(rand.NewSource(1)), Fatigue{}, LineContext{})
	for _, u := range ps.World.Units {
		if u.Role != RoleCB || !u.CoverJob.IsDeep() {
			continue
		}
		if u.CoverLand.Y < 30+10 {
			t.Fatalf("Cover 3 CB landmark too shallow: Y=%.1f", u.CoverLand.Y)
		}
	}
}

func TestCover2CornersAreInTheFlat(t *testing.T) {
	ps := StartSnap(30, playByID("hitch"), defByID("base"), playbook.ShellByID(playbook.ShellCover2), rand.New(rand.NewSource(1)), Fatigue{}, LineContext{})
	found := 0
	for _, u := range ps.World.Units {
		if u.Role == RoleCB && u.CoverJob.IsFlat() {
			found++
			if u.CoverLand.Y > 30+6 {
				t.Fatalf("Cover 2 CB not squatting: landmark Y=%.1f", u.CoverLand.Y)
			}
		}
	}
	if found < 2 {
		t.Fatalf("Cover 2 expected 2 flat CBs, got %d", found)
	}
}

func TestPostPrimaryIsDeepAndShellsStillHaveHelp(t *testing.T) {
	const los = 30.0
	play := playByID("post")
	if play.ID != "post" {
		t.Fatal("post missing from playbook")
	}
	for _, sh := range playbook.DefaultShells() {
		ps := StartSnap(los, play, defByID("base"), sh, rand.New(rand.NewSource(1)), Fatigue{}, LineContext{})
		if ps.PrimaryIdx < 0 {
			t.Fatal("post has no primary")
		}
		pr := ps.World.Units[ps.PrimaryIdx]
		if !pr.HasTarget || pr.Target.Y < los+14 {
			t.Fatalf("post primary target too short: %+v", pr.Target)
		}
		assertShellInvariants(t, ps.World, sh, los)
	}
}

func TestPreSnapPicturesAreReadable(t *testing.T) {
	const los = 30.0
	w3 := PlacePreSnap(los)
	AlignDefense(w3, defByID("base"), playbook.ShellByID(playbook.ShellCover3), los)
	w2 := PlacePreSnap(los)
	AlignDefense(w2, defByID("base"), playbook.ShellByID(playbook.ShellCover2), los)
	wm := PlacePreSnap(los)
	AlignDefense(wm, defByID("base"), playbook.ShellByID(playbook.ShellManFree), los)

	if n := countDeepSafeties(w3, los+12); n != 1 {
		t.Fatalf("Cover 3 should show one high safety, got %d", n)
	}
	if n := countDeepSafeties(w2, los+12); n < 2 {
		t.Fatalf("Cover 2 should show two high safeties, got %d", n)
	}
	if n := countDeepSafeties(wm, los+12); n != 1 {
		t.Fatalf("Man Free should show one high safety, got %d", n)
	}

	c3 := avgCBY(w3)
	c2 := avgCBY(w2)
	cm := avgCBY(wm)
	if c3 < los+6 {
		t.Fatalf("Cover 3 corners should play off, y=%.1f", c3)
	}
	if c2 > los+6 {
		t.Fatalf("Cover 2 corners should squat, y=%.1f", c2)
	}
	if cm > los+4 {
		t.Fatalf("Man Free corners should press, y=%.1f", cm)
	}
}

func countDeepSafeties(w *World, minY float64) int {
	n := 0
	for _, u := range w.Units {
		if u.Side == SideDefense && u.Role == RoleS && u.Pos.Y >= minY {
			n++
		}
	}
	return n
}

func avgCBY(w *World) float64 {
	var s float64
	var n int
	for _, u := range w.Units {
		if u.Side == SideDefense && u.Role == RoleCB {
			s += u.Pos.Y
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return s / float64(n)
}

func TestManFreeSafetyStaysDeep(t *testing.T) {
	ps := StartSnap(30, playByID("slant"), defByID("base"), playbook.ShellByID(playbook.ShellManFree), rand.New(rand.NewSource(1)), Fatigue{}, LineContext{})
	for _, u := range ps.World.Units {
		if u.CoverJob != CoverDeepMid {
			continue
		}
		if math.Abs(u.CoverLand.X-field.HashMid) > 8 {
			t.Fatalf("free safety landmark not middle: %+v", u.CoverLand)
		}
		if u.CoverLand.Y < 40 {
			t.Fatalf("free safety too shallow: %+v", u.CoverLand)
		}
		return
	}
	t.Fatal("no deep-mid safety")
}

func hitchCoverCB(ps *PlayState) int {
	if ps.PrimaryIdx < 0 {
		return -1
	}
	best, bestD := -1, 1e9
	for i, u := range ps.World.Units {
		if u.Side != SideDefense {
			continue
		}
		if u.CoverJob == CoverMan && u.CoverMan == ps.PrimaryIdx {
			return i
		}
		if !u.CoverJob.IsFlat() {
			continue
		}
		d := math.Hypot(u.Pos.X-ps.World.Units[ps.PrimaryIdx].Pos.X, u.Pos.Y-ps.World.Units[ps.PrimaryIdx].Pos.Y)
		if d < bestD {
			bestD, best = d, i
		}
	}
	return best
}

func TestHitchCornerDoesNotCrashTheQBOnThrow(t *testing.T) {
	// Film: hitch diet 21/22. On release the squat/man CB ran at BallPos (the QB).
	shells := []playbook.CoverageShell{
		playbook.ShellByID(playbook.ShellCover2),
		playbook.ShellByID(playbook.ShellManFree),
	}
	for _, sh := range shells {
		ps := StartSnap(30, playByID("hitch"), defByID("base"), sh,
			rand.New(rand.NewSource(4)), Fatigue{}, LineContext{})
		for i := 0; i < 36; i++ { // sit
			if !ps.Tick(1.0/60.0, Input{}) {
				t.Fatalf("%s died before the throw", sh.ID)
			}
		}
		cb := hitchCoverCB(ps)
		if cb < 0 || ps.QBIdx < 0 || ps.PrimaryIdx < 0 {
			t.Fatalf("%s: no hitch CB", sh.ID)
		}
		y0 := ps.World.Units[cb].Pos.Y
		qbY := ps.World.Units[ps.QBIdx].Pos.Y
		wrY := ps.World.Units[ps.PrimaryIdx].Pos.Y
		ps.Tick(1.0/60.0, Input{Throw: true})
		if !ps.BallInAir && !ps.Thrown {
			t.Fatalf("%s: throw did not leave", sh.ID)
		}
		for i := 0; i < 8 && ps.Alive && ps.BallInAir; i++ {
			ps.Tick(1.0/60.0, Input{})
		}
		cbNow := ps.World.Units[cb].Pos
		if cbNow.Y < y0-0.35 && cbNow.Y < wrY-0.5 {
			t.Fatalf("%s hitch CB crashed toward the QB on release (y0=%.1f now=%.1f qb=%.1f wr=%.1f)",
				sh.ID, y0, cbNow.Y, qbY, wrY)
		}
		dWR := math.Hypot(cbNow.X-ps.World.Units[ps.PrimaryIdx].Pos.X, cbNow.Y-ps.World.Units[ps.PrimaryIdx].Pos.Y)
		dQB := math.Hypot(cbNow.X-ps.World.Units[ps.QBIdx].Pos.X, cbNow.Y-ps.World.Units[ps.QBIdx].Pos.Y)
		if dQB+1.2 < dWR {
			t.Fatalf("%s hitch CB closer to the QB than the sit (dQB=%.1f dWR=%.1f)", sh.ID, dQB, dWR)
		}
	}
}
