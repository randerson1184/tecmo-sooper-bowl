package sim

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"testing"

	"github.com/randerson1184/tecmo-sooper-bowl/internal/field"
	"github.com/randerson1184/tecmo-sooper-bowl/internal/playbook"
)

func TestPAPostKeepsCoverageHelp(t *testing.T) {
	play := playByID("pa_post")
	if play.ID != "pa_post" {
		t.Fatal("pa_post missing from the book")
	}
	const los = 30.0
	for _, sh := range playbook.DefaultShells() {
		ps := StartSnap(los, play, defByID("base"), sh, rand.New(rand.NewSource(2)), Fatigue{}, LineContext{})
		if ps.PrimaryIdx < 0 {
			t.Fatal("PA post has no primary")
		}
		pr := ps.World.Units[ps.PrimaryIdx]
		if !pr.HasTarget || pr.Target.Y < los+14 {
			t.Fatalf("PA post primary too short: %+v", pr.Target)
		}
		assertShellInvariants(t, ps.World, sh, los)
	}
}

func TestPAPostRBSellsTheDive(t *testing.T) {
	ps := StartSnap(30, playByID("pa_post"), defByID("base"), playbook.DefaultShell(),
		rand.New(rand.NewSource(1)), Fatigue{}, LineContext{})
	if ps.RBIdx < 0 {
		t.Fatal("no RB")
	}
	rb := ps.World.Units[ps.RBIdx]
	if !rb.HasTarget || rb.Target.Y > 30 {
		t.Fatalf("PA RB should step to the mesh first, got %+v", rb.Target)
	}
	for i := 0; i < 20; i++ {
		ps.Tick(1.0/60.0, Input{})
	}
	if ps.Mesh != MeshDone {
		t.Fatalf("mesh should complete, got %s", ps.Mesh)
	}
	rb = ps.World.Units[ps.RBIdx]
	if !rb.HasTarget || rb.Target.Y < 30+8 {
		t.Fatalf("after the mesh the RB should dive like a run, got %+v", rb.Target)
	}
}

func TestPABitesHarderWhenTheRunIsWorking(t *testing.T) {
	const los = 30.0
	script := func(id string, threat float64) float64 {
		rng := rand.New(rand.NewSource(9))
		ps := StartSnap(los, playByID(id), defByID("base"), playbook.DefaultShell(),
			rng, Fatigue{}, LineContext{RunThreat: threat, Samples: 8, RunPct: 0.5})
		for i := 0; i < 24; i++ { // ~0.40s
			if !ps.Tick(1.0/60.0, Input{DY: -0.15}) {
				break
			}
		}
		return mikeY(ps)
	}
	postY := script("post", 2.8)
	paY := script("pa_post", 2.8)
	if paY > postY-0.8 {
		t.Fatalf("PA with live run threat should crash the Mike (pa Y=%.1f post Y=%.1f)", paY, postY)
	}
	cold := script("pa_post", 0)
	if paY > cold {
		t.Fatalf("working run should not sit deeper than a cold staff (hot=%.1f cold=%.1f)", paY, cold)
	}
}

func TestImmediateThrowAbortsTheFake(t *testing.T) {
	ps := StartSnap(30, playByID("pa_post"), defByID("base"), playbook.DefaultShell(),
		rand.New(rand.NewSource(3)), Fatigue{}, LineContext{RunThreat: 2.8, Samples: 8, RunPct: 0.5})
	if ps.Mesh != MeshLive {
		t.Fatalf("expected live mesh, got %s", ps.Mesh)
	}
	ps.tryThrow()
	if ps.Mesh != MeshAbort {
		t.Fatalf("immediate tryThrow should abort the fake, got %s", ps.Mesh)
	}
	if !ps.Thrown {
		t.Fatal("abort still throws")
	}
	if ps.ReleaseAt > 0.02 {
		t.Fatalf("abort should release now, release_at=%.3f", ps.ReleaseAt)
	}
	// One more tick so any leftover bite would register.
	ps.Tick(1.0/60.0, Input{})
	if ps.BiterN != 0 {
		t.Fatalf("aborted fake must not buy bite, biters=%v", ps.BiterIDs)
	}
}

