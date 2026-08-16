package sim

import (
	"math/rand"
	"testing"

	"github.com/randerson1184/tecmo-sooper-bowl/internal/playbook"
)

func TestPassProIsOneToOne(t *testing.T) {
	ps := StartPlay(30, playByID("slant"), defByID("base"), rand.New(rand.NewSource(1)), Fatigue{})
	claimed := map[int]int{}
	ol := 0
	for _, u := range ps.World.Units {
		if u.Side != SideOffense || u.Role != RoleOL {
			continue
		}
		ol++
		if u.BlockTarget < 0 {
			continue
		}
		claimed[u.BlockTarget]++
		if claimed[u.BlockTarget] > 1 {
			t.Fatalf("two OL claimed defender %d", u.BlockTarget)
		}
		tgt := ps.World.Units[u.BlockTarget]
		if tgt.Role != RoleDL {
			t.Fatalf("OL assigned to %s, want DL", tgt.Role)
		}
	}
	if ol < 5 {
		t.Fatalf("expected 5 OL, got %d", ol)
	}
	if len(claimed) != 4 {
		t.Fatalf("expected 4 claimed DL, got %d", len(claimed))
	}
}

func TestBlitzHasOneUnblockedRusher(t *testing.T) {
	ps := StartPlay(30, playByID("slant"), defByID("blitz"), rand.New(rand.NewSource(1)), Fatigue{})
	free := 0
	for _, u := range ps.World.Units {
		if u.RushFree {
			free++
			if claimedRusher(ps.World, indexOf(ps.World, u.ID)) {
				t.Fatal("RushFree LB was claimed by an OL")
			}
		}
	}
	if free != 1 {
		t.Fatalf("blitz should designate exactly one free rusher, got %d", free)
	}
	ubs := UnblockedRushers(ps.World)
	if len(ubs) < 1 {
		t.Fatal("expected at least one unblocked rusher on blitz")
	}
}

func indexOf(w *World, id int) int {
	for i, u := range w.Units {
		if u.ID == id {
			return i
		}
	}
	return -1
}

func scriptedPass(t *testing.T, def string, ctx LineContext, throwAt float64, n int) (sacks int) {
	return scriptedPassUntil(t, def, ctx, throwAt, 5.0, n)
}

func scriptedPassUntil(t *testing.T, def string, ctx LineContext, throwAt, until float64, n int) (sacks int) {
	t.Helper()
	play := playByID("slant")
	d := defByID(def)
	for seed := int64(0); seed < int64(n); seed++ {
		rng := rand.New(rand.NewSource(seed + 17))
		ps := StartSnap(30, play, d, playbook.DefaultShell(), rng, Fatigue{}, ctx)
		for i := 0; i < int(until*60)+4; i++ {
			sec := float64(i) / 60.0
			in := Input{DY: -0.15}
			if throwAt >= 0 && sec >= throwAt && sec < throwAt+0.05 {
				in.Throw = true
			}
			if !ps.Tick(1.0/60.0, in) {
				break
			}
		}
		if ps.Result.Outcome == OutcomeSack {
			sacks++
		}
	}
	return sacks
}

func TestQuickThrowUsuallyProtected(t *testing.T) {
	sacks := scriptedPass(t, "base", LineContext{}, 0.55, 20)
	if sacks > 4 {
		t.Fatalf("quick slant vs base/4 rushers should usually be clean; sacks=%d/20", sacks)
	}
}

func TestHoldingTheBallIsDangerousNotAutomatic(t *testing.T) {
	// Sit in the pocket ~1.5s — no throw. Should be dirty, not a 20/20 script.
	sacks := scriptedPassUntil(t, "base", LineContext{}, -1, 1.65, 20)
	if sacks < 8 {
		t.Fatalf("holding ~1.65s vs four rushers should collapse the pocket; got %d/20 sacks", sacks)
	}
}

func TestRunThreatDelaysOrdinaryRush(t *testing.T) {
	cold := scriptedPass(t, "base", LineContext{}, 0.70, 20)
	hot := scriptedPass(t, "base", LineContext{RunThreat: 3, Samples: 6, RunPct: 0.7}, 0.70, 20)
	if hot > cold {
		t.Fatalf("successful-run threat should not increase sacks on a 0.7s throw (cold=%d hot=%d)", cold, hot)
	}
}

