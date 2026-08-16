package sim

import (
	"math"
	"math/rand"
	"testing"

	"github.com/randerson1184/tecmo-sooper-bowl/internal/field"
	"github.com/randerson1184/tecmo-sooper-bowl/internal/playbook"
)

func defByID(id string) playbook.DefenseCall {
	for _, d := range playbook.DefaultDefenseCalls() {
		if d.ID == id {
			return d
		}
	}
	return playbook.DefaultDefenseCalls()[0]
}

func playByID(id string) playbook.Play {
	for _, p := range playbook.DefaultOffense() {
		if p.ID == id {
			return p
		}
	}
	return playbook.DefaultOffense()[0]
}

// Scripted “average player” inputs for balance sampling.
func samplePlay(playID, defID string, n int, drive Input, throwAt float64) (good int, yards []float64) {
	play := playByID(playID)
	def := defByID(defID)
	for seed := int64(0); seed < int64(n); seed++ {
		rng := rand.New(rand.NewSource(seed + 99))
		ps := StartPlay(30, play, def, rng, Fatigue{})
		for i := 0; i < 60*10; i++ {
			t := float64(i) / 60.0
			in := drive
			if play.Type == playbook.PlayPass && throwAt >= 0 && t >= throwAt && t < throwAt+0.05 {
				in.Throw = true
			}
			if !ps.Tick(1.0/60.0, in) {
				break
			}
		}
		y := ps.Result.YardsGained
		yards = append(yards, y)
		switch play.Type {
		case playbook.PlayRun:
			if y >= 3 {
				good++
			}
		case playbook.PlayPass:
			// Catch + some YAC, or pure completion with 0+ (incomplete = 0 and OutcomeIncomplete)
			if ps.Result.Outcome != OutcomeIncomplete && ps.Result.Outcome != OutcomeSack {
				if y >= 0 && ps.Result.Outcome != OutcomeNone {
					good++
				}
			}
			// Count catch specifically
			if ps.Result.Outcome == OutcomeTackle || ps.Result.Outcome == OutcomeOutOfBounds ||
				ps.Result.Outcome == OutcomeTouchdown {
				// already good if yards ok
			}
		}
	}
	return good, yards
}

func TestSweepOftenGains(t *testing.T) {
	// vs Base: stretch should still hit chunk gains sometimes
	good, _ := samplePlay("sweep", "base", 30, Input{DX: 0.4, DY: 1}, -1)
	if good < 8 {
		t.Fatalf("sweep vs base: want 3+ yards often enough, got %d/30", good)
	}
}

func TestSweepNotUnstoppableVsRunFit(t *testing.T) {
	// vs Run Fit: should not be free — many plays under 4 yards
	play := playByID("sweep")
	def := defByID("run_fit")
	stuffed := 0
	chunks := 0
	for seed := int64(0); seed < 30; seed++ {
		rng := rand.New(rand.NewSource(seed + 50))
		ps := StartPlay(30, play, def, rng, Fatigue{})
		for i := 0; i < 60*8; i++ {
			if !ps.Tick(1.0/60.0, Input{DX: 0.4, DY: 1}) {
				break
			}
		}
		if ps.Result.YardsGained < 4 {
			stuffed++
		}
		if ps.Result.YardsGained >= 10 {
			chunks++
		}
	}
	if stuffed < 10 {
		t.Fatalf("sweep vs run_fit should be stuffable; only %d/30 under 4 yds", stuffed)
	}
	if chunks > 18 {
		t.Fatalf("sweep vs run_fit still too cheesy: %d/30 were 10+ yd chunks", chunks)
	}
}

