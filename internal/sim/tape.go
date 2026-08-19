package sim

import "github.com/randerson1184/tecmo-sooper-bowl/internal/field"

// Pose tape of one snap. Recorded at the sim tick rate so the last play
// can be played back without re-running the RNG.
const (
	tapeHz        = 60.0
	tapeMaxN      = 60 * 20 // 20s of live play is plenty
	ReplayRate    = 0.5
	ReplayHoldSec = 1.0 // freeze on the snap / LOS before each loop
)

// TapePose is one body on one recorded frame.
type TapePose struct {
	Pos     field.Pos
	Role    Role
	Side    Side
	HasBall bool
	Juke    bool
}

// TapeFrame is one recorded tick.
type TapeFrame struct {
	Poses     []TapePose
	Ball      field.Pos
	BallInAir bool
	MeshLive  bool
}

// Tape is the last snap's poses, in tick order (frame 0 = snap).
type Tape struct {
	PrimaryIdx int
	Frames     []TapeFrame
}

// TapeView is a drawable world sampled at a tape time.
type TapeView struct {
	World      *World
	BallInAir  bool
	MeshLive   bool
	PrimaryIdx int
}

func newTape(primary int) *Tape {
	return &Tape{PrimaryIdx: primary}
}

func (t *Tape) Len() int {
	if t == nil {
		return 0
	}
	return len(t.Frames)
}

// Duration is live seconds on the tape (not playback time).
func (t *Tape) Duration() float64 {
	n := t.Len()
	if n < 2 {
		return 0
	}
	return float64(n-1) / tapeHz
}

func (t *Tape) capture(ps *PlayState) {
	if t == nil || ps == nil || ps.World == nil {
		return
	}
	if len(t.Frames) >= tapeMaxN {
		return
	}
	t.Frames = append(t.Frames, frameFromWorld(ps.World, ps.BallInAir, ps.PlayAction && ps.Mesh == MeshLive))
}

// StampLook overwrites frame 0 with the huddle the player actually saw.
func (t *Tape) StampLook(w *World) {
	if t == nil || w == nil {
		return
	}
	f := frameFromWorld(w, false, false)
	if len(t.Frames) == 0 {
		t.Frames = []TapeFrame{f}
		return
	}
	t.Frames[0] = f
}

func frameFromWorld(w *World, ballInAir, meshLive bool) TapeFrame {
	f := TapeFrame{
		Poses:     make([]TapePose, len(w.Units)),
		BallInAir: ballInAir,
		MeshLive:  meshLive,
	}
	if w == nil {
		return f
	}
	f.Ball = w.Ball
	for i, u := range w.Units {
		f.Poses[i] = TapePose{
			Pos:     u.Pos,
			Role:    u.Role,
			Side:    u.Side,
			HasBall: u.HasBall,
			Juke:    u.JukeTimer > 0,
		}
	}
	return f
}

// ViewAt samples the tape at live-play seconds, lerping between ticks.
func (t *Tape) ViewAt(sec float64) TapeView {
	v := TapeView{PrimaryIdx: -1, World: &World{}}
	if t == nil || len(t.Frames) == 0 {
		return v
	}
	v.PrimaryIdx = t.PrimaryIdx
	if sec < 0 {
		sec = 0
	}
	max := t.Duration()
	if sec > max {
		sec = max
	}
	if len(t.Frames) == 1 || max <= 0 {
		return t.viewFrom(t.Frames[0])
	}
	alpha := sec * tapeHz
	i0 := int(alpha)
	if i0 >= len(t.Frames)-1 {
		return t.viewFrom(t.Frames[len(t.Frames)-1])
	}
	frac := alpha - float64(i0)
	if frac <= 0 {
		return t.viewFrom(t.Frames[i0])
	}
	if frac >= 1 {
		return t.viewFrom(t.Frames[i0+1])
	}
	return t.lerp(t.Frames[i0], t.Frames[i0+1], frac)
}

func (t *Tape) viewFrom(f TapeFrame) TapeView {
	w := &World{Ball: f.Ball, Units: make([]Unit, len(f.Poses))}
	for i, p := range f.Poses {
		w.Units[i] = unitFromPose(p)
	}
	pri := -1
	if t != nil {
		pri = t.PrimaryIdx
	}
	return TapeView{World: w, BallInAir: f.BallInAir, MeshLive: f.MeshLive, PrimaryIdx: pri}
}

func (t *Tape) lerp(a, b TapeFrame, frac float64) TapeView {
	n := len(a.Poses)
	if len(b.Poses) < n {
		n = len(b.Poses)
	}
	src := a
	if frac >= 0.5 {
		src = b
	}
	w := &World{
		Ball:  field.Pos{X: lerp(a.Ball.X, b.Ball.X, frac), Y: lerp(a.Ball.Y, b.Ball.Y, frac)},
		Units: make([]Unit, n),
	}
	for i := 0; i < n; i++ {
		p := src.Poses[i]
		p.Pos.X = lerp(a.Poses[i].Pos.X, b.Poses[i].Pos.X, frac)
		p.Pos.Y = lerp(a.Poses[i].Pos.Y, b.Poses[i].Pos.Y, frac)
		w.Units[i] = unitFromPose(p)
	}
	pri := -1
	if t != nil {
		pri = t.PrimaryIdx
	}
	return TapeView{World: w, BallInAir: src.BallInAir, MeshLive: src.MeshLive, PrimaryIdx: pri}
}

func unitFromPose(p TapePose) Unit {
	u := Unit{
		Role:    p.Role,
		Side:    p.Side,
		Pos:     p.Pos,
		HasBall: p.HasBall,
	}
	if p.Juke {
		u.JukeTimer = 1
	}
	return u
}

func lerp(a, b, t float64) float64 {
	return a + (b-a)*t
}
