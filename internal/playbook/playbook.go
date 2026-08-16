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
		{ID: "post", Name: "Post", Type: PlayPass, Side: SideMiddle, DepthLabel: "intermediate", Description: "Stem vertical, break to the post ~16 yards"},
		{ID: "pa_post", Name: "PA Post", Type: PlayPass, Side: SideMiddle, DepthLabel: "intermediate", Description: "Fake inside zone, throw the post. Always callable."},
		{ID: "pa_glance", Name: "PA Glance", Type: PlayPass, Side: SideMiddle, DepthLabel: "short", Description: "Fake inside zone, throw the glance over the Mike. Always callable."},
	}
}

// DefenseCall is a named *front / pressure* (see DESIGN.md §6).
// Coverage lives on CoverageShell — do not encode Cover 2/3 here.
type DefenseCall struct {
	ID          string
	Name        string
	Description string
}

func DefaultDefenseCalls() []DefenseCall {
	return []DefenseCall{
		{ID: "base", Name: "Base", Description: "Balanced front"},
		{ID: "run_fit", Name: "Run Fit", Description: "Extra body in the box"},
		{ID: "soft_zone", Name: "Soft Zone", Description: "Light box; second level stays back"},
		{ID: "pass_rush", Name: "Pass Rush", Description: "Pressure over the front"},
		{ID: "blitz", Name: "Blitz", Description: "Send extra; risk a big play"},
	}
}