func TestEveryFrontSetsSweepContain(t *testing.T) {
	play := playByID("sweep")
	for _, def := range playbook.DefaultDefenseCalls() {
		ps := StartPlay(30, play, def, rand.New(rand.NewSource(1)), Fatigue{})
		idx := SweepContainIndex(ps.World)
		if idx < 0 {
			t.Fatalf("%s: no contain player", def.ID)
		}
		u := ps.World.Units[idx]
		if u.Pos.X < field.HashMid+8 {
			t.Fatalf("%s contain too inside: x=%.1f", def.ID, u.Pos.X)
		}
		for _, o := range ps.World.Units {
			if o.Side == SideOffense && o.BlockTarget == idx {
				t.Fatalf("%s contain still blocked by an OL", def.ID)
			}
		}
	}
}

func TestSweepVsBaseIsNotAHouseCall(t *testing.T) {
	play := playByID("sweep")
	def := defByID("base")
	house, stuffed := 0, 0
	const n = 30
	for seed := int64(0); seed < n; seed++ {
		rng := rand.New(rand.NewSource(seed + 50))
		ps := StartPlay(30, play, def, rng, Fatigue{})
		for i := 0; i < 60*8; i++ {
			if !ps.Tick(1.0/60.0, Input{DX: 0.4, DY: 1}) {
				break
			}
		}
		if ps.Result.YardsGained >= 20 {
			house++
		}
		if ps.Result.YardsGained < 4 {
			stuffed++
		}
	}
	if house > 12 {
		t.Fatalf("sweep vs base still a house call: %d/%d were 20+", house, n)
	}
	if stuffed < 6 {
		t.Fatalf("base must set the edge sometimes; only %d/%d under 4 yds", stuffed, n)
	}
}

func TestSweepVsBaseCover2IsNotAHouseCall(t *testing.T) {
	// The leak: Base + Cover 2 squat corners left the edge empty (~+70 on film).
	play := playByID("sweep")
	def := defByID("base")
	shell := playbook.ShellByID(playbook.ShellCover2)
	house, stuffed := 0, 0
	const n = 30
	for seed := int64(0); seed < n; seed++ {
		rng := rand.New(rand.NewSource(seed + 77))
		ps := StartSnap(30, play, def, shell, rng, Fatigue{}, LineContext{})
		if idx := SweepContainIndex(ps.World); idx < 0 {
			t.Fatal("base+cover2 missing contain")
		}
		for i := 0; i < 60*8; i++ {
			if !ps.Tick(1.0/60.0, Input{DX: 0.4, DY: 1}) {
				break
			}
		}
		if ps.Result.YardsGained >= 20 {
			house++
		}
		if ps.Result.YardsGained < 4 {
			stuffed++
		}
	}
	if house > 10 {
		t.Fatalf("sweep vs base+cover2 still a house call: %d/%d were 20+", house, n)
	}
	if stuffed < 5 {
		t.Fatalf("C2 must force the stretch sometimes; only %d/%d under 4 yds", stuffed, n)
	}
}

func TestLightBoxContainIsNotAWideGhost(t *testing.T) {
	play := playByID("sweep")
	for _, id := range []string{"soft_zone", "pass_rush"} {
		ps := StartPlay(30, play, defByID(id), rand.New(rand.NewSource(3)), Fatigue{})
		idx := SweepContainIndex(ps.World)
		if idx < 0 {
			t.Fatalf("%s: no contain", id)
		}
		alley := ps.World.Units[idx].Pos.X - field.HashMid
		if alley > 17.0 {
			t.Fatalf("%s contain too wide to force: alley=%.1f", id, alley)
		}
		if alley < 10.0 {
			t.Fatalf("%s contain pinched like Run Fit: alley=%.1f", id, alley)
		}
	}
}

func TestSweepVsSoftZoneIsNotAHouseCall(t *testing.T) {
	// Film: pass diet → Soft Zone, then sweep for 35–77. Correct read, wrong magnitude.
	play := playByID("sweep")
	def := defByID("soft_zone")
	house := 0
	const n = 30
	for seed := int64(0); seed < n; seed++ {
		rng := rand.New(rand.NewSource(seed + 81))
		ps := StartPlay(30, play, def, rng, Fatigue{})
		for i := 0; i < 60*8; i++ {
			if !ps.Tick(1.0/60.0, Input{DX: 0.4, DY: 1}) {
				break
			}
		}
		if ps.Result.YardsGained >= 20 {
			house++
		}
	}
	if house > 10 {
		t.Fatalf("sweep vs soft_zone still a house call: %d/%d were 20+", house, n)
	}
}

