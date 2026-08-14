package sim

import (
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