func TestBufferedThrowWaitsForMesh(t *testing.T) {
	ps := StartSnap(30, playByID("pa_post"), defByID("base"), playbook.DefaultShell(),
		rand.New(rand.NewSource(4)), Fatigue{}, LineContext{RunThreat: 2.8, Samples: 8})
	if !ps.Tick(1.0/60.0, Input{Throw: true}) {
		t.Fatal("play died on the buffer frame")
	}
	if ps.Thrown {
		t.Fatal("Space during mesh must not release immediately")
	}
	if !ps.ThrowArmed && ps.Mesh != MeshDone {
		t.Fatal("throw should be armed or mesh already complete")
	}
	for i := 0; i < 30 && !ps.Thrown; i++ {
		ps.Tick(1.0/60.0, Input{})
	}
	if !ps.Thrown {
		t.Fatal("buffered throw should release after the mesh")
	}
	if ps.Mesh != MeshDone {
		t.Fatalf("mesh should complete, got %s", ps.Mesh)
	}
	if ps.ReleaseAt < paMeshSec-0.02 {
		t.Fatalf("completed mesh should delay release (got %.3f, want >= %.2f)", ps.ReleaseAt, paMeshSec)
	}
	if ps.BiterN == 0 {
		t.Fatal("completed mesh should produce some bite")
	}
}

func TestLeftoverWindowIsEarned(t *testing.T) {
	_, coldL, coldB := computePAWindow(LineContext{Samples: 8}, "base")
	if coldL > 0.02 || math.Abs(coldB-paMeshSec) > 0.02 {
		t.Fatalf("cold leftover should be 0 (left=%.2f bite=%.2f mesh=%.2f)", coldL, coldB, paMeshSec)
	}
	_, warmL, _ := computePAWindow(LineContext{RunThreat: 1.6, Samples: 8, RunPct: 0.5}, "base")
	if warmL < 0.20 {
		t.Fatalf("earned run should buy leftover after the mesh, got %.2f", warmL)
	}
	_, hotL, hotB := computePAWindow(LineContext{RunThreat: 2.8, Samples: 8, RunPct: 0.5}, "base")
	if hotL <= warmL || hotB <= paMeshSec+0.20 {
		t.Fatalf("hot leftover should beat warm (hot=%.2f warm=%.2f bite=%.2f)", hotL, warmL, hotB)
	}
	_, sellL, _ := computePAWindow(LineContext{RunThreat: 2.8, PassThreat: 2.5, Samples: 8, PassPct: 0.8}, "base")
	if sellL >= hotL {
		t.Fatalf("pass-sell should shave leftover (hot=%.2f sell=%.2f)", hotL, sellL)
	}
	if sellL < paLiveLeftMin-0.01 {
		t.Fatalf("live run must keep a hittable leftover under pass-sell (got %.2f)", sellL)
	}
	_, crushL, _ := computePAWindow(LineContext{RunThreat: 0, PassThreat: 2.5, Samples: 8, PassPct: 0.8}, "run_fit")
	if crushL >= 0.08 {
		t.Fatalf("unearned leftover may still flatten (got %.2f)", crushL)
	}
	_, fitL, _ := computePAWindow(LineContext{RunThreat: 1.6, Samples: 8}, "run_fit")
	if fitL <= warmL {
		t.Fatalf("run-fit should add leftover (fit=%.2f warm=%.2f)", fitL, warmL)
	}
}

func TestPassSellReducesBite(t *testing.T) {
	hot := computeBiteSec(LineContext{RunThreat: 2.8, Samples: 8, RunPct: 0.5}, "base")
	sell := computeBiteSec(LineContext{RunThreat: 2.8, PassThreat: 2.5, Samples: 8, PassPct: 0.8}, "base")
	if sell >= hot {
		t.Fatalf("pass-sell should cut bite (hot=%.2f sell=%.2f)", hot, sell)
	}
	fit := computeBiteSec(LineContext{RunThreat: 2.8, Samples: 8}, "run_fit")
	if fit <= hot {
		t.Fatalf("run-fit should add bite (hot=%.2f fit=%.2f)", hot, fit)
	}

	script := func(passThreat float64) float64 {
		ps := StartSnap(30, playByID("pa_post"), defByID("base"), playbook.DefaultShell(),
			rand.New(rand.NewSource(5)), Fatigue{},
			LineContext{RunThreat: 2.8, PassThreat: passThreat, Samples: 8, PassPct: 0.7})
		for i := 0; i < 20; i++ {
			ps.Tick(1.0/60.0, Input{})
		}
		return mikeY(ps)
	}
	if script(2.5) < script(0)-0.15 {
		t.Fatal("pass-sell should not crash the Mike harder than a cold pass staff")
	}
}