func TestSweepVsSoftZoneManFreeIsNotAHouseCall(t *testing.T) {
	play := playByID("sweep")
	def := defByID("soft_zone")
	shell := playbook.ShellByID(playbook.ShellManFree)
	house := 0
	const n = 30
	for seed := int64(0); seed < n; seed++ {
		rng := rand.New(rand.NewSource(seed + 88))
		ps := StartSnap(30, play, def, shell, rng, Fatigue{}, LineContext{})
		for i := 0; i < 60*8; i++ {
			if !ps.Tick(1.0/60.0, Input{DX: 0.4, DY: 1}) {
				break
			}
		}
		if ps.Result.YardsGained >= 20 {
			house++
		}
	}
	if house > 10 {
		t.Fatalf("sweep vs soft_zone+man_free still a house call: %d/%d were 20+", house, n)
	}
}

func TestSweepVsPassRushHasAnEdge(t *testing.T) {
	play := playByID("sweep")
	def := defByID("pass_rush")
	house := 0
	const n = 25
	for seed := int64(0); seed < n; seed++ {
		rng := rand.New(rand.NewSource(seed + 9))
		ps := StartPlay(30, play, def, rng, Fatigue{})
		if SweepContainIndex(ps.World) < 0 {
			t.Fatal("pass rush missing contain")
		}
		for i := 0; i < 60*8; i++ {
			if !ps.Tick(1.0/60.0, Input{DX: 0.4, DY: 1}) {
				break
			}
		}
		if ps.Result.YardsGained >= 25 {
			house++
		}
	}
	if house > 10 {
		t.Fatalf("sweep vs pass rush still too free: %d/%d were 25+", house, n)
	}
}

func TestContainPursuitUsesStoredForceX(t *testing.T) {
	play := playByID("sweep")
	def := defByID("pass_rush")
	shell := playbook.ShellByID(playbook.ShellManFree)
	ps := StartSnap(30, play, def, shell, rand.New(rand.NewSource(6)), Fatigue{}, LineContext{})
	idx := SweepContainIndex(ps.World)
	if idx < 0 {
		t.Fatal("missing contain")
	}
	want := ps.World.Units[idx].ContainForceX
	if want < field.HashMid+10 || want > field.HashMid+15.5 {
		t.Fatalf("stored forceX not a tight alley: %.1f", want)
	}
	// Live a few ticks so pursuit runs.
	for i := 0; i < 25; i++ {
		if !ps.Tick(1.0/60.0, Input{DX: 0.4, DY: 1}) {
			break
		}
	}
	got := ps.World.Units[idx].Pos.X
	loose := field.HashMid + 17.5
	if math.Abs(got-loose) < math.Abs(got-want) {
		t.Fatalf("pursuit chased the old loose alley (x=%.1f want=%.1f loose=%.1f)", got, want, loose)
	}
}

func TestCover3PassRushContainIsNearTheLOS(t *testing.T) {
	// Film: Pass Rush / Cover 3 picked a bailed deep-third CB and gave +45.
	play := playByID("sweep")
	def := defByID("pass_rush")
	shell := playbook.ShellByID(playbook.ShellCover3)
	ps := StartSnap(30, play, def, shell, rand.New(rand.NewSource(4)), Fatigue{}, LineContext{})
	idx := SweepContainIndex(ps.World)
	if idx < 0 {
		t.Fatal("pass_rush+cover3 missing contain")
	}
	u := ps.World.Units[idx]
	if u.CoverJob.IsDeep() {
		t.Fatalf("deep third cannot be the alley player: job=%s y=%.1f", u.CoverJob, u.Pos.Y)
	}
	if u.Pos.Y > 30+8 {
		t.Fatalf("contain still too deep to force: y=%.1f", u.Pos.Y)
	}
}

