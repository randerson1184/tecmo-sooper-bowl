package playbook

// CoverageShell is who owns space after the snap. Independent of the front
// (Base / Run Fit / Pass Rush / Blitz). A call is Front + Shell.
type CoverageShell struct {
	ID          string
	Name        string
	Description string
}

const (
	ShellCover3  = "cover3"
	ShellCover2  = "cover2"
	ShellManFree = "man_free"
)

func ShellByID(id string) CoverageShell {
	for _, s := range DefaultShells() {
		if s.ID == id {
			return s
		}
	}
	return DefaultShells()[0]
}

func DefaultShell() CoverageShell {
	return ShellByID(ShellCover3)
}

func DefaultShells() []CoverageShell {
	return []CoverageShell{
		{ID: ShellCover3, Name: "Cover 3", Description: "Three deep; concede controlled underneath"},
		{ID: ShellCover2, Name: "Cover 2", Description: "Two deep; corners squat the flats"},
		{ID: ShellManFree, Name: "Man Free", Description: "Man under, one deep safety"},
	}
}

// Package is one defensive call: a front/pressure plus a coverage shell.
// Look is the pre-snap picture. When Disguised, Look != Shell and they rotate after the snap.
type Package struct {
	Front     DefenseCall
	Shell     CoverageShell
	Look      CoverageShell
	Disguised bool
}

func (p Package) String() string {
	if p.Disguised && p.Look.ID != "" && p.Look.ID != p.Shell.ID {
		return p.Front.Name + " / " + p.Look.Name + " → " + p.Shell.Name
	}
	return p.Front.Name + " / " + p.Shell.Name
}

func (p Package) LookOrShell() CoverageShell {
	if p.Look.ID != "" {
		return p.Look
	}
	return p.Shell
}

// DisguiseAs returns the picture this live shell fakes. Empty ID = cannot fake.
func DisguiseAs(live CoverageShell) CoverageShell {
	switch live.ID {
	case ShellCover2:
		return ShellByID(ShellCover3) // show 1-high, play 2-high
	case ShellCover3:
		return ShellByID(ShellCover2) // show 2-high, play 1-high
	case ShellManFree:
		return ShellByID(ShellCover3) // show off, play press
	default:
		return CoverageShell{}
	}
}

func (p Package) FrontID() string {
	return p.Front.ID
}

func (p Package) ShellID() string {
	return p.Shell.ID
}
