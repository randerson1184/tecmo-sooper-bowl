// Tecmo Sooper Bowl — Phase 2 feel slice
//
// Controls:
//   1-4     select offensive play (pre-snap)
//   SPACE   snap / (on pass) throw to primary receiver
//   Arrow keys  steer ball carrier / QB
//   SHIFT   juke / spin (brief evade + burst)
//   R       reset match
//   T       show play-log summary in tip + stdout
//   Esc     quit
package main

import (
	"fmt"
	"image/color"
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/randerson1184/tecmo-sooper-bowl/internal/ai"
	"github.com/randerson1184/tecmo-sooper-bowl/internal/field"
	"github.com/randerson1184/tecmo-sooper-bowl/internal/game"
	"github.com/randerson1184/tecmo-sooper-bowl/internal/logplay"
	"github.com/randerson1184/tecmo-sooper-bowl/internal/playbook"
	"github.com/randerson1184/tecmo-sooper-bowl/internal/render"
	"github.com/randerson1184/tecmo-sooper-bowl/internal/sim"
)

const (
	screenW = 960
	screenH = 720
)

type App struct {
	match    *game.Match
	world    *sim.World
	play     *sim.PlayState
	offense  []playbook.Play
	selected int
	defense  playbook.DefenseCall
	tracker  *ai.Tracker
	fatigue  sim.Fatigue
	rng      *rand.Rand
	tip      string
	layout   render.Layout
	cam      render.Camera
	plays    *logplay.Logger

	// Snapshot at snap for the log (before result mutates match).
	snapDown int
	snapDist float64
	snapBall float64
	snapDef  playbook.DefenseCall
	snapPlay playbook.Play

	deadBallTimer float64
	loggedPlay    bool // true once this dead-ball result has been written
}

func newApp() *App {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	logger, err := logplay.New("logs")
	if err != nil {
		log.Printf("play log file unavailable (memory only): %v", err)
		logger, _ = logplay.New("")
	}
	a := &App{
		match:   game.NewMatch(),
		offense: playbook.DefaultOffense(),
		tracker: ai.NewTracker(16),
		rng:     rng,
		plays:   logger,
		layout: render.Layout{
			ScreenW:   screenW,
			ScreenH:   screenH,
			PadTop:    78,
			PadBottom: 20,
			PadX:      16,
		},
		cam: render.NewCamera(),
	}
	a.selected = 0
	a.repickDefense()
	a.world = sim.PlacePreSnap(a.match.LineOfScrimmage)
	a.cam.Snap(field.Pos{X: field.HashMid, Y: a.match.LineOfScrimmage}, render.ScaleSelect)
	a.tip = "1-4 play · SPACE snap · Arrows · SHIFT juke · T log summary · R reset"
	if a.plays != nil && a.plays.Path() != "" {
		log.Printf("play log → %s", a.plays.Path())
	}
	return a
}

func (a *App) repickDefense() {
	a.defense = ai.ChooseDefense(a.match.Situation(), a.tracker.Snapshot(), a.rng)
}

func (a *App) Update() error {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		if a.plays != nil {
			fmt.Fprintln(os.Stderr, a.plays.FormatSummary())
			a.plays.Close()
		}
		return ebiten.Termination
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyT) {
		sum := a.plays.FormatSummary()
		fmt.Fprintln(os.Stderr, sum)
		// Short tip line
		a.tip = "Log summary printed to terminal (also logs/*.jsonl)"
		if last, ok := a.plays.Last(); ok {
			a.tip = fmt.Sprintf("Last: %s vs %s → %s %+.1f | T=full summary in terminal",
				last.OffName, last.DefCall, last.Outcome, last.Yards)
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyR) {
		if a.plays != nil {
			fmt.Fprintln(os.Stderr, a.plays.FormatSummary())
			a.plays.Close()
		}
		*a = *newApp()
		a.tip = "Drive reset — new log file started."
		return nil
	}

	dt := 1.0 / 60.0

	switch a.match.Phase {
	case game.PhasePlaySelect, game.PhasePreSnap:
		a.updatePreSnap()
		a.cam.SetTarget(field.Pos{X: field.HashMid, Y: a.match.LineOfScrimmage + 4}, render.ScaleSelect)
	case game.PhaseInPlay:
		a.updateInPlay(dt)
		a.updateLiveCamera()
	case game.PhaseDeadBall:
		a.deadBallTimer -= dt
		a.updateLiveCamera()
		if a.deadBallTimer <= 0 {
			a.finishDeadBall()
		}
	case game.PhaseScore:
		a.deadBallTimer -= dt
		a.cam.SetTarget(field.Pos{X: field.HashMid, Y: 95}, render.ScaleScore)
		if a.deadBallTimer <= 0 {
			a.match.SpotKickoffOrDrive()
			a.fatigue.BetweenDrives()
			a.repickDefense()
			a.world = sim.PlacePreSnap(a.match.LineOfScrimmage)
			a.play = nil
			a.cam.Snap(field.Pos{X: field.HashMid, Y: a.match.LineOfScrimmage}, render.ScaleSelect)
			a.tip = "Kickoff spot — drive on. 1-4 play, SPACE snap."
		}
	}

	a.cam.Update(dt)
	return nil
}