func TestSpyAndRushersNeverBite(t *testing.T) {
	ps := StartSnap(30, playByID("pa_post"), defByID("blitz"), playbook.DefaultShell(),
		rand.New(rand.NewSource(6)), Fatigue{},
		LineContext{RunThreat: 2.8, KeepThreat: 2.2, Samples: 8, RunPct: 0.5})
	var spy, free int
	for i, u := range ps.World.Units {
		if u.Spy {
			spy = i
		}
		if u.RushFree {
			free = i
		}
	}
	if spy == 0 && SpyIndex(ps.World) < 0 {
		t.Fatal("expected a spy on this staff")
	}
	if free == 0 {
		// blitz always marks a Mike
		for i, u := range ps.World.Units {
			if u.RushFree {
				free = i
				break
			}
		}
	}
	for i := 0; i < 22; i++ {
		ps.Tick(1.0/60.0, Input{})
	}
	for _, id := range ps.BiterIDs {
		if strings.HasPrefix(id, "DL#") {
			t.Fatalf("DL bit: %v", ps.BiterIDs)
		}
	}
	if spyIdx := SpyIndex(ps.World); spyIdx >= 0 {
		want := fmt.Sprintf("LB#%d", ps.World.Units[spyIdx].ID)
		alt := fmt.Sprintf("S#%d", ps.World.Units[spyIdx].ID)
		for _, id := range ps.BiterIDs {
			if id == want || id == alt {
				t.Fatalf("spy bit: %v", ps.BiterIDs)
			}
		}
	}
	if free >= 0 && free < len(ps.World.Units) {
		want := fmt.Sprintf("LB#%d", ps.World.Units[free].ID)
		for _, id := range ps.BiterIDs {
			if id == want {
				t.Fatalf("free rusher bit: %v", ps.BiterIDs)
			}
		}
	}
}

func TestDeepHelpStaysOverThePost(t *testing.T) {
	ps := StartSnap(30, playByID("pa_post"), defByID("base"), playbook.DefaultShell(),
		rand.New(rand.NewSource(7)), Fatigue{}, LineContext{RunThreat: 2.8, Samples: 8})
	for i := 0; i < 40 && !ps.Thrown; i++ {
		in := Input{}
		if i == 5 {
			in.Throw = true
		}
		if !ps.Tick(1.0/60.0, in) {
			break
		}
	}
	if ps.PrimaryIdx < 0 {
		t.Fatal("no primary")
	}
	prY := ps.World.Units[ps.PrimaryIdx].Pos.Y
	deep := 0
	for _, u := range ps.World.Units {
		if u.Side != SideDefense || !u.CoverJob.IsDeep() {
			continue
		}
		if u.Pos.Y >= prY-1.5 || u.Pos.Y >= ps.LOS+12 {
			deep++
		}
	}
	if deep < 1 {
		t.Fatalf("need a deep defender over the post (primary Y=%.1f)", prY)
	}
}

func TestCoverageRecoversAfterTheFake(t *testing.T) {
	ps := StartSnap(30, playByID("pa_post"), defByID("base"), playbook.DefaultShell(),
		rand.New(rand.NewSource(8)), Fatigue{}, LineContext{RunThreat: 2.8, Samples: 8})
	// Through the bite, then recover.
	n := int((ps.BiteSec + 0.55) * 60)
	for i := 0; i < n; i++ {
		if !ps.Tick(1.0/60.0, Input{DY: -0.2}) {
			t.Fatal("play died before recovery")
		}
	}
	if ps.Mesh != MeshDone {
		t.Fatalf("expected completed mesh, got %s", ps.Mesh)
	}
	hookLand, hookNow := 0.0, 0.0
	found := false
	for _, u := range ps.World.Units {
		if u.Side != SideDefense || u.CoverJob != CoverHook {
			continue
		}
		hookLand = u.CoverLand.Y
		hookNow = u.Pos.Y
		found = true
		break
	}
	if !found {
		t.Fatal("no hook defender")
	}
	// Recovered: closer to the landmark than to the crash point (LOS+1.2).
	if math.Abs(hookNow-hookLand) > math.Abs(hookNow-(ps.LOS+1.2))+0.4 {
		t.Fatalf("hook should recover after the fake (now=%.1f land=%.1f crash=%.1f)",
			hookNow, hookLand, ps.LOS+1.2)
	}
}

