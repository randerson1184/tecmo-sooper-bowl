package sim

import (
	"math"
	"math/rand"
	"testing"

	"github.com/randerson1184/tecmo-sooper-bowl/internal/playbook"
)

func TestTapeRecordsSnapAndTicks(t *testing.T) {
	ps := StartPlay(25, playbook.DefaultOffense()[0], playbook.DefaultDefenseCalls()[0],
		rand.New(rand.NewSource(1)), Fatigue{})
	if ps.Tape.Len() != 1 {
		t.Fatalf("snap should record frame 0, got %d", ps.Tape.Len())
	}
	for i := 0; i < 30; i++ {
		ps.Tick(1.0/60.0, Input{DY: 1})
	}
	if ps.Tape.Len() != 31 {
		t.Fatalf("expected 31 frames after 30 ticks, got %d", ps.Tape.Len())
	}
	want := 30.0 / 60.0
	if d := ps.Tape.Duration(); math.Abs(d-want) > 1e-9 {
		t.Fatalf("duration %.4f, want %.4f", d, want)
	}
}

func TestTapeViewAtEnds(t *testing.T) {
	ps := StartPlay(30, playbook.DefaultOffense()[0], playbook.DefaultDefenseCalls()[0],
		rand.New(rand.NewSource(2)), Fatigue{})
	start := ps.World.Ball
	for i := 0; i < 45; i++ {
		ps.Tick(1.0/60.0, Input{DY: 1})
	}
	end := ps.World.Ball

	v0 := ps.Tape.ViewAt(0)
	if math.Abs(v0.World.Ball.Y-start.Y) > 0.01 {
		t.Fatalf("t=0 ball Y=%.2f, snap was %.2f", v0.World.Ball.Y, start.Y)
	}
	v1 := ps.Tape.ViewAt(ps.Tape.Duration() + 1)
	if math.Abs(v1.World.Ball.Y-end.Y) > 0.01 {
		t.Fatalf("t=end ball Y=%.2f, last tick was %.2f", v1.World.Ball.Y, end.Y)
	}
	if len(v0.World.Units) != len(ps.World.Units) {
		t.Fatalf("view units %d, live %d", len(v0.World.Units), len(ps.World.Units))
	}
}

func TestTapeLerpIsBetweenFrames(t *testing.T) {
	ps := StartPlay(25, playbook.DefaultOffense()[0], playbook.DefaultDefenseCalls()[0],
		rand.New(rand.NewSource(3)), Fatigue{})
	ps.Tick(1.0/60.0, Input{DY: 1})
	a := ps.Tape.Frames[0].Ball.Y
	b := ps.Tape.Frames[1].Ball.Y
	mid := ps.Tape.ViewAt(0.5 / 60.0).World.Ball.Y
	lo, hi := a, b
	if lo > hi {
		lo, hi = hi, lo
	}
	if mid < lo-1e-9 || mid > hi+1e-9 {
		t.Fatalf("lerped Y=%.3f not between %.3f and %.3f", mid, a, b)
	}
}

func TestTapeStartsOnTheLook(t *testing.T) {
	const los = 30.0
	def := playbook.DefaultDefenseCalls()[0]
	look := playbook.DefaultShell()
	want := PlacePreSnap(los)
	AlignDefense(want, def, look, los)

	ps := StartSnap(los, playbook.DefaultOffense()[2], def, look,
		rand.New(rand.NewSource(4)), Fatigue{}, LineContext{})
	if ps.Tape.Len() < 1 {
		t.Fatal("expected a snap frame")
	}
	got := ps.Tape.ViewAt(0)
	for i, u := range want.Units {
		if u.Side != SideDefense {
			continue
		}
		if i >= len(got.World.Units) {
			t.Fatalf("tape missing unit %d", i)
		}
		p := got.World.Units[i].Pos
		if math.Abs(p.X-u.Pos.X) > 0.05 || math.Abs(p.Y-u.Pos.Y) > 0.05 {
			t.Fatalf("def[%d] %s tape=(%.1f,%.1f) look=(%.1f,%.1f)",
				i, u.Role, p.X, p.Y, u.Pos.X, u.Pos.Y)
		}
	}
}

func TestEmptyTapeViewIsSafe(t *testing.T) {
	var tpe *Tape
	v := tpe.ViewAt(1)
	if v.World == nil {
		t.Fatal("nil tape should still return a world")
	}
	if tpe.Duration() != 0 || tpe.Len() != 0 {
		t.Fatal("nil tape should be empty")
	}
}
