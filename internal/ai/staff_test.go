package ai

import (
	"math/rand"
	"testing"

	"github.com/randerson1184/tecmo-sooper-bowl/internal/game"
	"github.com/randerson1184/tecmo-sooper-bowl/internal/playbook"
)

func TestPoorStaffNeverDisguises(t *testing.T) {
	rng := rand.New(rand.NewSource(9))
	n := 0
	for i := 0; i < 80; i++ {
		pkg := ChooseStaffPackage(game.SitNormal, Snapshot{Samples: 6, PassPct: 0.5}, PoorStaff(), rng)
		if pkg.Disguised {
			n++
		}
		if pkg.Look.ID != "" && pkg.Look.ID != pkg.Shell.ID && !pkg.Disguised {
			t.Fatalf("look != shell without Disguised: %+v", pkg)
		}
	}
	if n != 0 {
		t.Fatalf("poor staff must never disguise, got %d/80", n)
	}
}

func TestEliteStaffSometimesDisguises(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	n := 0
	for i := 0; i < 80; i++ {
		pkg := ChooseStaffPackage(game.SitNormal, Snapshot{Samples: 6, PassPct: 0.5}, EliteStaff(), rng)
		if pkg.Disguised {
			n++
			if pkg.Look.ID == pkg.Shell.ID {
				t.Fatalf("disguised but look==shell: %+v", pkg)
			}
		}
	}
	if n < 8 {
		t.Fatalf("elite staff should disguise sometimes, got %d/80", n)
	}
	if n > 50 {
		t.Fatalf("disguise should stay rare even for elite, got %d/80", n)
	}
}

func TestDisguiseAsFlipsThePicture(t *testing.T) {
	c2 := playbook.DisguiseAs(playbook.ShellByID(playbook.ShellCover2))
	if c2.ID != playbook.ShellCover3 {
		t.Fatalf("Cover 2 should show 1-high, got %s", c2.ID)
	}
	c3 := playbook.DisguiseAs(playbook.ShellByID(playbook.ShellCover3))
	if c3.ID != playbook.ShellCover2 {
		t.Fatalf("Cover 3 should show 2-high, got %s", c3.ID)
	}
	mf := playbook.DisguiseAs(playbook.ShellByID(playbook.ShellManFree))
	if mf.ID != playbook.ShellCover3 {
		t.Fatalf("Man Free should show off, got %s", mf.ID)
	}
}