func TestHotPAWindowBeatsCold(t *testing.T) {
	run := func(threat float64) float64 {
		ps := StartSnap(30, playByID("pa_post"), defByID("base"), playbook.DefaultShell(),
			rand.New(rand.NewSource(10)), Fatigue{},
			LineContext{RunThreat: threat, Samples: 8, RunPct: 0.55})
		// Sample in the leftover: after mesh, before hot bite ends.
		n := int(math.Round((paMeshSec + 0.16) * 60))
		for i := 0; i < n; i++ {
			if !ps.Tick(1.0/60.0, Input{}) {
				break
			}
		}
		return mikeY(ps)
	}
	hotMike := run(2.8)
	coldMike := run(0)
	if hotMike > coldMike-0.4 {
		t.Fatalf("hot leftover should crash Mike more (hot Y=%.1f cold Y=%.1f)", hotMike, coldMike)
	}
}

func TestHotLeftoverCrashesAfterMesh(t *testing.T) {
	hot := StartSnap(30, playByID("pa_post"), defByID("base"), playbook.DefaultShell(),
		rand.New(rand.NewSource(12)), Fatigue{},
		LineContext{RunThreat: 2.8, Samples: 8, RunPct: 0.55})
	if hot.LeftoverSec < 0.20 {
		t.Fatalf("hot snap should have leftover, got %.2f", hot.LeftoverSec)
	}
	n := int(math.Round((hot.MeshSec + hot.LeftoverSec*0.55) * 60))
	for i := 0; i < n; i++ {
		if !hot.Tick(1.0/60.0, Input{DY: -0.1}) {
			t.Fatal("hot PA died in the leftover window")
		}
	}
	if hot.Mesh != MeshDone {
		t.Fatalf("mesh should be done, got %s", hot.Mesh)
	}
	if !hot.InLeftoverWindow() {
		t.Fatalf("should still be in leftover (elapsed=%.2f bite=%.2f left=%.2f)",
			hot.Elapsed, hot.BiteSec, hot.LeftoverSec)
	}

	cold := StartSnap(30, playByID("pa_post"), defByID("base"), playbook.DefaultShell(),
		rand.New(rand.NewSource(12)), Fatigue{}, LineContext{Samples: 8})
	for i := 0; i < n; i++ {
		cold.Tick(1.0/60.0, Input{DY: -0.1})
	}
	if cold.InLeftoverWindow() {
		t.Fatal("cold PA must not have a leftover window")
	}
	if mikeY(hot) > mikeY(cold)-0.5 {
		t.Fatalf("hot leftover Mike should still be crashed (hot=%.1f cold=%.1f)", mikeY(hot), mikeY(cold))
	}
}

func TestAbortHasNoLeftoverCrash(t *testing.T) {
	hold := StartSnap(30, playByID("pa_post"), defByID("base"), playbook.DefaultShell(),
		rand.New(rand.NewSource(13)), Fatigue{},
		LineContext{RunThreat: 2.8, Samples: 8, RunPct: 0.55})
	abort := StartSnap(30, playByID("pa_post"), defByID("base"), playbook.DefaultShell(),
		rand.New(rand.NewSource(13)), Fatigue{},
		LineContext{RunThreat: 2.8, Samples: 8, RunPct: 0.55})
	abort.Tick(1.0/60.0, Input{Juke: true})
	if abort.Mesh != MeshAbort {
		t.Fatalf("Shift during mesh should abort, got %s", abort.Mesh)
	}
	n := int(math.Round((paMeshSec + 0.20) * 60))
	for i := 0; i < n; i++ {
		hold.Tick(1.0/60.0, Input{DY: -0.1})
		abort.Tick(1.0/60.0, Input{DY: -0.1})
	}
	if abort.InLeftoverWindow() {
		t.Fatal("aborted fake must not keep leftover bite")
	}
	if mikeY(hold) > mikeY(abort)-0.35 {
		t.Fatalf("abort should not keep crashing (hold=%.1f abort=%.1f)", mikeY(hold), mikeY(abort))
	}
}

