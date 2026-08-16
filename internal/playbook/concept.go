package playbook

// Slot is one of the four visible categories. The player picks the slot,
// then an explicit variant — never a silent down-and-distance swap.
type Slot struct {
	Key   int
	Name  string // Inside / Outside / Quick / Shot
	Plays []Play
}

func PlaySlots() []Slot {
	off := DefaultOffense()
	byID := map[string]Play{}
	for _, p := range off {
		byID[p.ID] = p
	}
	return []Slot{
		{Key: 1, Name: "Inside", Plays: []Play{byID["inside_zone"]}},
		{Key: 2, Name: "Outside", Plays: []Play{byID["sweep"]}},
		{Key: 3, Name: "Quick", Plays: []Play{byID["slant"], byID["hitch"]}},
		{Key: 4, Name: "Shot", Plays: []Play{byID["post"], byID["pa_post"], byID["pa_glance"]}},
	}
}

// Concept is the reusable play definition. New plays should go through here
// instead of more play.ID switches in sim.
type Concept struct {
	Play
	DropYards    float64 // QB drop; 0 = handoff
	PlayAction   bool
	PrimaryDepth float64 // yards past LOS for the primary
	PrimaryBreak string  // "post", "glance", "slant", "hitch", "go"
}

func ConceptFor(id string) (Concept, bool) {
	c, ok := conceptByID()[id]
	return c, ok
}

var conceptCache map[string]Concept

func conceptByID() map[string]Concept {
	if conceptCache != nil {
		return conceptCache
	}
	off := DefaultOffense()
	byID := map[string]Play{}
	for _, p := range off {
		byID[p.ID] = p
	}
	conceptCache = map[string]Concept{
		"post": {
			Play:         byID["post"],
			DropYards:    3.4,
			PlayAction:   false,
			PrimaryDepth: 16,
			PrimaryBreak: "post",
		},
		"pa_post": {
			Play:         byID["pa_post"],
			DropYards:    3.8,
			PlayAction:   true,
			PrimaryDepth: 16,
			PrimaryBreak: "post",
		},
		"pa_glance": {
			Play:         byID["pa_glance"],
			DropYards:    3.8,
			PlayAction:   true,
			PrimaryDepth: 11,
			PrimaryBreak: "glance",
		},
	}
	return conceptCache
}