func TestSweepVsPassRushCover3IsNotAHouseCall(t *testing.T) {
	play := playByID("sweep")
	def := defByID("pass_rush")
	shell := playbook.ShellByID(playbook.ShellCover3)
	house := 0
	const n = 30
	for seed := int64(0); seed < n; seed++ {
		rng := rand.New(rand.NewSource(seed + 63))
		ps := StartSnap(30, play, def, shell, rng, Fatigue{}, LineContext{})
		for i := 0; i < 60*8; i++ {
			if !ps.Tick(1.0/60.0, Input{DX: 0.4, DY: 1}) {
				break
			}
		}
		if ps.Result.YardsGained >= 20 {
			house++
		}
	}
	if house > 8 {
		t.Fatalf("sweep vs pass_rush+cover3 still a house call: %d/%d were 20+", house, n)
	}
}

func TestBlitzSweepIsNotAHouseCall(t *testing.T) {
	play := playByID("sweep")
	def := defByID("blitz")
	house := 0
	const n = 30
	for seed := int64(0); seed < n; seed++ {
		rng := rand.New(rand.NewSource(seed + 19))
		ps := StartPlay(30, play, def, rng, Fatigue{})
		if SweepContainIndex(ps.World) < 0 {
			t.Fatal("blitz missing contain")
		}
		alley := ps.World.Units[SweepContainIndex(ps.World)].Pos.X - field.HashMid
		if alley > 16.0 {
			t.Fatalf("blitz contain too wide: alley=%.1f", alley)
		}
		for i := 0; i < 60*8; i++ {
			if !ps.Tick(1.0/60.0, Input{DX: 0.4, DY: 1}) {
				break
			}
		}
		if ps.Result.YardsGained >= 20 {
			house++
		}
	}
	if house > 8 {
		t.Fatalf("sweep vs blitz still a house call: %d/%d were 20+", house, n)
	}
}

func TestPostVsCover2IsNotAHouseCall(t *testing.T) {
	// Film: Run Fit / Cover 2 gave +24 / +30 / +29 after the catch.
	play := playByID("post")
	def := defByID("run_fit")
	shell := playbook.ShellByID(playbook.ShellCover2)
	house, caught := 0, 0
	const n = 30
	for seed := int64(0); seed < n; seed++ {
		rng := rand.New(rand.NewSource(seed + 23))
		ps := StartSnap(30, play, def, shell, rng, Fatigue{}, LineContext{})
		for i := 0; i < 60*10; i++ {
			t := float64(i) / 60.0
			in := Input{DY: -0.2} // stay in the pocket
			if t >= 0.85 && t < 0.92 {
				in.Throw = true
			}
			if ps.Thrown && !ps.BallInAir {
				in.DY = 1 // turn up after the catch
			}
			if !ps.Tick(1.0/60.0, in) {
				break
			}
		}
		if ps.Result.Thrown && (ps.Result.Outcome == OutcomeTackle ||
			ps.Result.Outcome == OutcomeOutOfBounds ||
			ps.Result.Outcome == OutcomeTouchdown) {
			caught++
			if ps.Result.YardsGained >= 20 {
				house++
			}
		}
	}
	if caught < 6 {
		t.Fatalf("post vs cover2 should still complete sometimes; caught=%d/%d", caught, n)
	}
	if house > 6 {
		t.Fatalf("post vs cover2 still a house call: %d/%d catches were 20+", house, n)
	}
}