func TestHoldingPACostsPressure(t *testing.T) {
	closest := func(id string) (dist float64, sacked bool) {
		ps := StartSnap(30, playByID(id), defByID("pass_rush"), playbook.DefaultShell(),
			rand.New(rand.NewSource(11)), Fatigue{}, LineContext{Samples: 4})
		for i := 0; i < 70; i++ {
			if !ps.Tick(1.0/60.0, Input{DY: -0.25}) {
				return 0, ps.Result.Outcome == OutcomeSack
			}
		}
		if ps.QBIdx < 0 {
			t.Fatal("no QB")
		}
		qb := ps.World.Units[ps.QBIdx].Pos
		best := 1e9
		for _, u := range ps.World.Units {
			if u.Side != SideDefense || (u.Role != RoleDL && !u.RushFree) {
				continue
			}
			d := math.Hypot(u.Pos.X-qb.X, u.Pos.Y-qb.Y)
			if d < best {
				best = d
			}
		}
		return best, false
	}
	paDist, paSack := closest("pa_post")
	postDist, postSack := closest("post")
	if paSack && !postSack {
		return
	}
	if postSack && !paSack {
		t.Fatal("holding PA should not be safer than holding Post vs pass rush")
	}
	if paDist > postDist+0.35 {
		t.Fatalf("holding PA should bring rushers closer (pa=%.2f post=%.2f)", paDist, postDist)
	}
}

func TestPAGlanceReusesTheMesh(t *testing.T) {
	play := playByID("pa_glance")
	if play.ID != "pa_glance" {
		t.Fatal("pa_glance missing from the book")
	}
	const los = 30.0
	ps := StartSnap(los, play, defByID("base"), playbook.DefaultShell(),
		rand.New(rand.NewSource(2)), Fatigue{}, LineContext{RunThreat: 2.0, Samples: 8})
	if !ps.PlayAction || ps.Mesh != MeshLive {
		t.Fatalf("glance should snap into a live mesh, playAction=%v mesh=%s", ps.PlayAction, ps.Mesh)
	}
	if ps.IsPostShot {
		t.Fatal("glance must not use post wrap")
	}
	if ps.PrimaryIdx < 0 {
		t.Fatal("no primary")
	}
	pr := ps.World.Units[ps.PrimaryIdx]
	if !pr.HasTarget || pr.Target.Y < los+10 || pr.Target.Y > los+12.5 {
		t.Fatalf("glance primary should sit 10–12 past the LOS, got %+v", pr.Target)
	}
	if math.Abs(pr.Target.X-field.HashMid) < 3.5 {
		t.Fatalf("glance should sit beside the A-gap, not on Mike, got %+v", pr.Target)
	}
	if ps.LeftoverSec < 0.19 {
		t.Fatalf("hot glance should earn leftover, got %.2f", ps.LeftoverSec)
	}
}

func TestPAGlanceSitsBehindTheMike(t *testing.T) {
	const los = 30.0
	ps := StartSnap(los, playByID("pa_glance"), defByID("base"), playbook.DefaultShell(),
		rand.New(rand.NewSource(15)), Fatigue{},
		LineContext{RunThreat: 2.8, Samples: 8, RunPct: 0.55})
	until := ps.MeshSec + ps.LeftoverSec*0.5
	for i := 0; i < 40; i++ {
		if ps.Elapsed >= until-1e-6 {
			break
		}
		if !ps.Tick(1.0/60.0, Input{}) {
			t.Fatal("glance died before leftover")
		}
	}
	if ps.PrimaryIdx < 0 {
		t.Fatal("no primary")
	}
	pr := ps.World.Units[ps.PrimaryIdx].Pos
	mike := mikeY(ps)
	if pr.Y < los+4.0 {
		t.Fatalf("glance should be downfield at leftover, Y=%.1f los=%.1f", pr.Y, los)
	}
	// Beside the crash, not through the A-gap. Depth vs Mike can be close
	// at leftover start; wrap + leftover vacancy is the gain, not a 10-yard lead.
	if math.Abs(pr.X-field.HashMid) < 3 {
		t.Fatalf("glance ran into the A-gap (x=%.1f mid=%.1f mikeY=%.1f prY=%.1f)", pr.X, field.HashMid, mike, pr.Y)
	}
}

func TestPAGlanceKeepsDeepHelp(t *testing.T) {
	const los = 30.0
	for _, sh := range playbook.DefaultShells() {
		ps := StartSnap(los, playByID("pa_glance"), defByID("base"), sh,
			rand.New(rand.NewSource(2)), Fatigue{}, LineContext{})
		assertShellInvariants(t, ps.World, sh, los)
	}
}

