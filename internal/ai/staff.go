package ai

import (
	"math/rand"

	"github.com/randerson1184/tecmo-sooper-bowl/internal/game"
	"github.com/randerson1184/tecmo-sooper-bowl/internal/playbook"
)

// Staff is the defensive coordinator. Disguise is 0..1:
// poor teams never lie; elite teams lie often. This roster is the baseline.
type Staff struct {
	Name     string
	Disguise float64
}

func DefaultStaff() Staff {
	return Staff{Name: "baseline", Disguise: 0.40}
}

func PoorStaff() Staff {
	return Staff{Name: "poor", Disguise: 0}
}

func EliteStaff() Staff {
	return Staff{Name: "elite", Disguise: 1}
}

// DisguiseChance is P(this snap shows a fake picture). 0 at quality 0, ~30% at 1.
func (s Staff) DisguiseChance() float64 {
	if s.Disguise <= 0 {
		return 0
	}
	q := s.Disguise
	if q > 1 {
		q = 1
	}
	return 0.06 + q*0.24
}

// ChooseStaffPackage is ChoosePackage with an explicit staff.
func ChooseStaffPackage(sit game.SituationClass, snap Snapshot, staff Staff, rng *rand.Rand) playbook.Package {
	if rng == nil {
		rng = rand.New(rand.NewSource(1))
	}
	pkg := playbook.Package{
		Front: ChooseDefense(sit, snap, rng),
		Shell: ChooseShell(sit, snap, rng),
	}
	pkg.Look = pkg.Shell
	if rng.Float64() < staff.DisguiseChance() {
		if look := playbook.DisguiseAs(pkg.Shell); look.ID != "" && look.ID != pkg.Shell.ID {
			pkg.Look = look
			pkg.Disguised = true
		}
	}
	return pkg
}