func TestStuffedRunsAreNotRunThreat(t *testing.T) {
	// LineContext.RunThreat 0 with high RunPct = frequency without success.
	sacksFreq := scriptedPass(t, "base", LineContext{RunPct: 0.9, Samples: 8, RunThreat: 0}, 0.70, 16)
	sacksNone := scriptedPass(t, "base", LineContext{}, 0.70, 16)
	// Frequency alone must not buy a pocket.
	if sacksFreq+2 < sacksNone {
		t.Fatalf("stuffed-run frequency bought protection (freq sacks=%d none=%d)", sacksFreq, sacksNone)
	}
}

func TestFreshPassHasNoDedicatedSpy(t *testing.T) {
	ps := StartPlay(30, playByID("slant"), defByID("base"), rand.New(rand.NewSource(1)), Fatigue{})
	if SpyIndex(ps.World) >= 0 {
		t.Fatal("a fresh 1st-down slant should not pull a dedicated spy")
	}
}

func TestDedicatedSpyWhenKeepsAreWorking(t *testing.T) {
	ctx := LineContext{KeepThreat: 2.2, Samples: 6}
	ps := StartSnap(30, playByID("slant"), defByID("base"), playbook.DefaultShell(),
		rand.New(rand.NewSource(1)), Fatigue{}, ctx)
	if SpyIndex(ps.World) < 0 {
		t.Fatal("working QB-keep threat should buy a dedicated spy")
	}
}

func TestQBKeepUpTheMiddleIsNotAHouseCall(t *testing.T) {
	play := playByID("slant")
	def := defByID("base")
	house, keeps := 0, 0
	const n = 25
	for seed := int64(0); seed < n; seed++ {
		rng := rand.New(rand.NewSource(seed + 3))
		ps := StartPlay(30, play, def, rng, Fatigue{})
		for i := 0; i < 60*8; i++ {
			// The exploit: snap a pass, never throw, mash toward the end zone.
			if !ps.Tick(1.0/60.0, Input{DY: 1}) {
				break
			}
		}
		if ps.QBKeep {
			keeps++
		}
		if ps.Result.Thrown {
			t.Fatal("mash-up declared a throw")
		}
		if ps.Result.Outcome != OutcomeSack {
			if !ps.Result.QBKeep || ps.Result.Carrier != RoleQB {
				t.Fatalf("whistle must tag the QB run: keep=%v carrier=%s outcome=%s yards=%.1f",
					ps.Result.QBKeep, ps.Result.Carrier, ps.Result.Outcome, ps.Result.YardsGained)
			}
		}
		if ps.Result.YardsGained >= 20 {
			house++
		}
	}
	if keeps < 15 {
		t.Fatalf("expected the mash-up to declare keep often; keeps=%d/%d", keeps, n)
	}
	if house > 8 {
		t.Fatalf("QB draw up the middle still a house call: %d/%d were 20+", house, n)
	}
}

func TestSpyReducesKeepYardsVsNoSpy(t *testing.T) {
	play := playByID("slant")
	def := defByID("base")
	shell := playbook.DefaultShell()
	const n = 25
	var with, without float64
	var withN, withoutN int
	for seed := int64(0); seed < n; seed++ {
		run := func(threat float64) (float64, bool) {
			rng := rand.New(rand.NewSource(seed + 11))
			ps := StartSnap(30, play, def, shell, rng, Fatigue{}, LineContext{KeepThreat: threat, Samples: 8})
			for i := 0; i < 60*8; i++ {
				if !ps.Tick(1.0/60.0, Input{DY: 1}) {
					break
				}
			}
			if !ps.Result.QBKeep {
				return 0, false
			}
			return ps.Result.YardsGained, true
		}
		if y, ok := run(0); ok {
			without += y
			withoutN++
		}
		if y, ok := run(2.4); ok {
			with += y
			withN++
		}
	}
	if withN < 8 || withoutN < 8 {
		t.Fatalf("not enough keeps to compare (spy=%d bare=%d)", withN, withoutN)
	}
	avgWith := with / float64(withN)
	avgBare := without / float64(withoutN)
	if avgWith > avgBare-1.0 {
		t.Fatalf("spy should cut keep yards (spy=%.1f bare=%.1f)", avgWith, avgBare)
	}
}

