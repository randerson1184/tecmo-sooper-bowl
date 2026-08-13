package ai

import (
	"math/rand"
	"testing"

	"github.com/randerson1184/tecmo-sooper-bowl/internal/game"
	"github.com/randerson1184/tecmo-sooper-bowl/internal/playbook"
)

func TestTrackerRunHeavyBiasesRunFit(t *testing.T) {
	tr := NewTracker(16)
	for i := 0; i < 12; i++ {
		tr.Observe(PlayObservation{
			Type:      playbook.PlayRun,
			Side:      playbook.SideMiddle,
			Situation: game.SitNormal,
		})
	}
	snap := tr.Snapshot()
	if snap.RunPct < 0.9 {
		t.Fatalf("expected run-heavy snapshot, got %+v", snap)
	}

	// Many rolls should favor run_fit more than soft_zone under run spam.
	rng := rand.New(rand.NewSource(1))
	counts := map[string]int{}
	for i := 0; i < 200; i++ {
		c := ChooseDefense(game.SitNormal, snap, rng)
		counts[c.ID]++
	}
	if counts["run_fit"] < counts["soft_zone"] {
		t.Fatalf("expected run_fit >= soft_zone under run spam; counts=%v", counts)
	}
}
