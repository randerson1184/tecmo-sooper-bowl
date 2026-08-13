// Package playbook defines a tiny Tecmo-sized set of formations and plays.
package playbook

// Side bias for run/pass concepts.
type Side int

const (
	SideLeft Side = iota
	SideMiddle
	SideRight
)

// PlayType is the coarse tag used by tendency tracking.
type PlayType int

const (
	PlayRun PlayType = iota
	PlayPass
)

// Play is a callable offensive concept (MVP: labels + tags; routes come in Phase 1).
type Play struct {
	ID          string
	Name        string
	Type        PlayType
	Side        Side
	DepthLabel  string // "short" | "intermediate" | "deep" | "none"
	Description string
}

// DefaultOffense is the Phase-1 target playbook (usable as stubs today).
func DefaultOffense() []Play {
	return []Play{
		{ID: "inside_zone", Name: "Inside Zone", Type: PlayRun, Side: SideMiddle, DepthLabel: "none", Description: "Hand off up the gut"},
		{ID: "sweep", Name: "Toss Sweep", Type: PlayRun, Side: SideRight, DepthLabel: "none", Description: "Stretch to the right"},
		{ID: "slant", Name: "Slant", Type: PlayPass, Side: SideLeft, DepthLabel: "short", Description: "Quick slant timing throw"},
		{ID: "hitch", Name: "Hitch", Type: PlayPass, Side: SideRight, DepthLabel: "short", Description: "Stop route on the outside"},
	}
}

// DefenseCall is a named defensive game plan (see DESIGN.md §6).
type DefenseCall struct {
	ID          string
	Name        string
	Description string
}

func DefaultDefenseCalls() []DefenseCall {
	return []DefenseCall{
		{ID: "base", Name: "Base", Description: "Balanced front & coverage"},
		{ID: "run_fit", Name: "Run Fit", Description: "Extra body in the box"},
		{ID: "soft_zone", Name: "Soft Zone", Description: "Protect deep; give underneath"},
		{ID: "pass_rush", Name: "Pass Rush", Description: "Pressure over coverage"},
		{ID: "blitz", Name: "Blitz", Description: "Send extra; risk big play"},
	}
}