func TestInsideZoneVsCover2IsNotAHouseCall(t *testing.T) {
	// Film: Base / Cover 2 IZ went +62 — two-high vacated the A-gap.
	play := playByID("inside_zone")
	def := defByID("base")
	shell := playbook.ShellByID(playbook.ShellCover2)
	house, good := 0, 0
	const n = 30
	for seed := int64(0); seed < n; seed++ {
		rng := rand.New(rand.NewSource(seed + 31))
		ps := StartSnap(30, play, def, shell, rng, Fatigue{}, LineContext{})
		for i := 0; i < 60*18; i++ {
			if !ps.Tick(1.0/60.0, Input{DY: 1}) {
				break
			}
		}
		if ps.Result.YardsGained >= 2.5 {
			good++
		}
		if ps.Result.YardsGained >= 20 {
			house++
		}
	}
	if good < 8 {
		t.Fatalf("IZ vs cover2 should still gain sometimes; %d/%d were 2.5+", good, n)
	}
	if house > 6 {
		t.Fatalf("IZ vs cover2 still a house call: %d/%d were 20+", house, n)
	}
}

func TestSlantOftenCompletes(t *testing.T) {
	// Throw at ~0.55s — classic quick slant timing
	good, _ := samplePlay("slant", "base", 30, Input{DY: -0.2}, 0.55)
	// Count completions differently: not incomplete/sack
	play := playByID("slant")
	def := defByID("base")
	ok := 0
	for seed := int64(0); seed < 30; seed++ {
		rng := rand.New(rand.NewSource(seed + 7))
		ps := StartPlay(30, play, def, rng, Fatigue{})
		for i := 0; i < 60*8; i++ {
			t := float64(i) / 60.0
			in := Input{DY: -0.15}
			if t >= 0.5 && t < 0.55 {
				in.Throw = true
			}
			if !ps.Tick(1.0/60.0, in) {
				break
			}
		}
		if ps.Result.Outcome != OutcomeIncomplete && ps.Result.Outcome != OutcomeSack {
			ok++
		}
	}
	if ok < 12 {
		t.Fatalf("slant vs base: want completions often, got %d/30 (samplePlay good=%d)", ok, good)
	}
}

func TestHitchStillWorks(t *testing.T) {
	ok := 0
	play := playByID("hitch")
	def := defByID("base")
	for seed := int64(0); seed < 25; seed++ {
		rng := rand.New(rand.NewSource(seed + 3))
		ps := StartPlay(30, play, def, rng, Fatigue{})
		for i := 0; i < 60*8; i++ {
			t := float64(i) / 60.0
			in := Input{}
			if t >= 0.55 && t < 0.6 {
				in.Throw = true
			}
			if !ps.Tick(1.0/60.0, in) {
				break
			}
		}
		if ps.Result.Outcome != OutcomeIncomplete && ps.Result.Outcome != OutcomeSack {
			ok++
		}
	}
	if ok < 12 {
		t.Fatalf("hitch regressed: %d/25", ok)
	}
}

func TestSlantCompletesWithoutExploding(t *testing.T) {
	play := playByID("slant")
	def := defByID("base")
	ok, house := 0, 0
	const n = 30
	for seed := int64(0); seed < n; seed++ {
		rng := rand.New(rand.NewSource(seed + 11))
		ps := StartPlay(30, play, def, rng, Fatigue{})
		for i := 0; i < 60*8; i++ {
			t := float64(i) / 60.0
			in := Input{DY: 1}
			if t >= 0.5 && t < 0.55 {
				in.Throw = true
			}
			if !ps.Tick(1.0/60.0, in) {
				break
			}
		}
		if ps.Result.Thrown && (ps.Result.Outcome == OutcomeTackle ||
			ps.Result.Outcome == OutcomeOutOfBounds ||
			ps.Result.Outcome == OutcomeTouchdown) {
			ok++
			if ps.Result.YardsGained >= 16 {
				house++
			}
		}
	}
	if ok < 10 {
		t.Fatalf("slant should still complete often; %d/%d", ok, n)
	}
	if house > 8 {
		t.Fatalf("slant YAC still a house call: %d/%d completions were 16+", house, n)
	}
}

