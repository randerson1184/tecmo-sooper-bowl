package playbook

import "testing"

func TestPlaySlotsAreExplicit(t *testing.T) {
	slots := PlaySlots()
	if len(slots) != 4 {
		t.Fatalf("want 4 slots, got %d", len(slots))
	}
	if slots[2].Name != "Quick" || slots[2].Plays[0].ID != "slant" || slots[2].Plays[1].ID != "hitch" {
		t.Fatalf("slot 3 should be slant/hitch, got %+v", slots[2])
	}
	if slots[3].Name != "Shot" || slots[3].Plays[0].ID != "post" || slots[3].Plays[1].ID != "pa_post" {
		t.Fatalf("slot 4 should be post / PA post, got %+v", slots[3])
	}
	for _, s := range slots {
		for _, p := range s.Plays {
			if p.ID == "" {
				t.Fatalf("empty play in slot %s", s.Name)
			}
		}
	}
}

func TestPostConceptIsAShot(t *testing.T) {
	c, ok := ConceptFor("post")
	if !ok {
		t.Fatal("post concept missing")
	}
	if c.PrimaryDepth < 14 || c.PrimaryBreak != "post" {
		t.Fatalf("post is not a shot: %+v", c)
	}
	if c.PlayAction {
		t.Fatal("base post should be callable without play-action unlock")
	}
}

func TestPAPostIsAlwaysCallable(t *testing.T) {
	c, ok := ConceptFor("pa_post")
	if !ok {
		t.Fatal("pa_post concept missing")
	}
	if !c.PlayAction || c.PrimaryBreak != "post" || c.PrimaryDepth < 14 {
		t.Fatalf("PA post is not a play-action shot: %+v", c)
	}
}