func TestSpyMakesWorkingKeepsStuffable(t *testing.T) {
	play := playByID("slant")
	def := defByID("base")
	ctx := LineContext{KeepThreat: 2.4, Samples: 8}
	house, keeps, stuffed := 0, 0, 0
	const n = 25
	for seed := int64(0); seed < n; seed++ {
		rng := rand.New(rand.NewSource(seed + 4))
		ps := StartSnap(30, play, def, playbook.DefaultShell(), rng, Fatigue{}, ctx)
		if SpyIndex(ps.World) < 0 {
			t.Fatal("expected a spy on a live keep threat")
		}
		for i := 0; i < 60*8; i++ {
			if !ps.Tick(1.0/60.0, Input{DY: 1}) {
				break
			}
		}
		if ps.Result.QBKeep {
			keeps++
			if ps.Result.YardsGained < 6 {
				stuffed++
			}
		}
		if ps.Result.YardsGained >= 16 {
			house++
		}
	}
	if keeps < 8 {
		t.Fatalf("expected mash-up keeps; keeps=%d/%d", keeps, n)
	}
	if house > 6 {
		t.Fatalf("spy still letting keeps house: %d/%d were 16+", house, n)
	}
	if stuffed < 4 {
		t.Fatalf("spy should finish some keeps; stuffed=%d keeps=%d", stuffed, keeps)
	}
}

func TestCover2SlantShadesTheHoleWithoutASpy(t *testing.T) {
	ps := StartSnap(30, playByID("slant"), defByID("base"),
		playbook.ShellByID(playbook.ShellCover2),
		rand.New(rand.NewSource(1)), Fatigue{}, LineContext{})
	if SpyIndex(ps.World) >= 0 {
		t.Fatal("Cover 2 scrape is not a dedicated spy")
	}
	idx := closestHookLB(ps.World)
	if idx < 0 {
		t.Fatal("expected a hook LB")
	}
	if ps.World.Units[idx].Pos.Y > 30+5.5 {
		t.Fatalf("Cover 2 slant hole should sit near the LOS, y=%.1f", ps.World.Units[idx].Pos.Y)
	}
}

func TestSlantKeepVsBaseCover2IsNotAHouseCall(t *testing.T) {
	// Film: keeps vs Base / Cover 2 still +12–15 after the spy tax.
	play := playByID("slant")
	def := defByID("base")
	shell := playbook.ShellByID(playbook.ShellCover2)
	house, keeps, stuffed := 0, 0, 0
	var sum float64
	const n = 25
	for seed := int64(0); seed < n; seed++ {
		rng := rand.New(rand.NewSource(seed + 21))
		ps := StartSnap(30, play, def, shell, rng, Fatigue{}, LineContext{})
		for i := 0; i < 60*8; i++ {
			if !ps.Tick(1.0/60.0, Input{DY: 1}) {
				break
			}
		}
		if ps.Result.QBKeep {
			keeps++
			sum += ps.Result.YardsGained
			if ps.Result.YardsGained < 6 {
				stuffed++
			}
		}
		if ps.Result.YardsGained >= 14 {
			house++
		}
	}
	if keeps < 10 {
		t.Fatalf("expected mash-up keeps; keeps=%d/%d", keeps, n)
	}
	if house > 5 {
		t.Fatalf("slant keep vs base/cover2 still a house: %d/%d were 14+", house, n)
	}
	avg := sum / float64(keeps)
	if avg > 8.5 {
		t.Fatalf("keep vs cover2 should finish in the hole (avg=%.1f, want ≤8.5)", avg)
	}
	if stuffed < 4 {
		t.Fatalf("hole scrape should stuff some keeps; stuffed=%d keeps=%d avg=%.1f", stuffed, keeps, avg)
	}
}

func TestObviousPassIsHotterThanFirstDown(t *testing.T) {
	normal := scriptedPassUntil(t, "base", LineContext{}, -1, 1.45, 16)
	long := scriptedPassUntil(t, "base", LineContext{ObviousPass: true, Samples: 4}, -1, 1.45, 16)
	if long < normal {
		t.Fatalf("3rd/4th & long should not be cooler than 1st down (normal=%d long=%d)", normal, long)
	}
}
