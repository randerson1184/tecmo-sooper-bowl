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
type Package struct {
	Front DefenseCall
	Shell CoverageShell
}

func (p Package) String() string {
	return p.Front.Name + " / " + p.Shell.Name
}

func (p Package) FrontID() string {
	return p.Front.ID
}

func (p Package) ShellID() string {
	return p.Shell.ID
}
