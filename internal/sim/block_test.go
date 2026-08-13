package sim

import (
	"math/rand"
	"testing"

	"github.com/randerson1184/tecmo-sooper-bowl/internal/playbook"
)

// Holding ↑ on inside zone against soft zone should often clear a few yards
// (regression for "O-line has no push").
func TestInsideZoneCanGainYards(t *testing.T) {
	play := playbook.DefaultOffense()[0] // inside zone
	var soft playbook.DefenseCall
	for _, d := range playbook.DefaultDefenseCalls() {
		if d.ID == "soft_zone" {
			soft = d
			break
		}
	}
	gains := 0
	const n = 25
	for seed := int64(0); seed < n; seed++ {
		rng := rand.New(rand.NewSource(seed))
		ps := StartPlay(25, play, soft, rng, Fatigue{})
		in := Input{DY: 1}
		for i := 0; i < 60*8; i++ {
			if !ps.Tick(1.0/60.0, in) {
				break
			}
		}
		if ps.Result.YardsGained >= 3 {
			gains++
		}
	}
	if gains < 10 {
		t.Fatalf("expected inside zone to gain 3+ yards often vs soft zone; only %d/%d did", gains, n)
	}
}