func TestSlantVsManFreeIsNotAHouseCall(t *testing.T) {
	play := playByID("slant")
	def := defByID("base")
	shell := playbook.ShellByID(playbook.ShellManFree)
	house := 0
	const n = 30
	for seed := int64(0); seed < n; seed++ {
		rng := rand.New(rand.NewSource(seed + 13))
		ps := StartSnap(30, play, def, shell, rng, Fatigue{}, LineContext{})
		for i := 0; i < 60*8; i++ {
			t := float64(i) / 60.0
			in := Input{DY: 1}
			if t >= 0.5 && t < 0.55 {
				in.Throw = true
			}
			if !ps.Tick(1.0/60.0, in) {
				break
			}
		}
		if ps.Result.Thrown && ps.Result.YardsGained >= 18 {
			house++
		}
	}
	if house > 8 {
		t.Fatalf("slant vs man free still a house call: %d/%d were 18+", house, n)
	}
}

func TestHitchCompletesWithoutExploding(t *testing.T) {
	play := playByID("hitch")
	def := defByID("base")
	ok, chunks := 0, 0
	const n = 25
	for seed := int64(0); seed < n; seed++ {
		rng := rand.New(rand.NewSource(seed + 3))
		ps := StartPlay(30, play, def, rng, Fatigue{})
		for i := 0; i < 60*8; i++ {
			t := float64(i) / 60.0
			in := Input{DY: 1} // turn upfield after the catch, like a human
			if t >= 0.55 && t < 0.6 {
				in.Throw = true
			}
			if !ps.Tick(1.0/60.0, in) {
				break
			}
		}
		caught := ps.Result.Outcome == OutcomeTackle ||
			ps.Result.Outcome == OutcomeOutOfBounds ||
			ps.Result.Outcome == OutcomeTouchdown
		if caught {
			ok++
		}
		if caught && ps.Result.YardsGained >= 14 {
			chunks++
		}
	}
	if ok < 10 {
		t.Fatalf("hitch should still complete often; %d/%d", ok, n)
	}
	if chunks > 10 {
		t.Fatalf("hitch still too explosive after the catch: %d/%d were 14+", chunks, n)
	}
}

func TestInsideZoneVsBaseOftenGains(t *testing.T) {
	good, _ := samplePlay("inside_zone", "base", 25, Input{DY: 1}, -1)
	if good < 9 {
		t.Fatalf("inside zone vs base should pick up 3+ yards often; %d/25", good)
	}
}

func TestInsideZoneVsRunFitStuffable(t *testing.T) {
	play := playByID("inside_zone")
	def := defByID("run_fit")
	stuffed, chunks := 0, 0
	const n = 25
	for seed := int64(0); seed < n; seed++ {
		rng := rand.New(rand.NewSource(seed + 21))
		ps := StartPlay(30, play, def, rng, Fatigue{})
		for i := 0; i < 60*8; i++ {
			if !ps.Tick(1.0/60.0, Input{DY: 1}) {
				break
			}
		}
		if ps.Result.YardsGained < 3 {
			stuffed++
		}
		if ps.Result.YardsGained >= 8 {
			chunks++
		}
	}
	if stuffed < 10 {
		t.Fatalf("inside zone vs run_fit should be stuffable; only %d/%d under 3 yds", stuffed, n)
	}
	if chunks > 10 {
		t.Fatalf("inside zone vs run_fit still chunky: %d/%d were 8+", chunks, n)
	}
}

func TestHoldingTheBallCanSack(t *testing.T) {
	play := playByID("slant")
	def := defByID("pass_rush")
	sacks := 0
	const n = 20
	for seed := int64(0); seed < n; seed++ {
		rng := rand.New(rand.NewSource(seed + 4))
		ps := StartPlay(30, play, def, rng, Fatigue{})
		for i := 0; i < 60*4; i++ {
			// Never throw — just sit
			if !ps.Tick(1.0/60.0, Input{DY: -0.2}) {
				break
			}
		}
		if ps.Result.Outcome == OutcomeSack {
			sacks++
		}
	}
	if sacks < 6 {
		t.Fatalf("holding a slant vs pass rush should sack sometimes; only %d/%d", sacks, n)
	}
}