func (a *App) updateLiveCamera() {
	focus := field.Pos{X: field.HashMid, Y: a.match.LineOfScrimmage}
	if a.play != nil {
		if a.play.BallInAir {
			focus = a.play.BallPos
		} else if a.world != nil {
			focus = a.world.Ball
			// slight look-ahead toward opponent end zone
			focus.Y += 3
		}
	}
	// Keep focus on field
	if focus.X < 8 {
		focus.X = 8
	}
	if focus.X > field.WidthYards-8 {
		focus.X = field.WidthYards - 8
	}
	if focus.Y < 5 {
		focus.Y = 5
	}
	if focus.Y > 95 {
		focus.Y = 95
	}
	a.cam.SetTarget(focus, render.ScaleLive)
}

func (a *App) updatePreSnap() {
	for i, key := range []ebiten.Key{ebiten.Key1, ebiten.Key2, ebiten.Key3, ebiten.Key4} {
		if inpututil.IsKeyJustPressed(key) && i < len(a.offense) {
			a.selected = i
			p := a.offense[i]
			a.tip = fmt.Sprintf("Selected: %s — %s (D: %s)", p.Name, p.Description, a.defense.Name)
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		a.snap()
	}
}

func (a *App) snap() {
	p := a.offense[a.selected]

	// Freeze pre-snap context for the play log
	a.snapDown = a.match.Down
	a.snapDist = a.match.Distance
	a.snapBall = a.match.BallY
	a.snapDef = a.defense
	a.snapPlay = p
	a.loggedPlay = false

	depth := 0
	switch p.DepthLabel {
	case "intermediate":
		depth = 1
	case "deep":
		depth = 2
	}
	a.tracker.Observe(ai.PlayObservation{
		Type:      p.Type,
		Side:      p.Side,
		Depth:     depth,
		Situation: a.match.Situation(),
	})

	a.play = sim.StartPlay(a.match.LineOfScrimmage, p, a.defense, a.rng, a.fatigue)
	a.world = a.play.World
	a.match.Phase = game.PhaseInPlay
	a.cam.SetTarget(a.world.Ball, render.ScaleLive)

	switch p.ID {
	case "inside_zone":
		a.tip = "ZONE — hold ↑, cut into the hole · SHIFT juke"
	case "sweep":
		a.tip = "SWEEP — pitch right; ride ↑/→ then cut up before sideline · SHIFT juke"
	case "slant":
		a.tip = "SLANT — quick drop, SPACE when primary breaks in (~0.5s) · green ring"
	case "hitch":
		a.tip = "HITCH — SPACE as primary stops outside · then run YAC"
	default:
		a.tip = "Arrows move · SPACE throw · SHIFT juke"
	}
}

func (a *App) updateInPlay(dt float64) {
	if a.play == nil {
		return
	}
	in := sim.Input{}
	if ebiten.IsKeyPressed(ebiten.KeyArrowLeft) {
		in.DX -= 1
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowRight) {
		in.DX += 1
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowDown) {
		// Down = toward own end zone (−Y on field)
		in.DY -= 1
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowUp) {
		// Up = toward opponent end zone (+Y)
		in.DY += 1
	}
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) || inpututil.IsKeyJustPressed(ebiten.KeyF) {
		in.Throw = true
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyShift) || inpututil.IsKeyJustPressed(ebiten.KeyShiftLeft) ||
		inpututil.IsKeyJustPressed(ebiten.KeyShiftRight) || inpututil.IsKeyJustPressed(ebiten.KeyE) {
		in.Juke = true
	}

	stillLive := a.play.Tick(dt, in)
	a.world = a.play.World

	stam := a.fatigue.Display(sim.RoleRB)
	if stillLive && !a.play.BallInAir {
		if idx := ballCarrier(a.world); idx >= 0 {
			y := a.world.Units[idx].Pos.Y
			gained := y - a.play.LOS
			role := a.world.Units[idx].Role
			stam = a.fatigue.Display(role)
			a.tip = fmt.Sprintf("LIVE  %.0f yd line  (%+.1f)  D:%s  STA %d%%",
				y, gained, a.defense.Name, int(stam*100))
			if a.play.Play.Type == playbook.PlayPass && a.play.ControlIdx == a.play.QBIdx {
				a.tip += "  SPACE throw"
			}
			if a.play.JukeCooldown <= 0 {
				a.tip += "  SHIFT juke"
			}
		}
	} else if stillLive && a.play.BallInAir {
		a.tip = "Ball in the air…"
	}

	if !stillLive {
		a.beginDeadBall()
	}
}

func ballCarrier(w *sim.World) int {
	if w == nil {
		return -1
	}
	for i, u := range w.Units {
		if u.HasBall {
			return i
		}
	}
	return -1
}

