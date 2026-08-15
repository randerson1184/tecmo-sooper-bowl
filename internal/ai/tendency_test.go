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

func TestHitchDietPrefersCover2(t *testing.T) {
	tr := NewTracker(16)
	for i := 0; i < 12; i++ {
		tr.Observe(PlayObservation{
			Type:      playbook.PlayPass,
			Side:      playbook.SideRight,
			Situation: game.SitNormal,
		})
	}
	snap := tr.Snapshot()
	rng := rand.New(rand.NewSource(2))
	counts := map[string]int{}
	for i := 0; i < 200; i++ {
		s := ChooseShell(game.SitNormal, snap, rng)
		counts[s.ID]++
	}
	if counts[playbook.ShellCover2] < counts[playbook.ShellCover3] {
		t.Fatalf("hitch diet should prefer Cover 2 over Cover 3; counts=%v", counts)
	}
}

func TestThreatUsesSuccessNotFrequency(t *testing.T) {
	stuffed := NewTracker(12)
	for i := 0; i < 8; i++ {
		stuffed.Observe(PlayObservation{Type: playbook.PlayRun, Yards: 0.5, Outcome: "tackle"})
	}
	if stuffed.Snapshot().RunThreat >= 1 {
		t.Fatalf("eight stuffs should not create run threat: %+v", stuffed.Snapshot())
	}
	if stuffed.Snapshot().RunPct < 0.9 {
		t.Fatalf("frequency should still show run-heavy: %+v", stuffed.Snapshot())
	}

	chunk := NewTracker(12)
	chunk.Observe(PlayObservation{Type: playbook.PlayRun, Yards: 18, Outcome: "tackle"})
	chunk.Observe(PlayObservation{Type: playbook.PlayRun, Yards: 12, Outcome: "tackle"})
	if chunk.Snapshot().RunThreat <= stuffed.Snapshot().RunThreat {
		t.Fatalf("two explosives should out-threat eight stuffs: chunk=%v stuffed=%v",
			chunk.Snapshot().RunThreat, stuffed.Snapshot().RunThreat)
	}
}

func TestKeepThreatFromSuccessfulScrambles(t *testing.T) {
	tr := NewTracker(12)
	for i := 0; i < 4; i++ {
		tr.Observe(PlayObservation{
			Type:    playbook.PlayPass,
			Yards:   40,
			Outcome: "touchdown",
			QBKeep:  true,
		})
	}
	if tr.Snapshot().KeepThreat < 2 {
		t.Fatalf("four keep TDs should raise keep threat: %+v", tr.Snapshot())
	}
	throws := NewTracker(12)
	for i := 0; i < 4; i++ {
		throws.Observe(PlayObservation{Type: playbook.PlayPass, Yards: 6, Outcome: "tackle", QBKeep: false})
	}
	if throws.Snapshot().KeepThreat != 0 {
		t.Fatalf("actual throws should not look like keeps: %+v", throws.Snapshot())
	}

	stuffed := NewTracker(12)
	for i := 0; i < 4; i++ {
		stuffed.Observe(PlayObservation{
			Type:    playbook.PlayPass,
			Yards:   -2,
			Outcome: "tackle",
			QBKeep:  true,
		})
	}
	if stuffed.Snapshot().KeepThreat != 0 {
		t.Fatalf("stuffed keeps must not raise keep threat: %+v", stuffed.Snapshot())
	}
	if stuffed.Snapshot().KeepN != 4 {
		t.Fatalf("keep count should still record the attempts: %+v", stuffed.Snapshot())
	}
}

func TestKeepDoesNotInflatePassThreat(t *testing.T) {
	tr := NewTracker(12)
	for i := 0; i < 4; i++ {
		tr.Observe(PlayObservation{
			Type:    playbook.PlayPass,
			Yards:   14,
			Outcome: "tackle",
			QBKeep:  true,
		})
	}
	s := tr.Snapshot()
	if s.PassPct < 0.9 {
		t.Fatalf("called-pass frequency should still count keeps: %+v", s)
	}
	if s.ThrowN != 0 {
		t.Fatalf("keeps are not throws: %+v", s)
	}
	if s.PassThreat != 0 || s.PassYds != 0 {
		t.Fatalf("keep yards must not raise pass threat: %+v", s)
	}
	if s.KeepN != 4 || s.KeepThreat < 2 || s.KeepYds < 50 {
		t.Fatalf("keep film should live on Keep*: %+v", s)
	}

	tr.Observe(PlayObservation{Type: playbook.PlayPass, Yards: 8, Outcome: "tackle"})
	s = tr.Snapshot()
	if s.ThrowN != 1 || s.PassThreat <= 0 {
		t.Fatalf("an actual throw should raise pass threat: %+v", s)
	}
}

func TestPassHeavyWithoutRunThreatLightsTheBox(t *testing.T) {
	snap := Snapshot{Samples: 10, PassPct: 0.8, RunPct: 0.2}
	rng := rand.New(rand.NewSource(4))
	counts := map[string]int{}
	for i := 0; i < 200; i++ {
		counts[ChooseDefense(game.SitNormal, snap, rng).ID]++
	}
	if counts["soft_zone"]+counts["pass_rush"] < counts["run_fit"]+counts["base"] {
		t.Fatalf("pass diet with no run threat should light the box; counts=%v", counts)
	}
}

func TestPassHeavyWithLiveKeepThreatStaysInTheBox(t *testing.T) {
	snap := Snapshot{Samples: 10, PassPct: 0.8, RunPct: 0.2, KeepThreat: 2.4}
	rng := rand.New(rand.NewSource(6))
	counts := map[string]int{}
	for i := 0; i < 200; i++ {
		counts[ChooseDefense(game.SitNormal, snap, rng).ID]++
	}
	if counts["soft_zone"] >= counts["base"] {
		t.Fatalf("live keep threat must veto lighting the box; counts=%v", counts)
	}
}

func TestPassHeavyWithLiveRunThreatStaysInTheBox(t *testing.T) {
	// Same pass frequency as the 77-yard sweep film, but the run already worked.
	snap := Snapshot{Samples: 10, PassPct: 0.8, RunPct: 0.2, RunThreat: 2.4}
	rng := rand.New(rand.NewSource(5))
	counts := map[string]int{}
	for i := 0; i < 200; i++ {
		counts[ChooseDefense(game.SitNormal, snap, rng).ID]++
	}
	if counts["soft_zone"] >= counts["base"] {
		t.Fatalf("live run threat must veto lighting the box; counts=%v", counts)
	}
	if counts["soft_zone"] >= counts["run_fit"] {
		t.Fatalf("live run threat should not prefer soft_zone over run_fit; counts=%v", counts)
	}
}

func TestChoosePackageSplitsFrontAndShell(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	pkg := ChoosePackage(game.SitNormal, Snapshot{}, rng)
	if pkg.Front.ID == "" || pkg.Shell.ID == "" {
		t.Fatalf("package missing half: %+v", pkg)
	}
	// Empty snap should still be a valid pair, not a fused enum.
	if pkg.Shell.ID != playbook.ShellCover3 && pkg.Shell.ID != playbook.ShellCover2 && pkg.Shell.ID != playbook.ShellManFree {
		t.Fatalf("unknown shell %q", pkg.Shell.ID)
	}
}
