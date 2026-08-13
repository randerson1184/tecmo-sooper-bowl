package sim

import (
	"math/rand"
	"testing"

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