func (a *App) beginDeadBall() {
	a.match.Phase = game.PhaseDeadBall
	a.deadBallTimer = 1.0
	r := a.play.Result
	// Fatigue from this snap
	wasPass := a.play.Play.Type == playbook.PlayPass
	incomplete := r.Outcome == sim.OutcomeIncomplete
	role := a.play.CarrierRole
	if role == "" {
		if wasPass {
			role = sim.RoleQB
		} else {
			role = sim.RoleRB
		}
	}
	a.fatigue.OnPlayEnd(role, r.YardsGained, wasPass, incomplete)

	tend := a.tracker.Snapshot()
	a.recordPlay(r, tend)

	a.tip = fmt.Sprintf("%s  |  %+.1f yds  |  STA %d%%  |  was %s",
		r.Message, r.YardsGained, int(a.fatigue.Display(sim.RoleRB)*100), a.snapDef.Name)
}

func (a *App) recordPlay(r sim.Result, tend ai.Snapshot) {
	if a.loggedPlay || a.plays == nil {
		return
	}
	a.loggedPlay = true
	// After-result ball/down estimated from result (match not updated yet)
	ballAfter := r.BallY
	downAfter := a.snapDown
	distAfter := a.snapDist
	if r.Outcome == sim.OutcomeIncomplete {
		downAfter = a.snapDown + 1
	} else if r.Outcome == sim.OutcomeTouchdown {
		ballAfter = 100
	} else {
		distAfter = a.snapDist - r.YardsGained
		if distAfter <= 0 {
			downAfter = 1
			distAfter = 10
		} else {
			downAfter = a.snapDown + 1
		}
	}
	e := logplay.Entry{
		OffPlay:    a.snapPlay.ID,
		OffName:    a.snapPlay.Name,
		DefCall:    a.snapDef.ID,
		Outcome:    r.Outcome.String(),
		Yards:      r.YardsGained,
		DownBefore: a.snapDown,
		DistBefore: a.snapDist,
		BallBefore: a.snapBall,
		DownAfter:  downAfter,
		DistAfter:  distAfter,
		BallAfter:  ballAfter,
		RunPct:     tend.RunPct,
		PassPct:    tend.PassPct,
		RightPct:   tend.RightPct,
		Stamina:    a.fatigue.Display(sim.RoleRB),
		Message:    r.Message,
	}
	a.plays.Record(e)
	// One line to terminal for live tuning sessions
	fmt.Fprintf(os.Stderr, "play#%d %s vs %s → %s %+.1f yds (%s) tend run=%.0f%% right=%.0f%%\n",
		e.N, e.OffPlay, e.DefCall, e.Outcome, e.Yards, e.Message, e.RunPct*100, e.RightPct*100)
}

func (a *App) finishDeadBall() {
	r := a.play.Result
	incomplete := r.Outcome == sim.OutcomeIncomplete
	td := r.Outcome == sim.OutcomeTouchdown

	a.match.ApplyPlayResult(r.BallY, r.YardsGained, incomplete, td)

	if a.match.Phase == game.PhaseScore {
		a.tip = fmt.Sprintf("TOUCHDOWN! Home %d — next drive loading…", a.match.HomeScore)
		a.deadBallTimer = 1.4
		a.play = nil
		return
	}

	a.repickDefense()
	a.world = sim.PlacePreSnap(a.match.LineOfScrimmage)
	a.play = nil
	a.match.Phase = game.PhasePlaySelect
	snap := a.tracker.Snapshot()
	a.tip = fmt.Sprintf("%s (%.1f yds) → %d & %.0f | D [%s] | run %.0f%% | STA %d%%",
		r.Message, r.YardsGained, a.match.Down, a.match.Distance, a.defense.Name,
		snap.RunPct*100, int(a.fatigue.Display(sim.RoleRB)*100))
}

func (a *App) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{10, 12, 18, 255})
	render.DrawField(screen, a.layout, &a.cam, a.match)
	primary := -1
	if a.play != nil {
		primary = a.play.PrimaryIdx
	}
	render.DrawUnits(screen, &a.cam, a.world, primary)
	if a.play != nil && a.play.BallInAir {
		render.DrawBall(screen, &a.cam, a.play.BallPos)
	}
	phase := "SELECT"
	switch a.match.Phase {
	case game.PhaseInPlay:
		phase = "LIVE"
	case game.PhaseDeadBall:
		phase = "WHISTLE"
	case game.PhaseScore:
		phase = "SCORE"
	}
	stam := a.fatigue.Display(sim.RoleRB)
	jukeCD := 0.0
	if a.play != nil {
		jukeCD = a.play.JukeCooldown
		if idx := ballCarrier(a.world); idx >= 0 {
			stam = a.fatigue.Display(a.world.Units[idx].Role)
		}
	}
	render.DrawHUD(screen, a.match, a.offense[a.selected], a.defense, a.tip, phase, stam, jukeCD)
}

func (a *App) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenW, screenH
}

func main() {
	ebiten.SetWindowSize(screenW, screenH)
	ebiten.SetWindowTitle("Tecmo Sooper Bowl")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetTPS(60)

	if err := ebiten.RunGame(newApp()); err != nil {
		log.Fatal(err)
	}
}
