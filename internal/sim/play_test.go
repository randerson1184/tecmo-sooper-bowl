package sim

import (
	"math/rand"
	"testing"

	"github.com/randerson1184/tecmo-sooper-bowl/internal/playbook"
)

func TestRunPlayEventuallyEnds(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	play := playbook.DefaultOffense()[0] // inside zone
	def := playbook.DefaultDefenseCalls()[0]
	ps := StartPlay(25, play, def, rng, Fatigue{})

	in := Input{DY: 1} // always north
	for i := 0; i < 60*20; i++ {
		if !ps.Tick(1.0/60.0, in) {
			if ps.Result.Outcome == OutcomeNone {
				t.Fatal("ended with no outcome")
			}
			return
		}
	}
	t.Fatal("play did not end within 20s")
}

func TestPassThrowCanIncompleteOrCatch(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	play := playbook.DefaultOffense()[2] // slant
	def := playbook.DefaultDefenseCalls()[0]
	ps := StartPlay(30, play, def, rng, Fatigue{})

	// Drop back a bit then throw
	for i := 0; i < 20; i++ {
		ps.Tick(1.0/60.0, Input{DY: -0.2})
	}
	ps.Tick(1.0/60.0, Input{Throw: true})
	if !ps.BallInAir && ps.Alive {
		// might have been sacked already — still ok
		return
	}
	for i := 0; i < 60*8; i++ {
		if !ps.Tick(1.0/60.0, Input{}) {
			return
		}
	}
	t.Fatal("pass play did not resolve")
}