func TestPAGlanceInWindowBeatsCold(t *testing.T) {
	resolve := func(threat float64) (yards, sep, mike float64) {
		ps := StartSnap(30, playByID("pa_glance"), defByID("base"), playbook.DefaultShell(),
			rand.New(rand.NewSource(14)), Fatigue{},
			LineContext{RunThreat: threat, Samples: 8, RunPct: 0.55})
		until := ps.MeshSec + 0.12
		if ps.LeftoverSec > 0.1 {
			until = ps.MeshSec + ps.LeftoverSec*0.45
		}
		for i := 0; i < 40; i++ {
			if ps.Elapsed >= until-1e-6 {
				break
			}
			if !ps.Tick(1.0/60.0, Input{}) {
				break
			}
		}
		ps.Tick(1.0/60.0, Input{Throw: true})
		mike = mikeY(ps)
		sep = ps.SepAtThrow
		for i := 0; i < 180 && ps.Alive; i++ {
			ps.Tick(1.0/60.0, Input{})
		}
		return ps.Result.YardsGained, sep, mike
	}
	hotY, hotSep, hotMike := resolve(2.8)
	coldY, coldSep, coldMike := resolve(0)
	if hotMike > coldMike-0.35 {
		t.Fatalf("hot glance leftover should still crash Mike (hot=%.1f cold=%.1f)", hotMike, coldMike)
	}
	if hotY+1.5 < coldY && hotSep+0.4 < coldSep {
		t.Fatalf("in-window glance should not lose to cold (hot y=%.1f sep=%.1f / cold y=%.1f sep=%.1f)",
			hotY, hotSep, coldY, coldSep)
	}
	if hotY < coldY+1.0 {
		t.Fatalf("leftover glance should pay more than cold (hot=%.1f cold=%.1f)", hotY, coldY)
	}
}

func TestPAGlanceColdIsNotAHouse(t *testing.T) {
	var yards []float64
	for seed := int64(20); seed < 32; seed++ {
		ps := StartSnap(30, playByID("pa_glance"), defByID("base"), playbook.DefaultShell(),
			rand.New(rand.NewSource(seed)), Fatigue{}, LineContext{Samples: 8})
		for i := 0; i < 28; i++ { // ~0.47s — leftover timing, no leftover
			if i == 26 {
				ps.Tick(1.0/60.0, Input{Throw: true})
				continue
			}
			if !ps.Tick(1.0/60.0, Input{}) {
				break
			}
		}
		for i := 0; i < 180 && ps.Alive; i++ {
			ps.Tick(1.0/60.0, Input{})
		}
		if ps.Result.Outcome == OutcomeIncomplete || ps.Result.Outcome == OutcomeSack {
			continue
		}
		yards = append(yards, ps.Result.YardsGained)
	}
	if len(yards) < 4 {
		t.Fatalf("need some cold glance catches, got %d", len(yards))
	}
	sum := 0.0
	for _, y := range yards {
		sum += y
		if y >= 12.5 {
			t.Fatalf("cold glance should not be a 12-yard button (got %.1f in %v)", y, yards)
		}
	}
	avg := sum / float64(len(yards))
	if avg > 9.5 {
		t.Fatalf("cold glance avg should sit ~6–8, got %.1f %v", avg, yards)
	}
}

func TestPAGlanceSpyAndRushersNeverBite(t *testing.T) {
	ps := StartSnap(30, playByID("pa_glance"), defByID("blitz"), playbook.DefaultShell(),
		rand.New(rand.NewSource(6)), Fatigue{},
		LineContext{RunThreat: 2.8, KeepThreat: 2.2, Samples: 8, RunPct: 0.5})
	for i := 0; i < 22; i++ {
		ps.Tick(1.0/60.0, Input{})
	}
	for _, id := range ps.BiterIDs {
		if strings.HasPrefix(id, "DL#") {
			t.Fatalf("DL bit glance: %v", ps.BiterIDs)
		}
	}
	if spyIdx := SpyIndex(ps.World); spyIdx >= 0 {
		want := fmt.Sprintf("LB#%d", ps.World.Units[spyIdx].ID)
		alt := fmt.Sprintf("S#%d", ps.World.Units[spyIdx].ID)
		for _, id := range ps.BiterIDs {
			if id == want || id == alt {
				t.Fatalf("spy bit glance: %v", ps.BiterIDs)
			}
		}
	}
}

func mikeY(ps *PlayState) float64 {
	best := 1e9
	y := 0.0
	for _, u := range ps.World.Units {
		if u.Side != SideDefense || u.Role != RoleLB {
			continue
		}
		d := math.Abs(u.Pos.X - field.HashMid)
		if d < best {
			best = d
			y = u.Pos.Y
		}
	}
	return y
}
