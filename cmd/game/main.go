// Tecmo Sooper Bowl — Phase 2 feel slice
//
// Controls:
//
//	1-4     select offensive play (pre-snap)
//	SPACE   snap / (on pass) throw to primary receiver
//	Arrow keys  steer ball carrier / QB
//	SHIFT   juke / spin (brief evade + burst)
//	R       reset match
//	T       show play-log summary in tip + stdout
//	D       toggle named defensive call (hidden by default)
//	Esc     quit
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
	match   *game.Match
	world   *sim.World
	play    *sim.PlayState
	slots   []playbook.Slot
	slotI   int
	varI    int
	offense []playbook.Play // flat list (tests / fallback)
	defense playbook.DefenseCall
	shell   playbook.CoverageShell
	tracker *ai.Tracker
	fatigue sim.Fatigue
	rng     *rand.Rand
	tip     string
	layout  render.Layout
	cam     render.Camera
	plays   *logplay.Logger

	// Snapshot at snap for the log (before result mutates match).
	snapDown      int
	snapDist      float64
	snapBall      float64
	snapDef       playbook.DefenseCall
	snapShell     playbook.CoverageShell
	snapLook      playbook.CoverageShell
	snapDisguised bool
	snapPlay      playbook.Play
	snapSit       game.SituationClass

	deadBallTimer float64
	loggedPlay    bool // true once this dead-ball result has been written
	showCall      bool // debug: name the front / shell
	staff         ai.Staff
	look          playbook.CoverageShell
	disguised     bool
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
		slots:   playbook.PlaySlots(),
		offense: playbook.DefaultOffense(),
		tracker: ai.NewTracker(12),
		rng:     rng,
		plays:   logger,
		layout: render.Layout{
			ScreenW:   screenW,
			ScreenH:   screenH,
			PadTop:    78,
			PadBottom: 20,
			PadX:      16,
		},
		cam:   render.NewCamera(),
		staff: ai.DefaultStaff(),
	}
	a.slotI = 0
	a.varI = 0
	a.repickDefense()
	a.world = a.placeLook()
	a.cam.Snap(field.Pos{X: field.HashMid, Y: a.match.LineOfScrimmage}, render.ScaleSelect)
	a.tip = "1-4 slot · SHIFT+3 hitch · SHIFT+4 PA · SPACE snap · D names the D"
	if a.plays != nil && a.plays.Path() != "" {
		log.Printf("play log → %s", a.plays.Path())
	}
	return a
}

func (a *App) repickDefense() {
	pkg := ai.ChooseStaffPackage(a.match.Situation(), a.tracker.Snapshot(), a.staff, a.rng)
	a.defense = pkg.Front
	a.shell = pkg.Shell
	a.look = pkg.LookOrShell()
	a.disguised = pkg.Disguised
}

func (a *App) placeLook() *sim.World {
	w := sim.PlacePreSnap(a.match.LineOfScrimmage)
	look := a.look
	if look.ID == "" {
		look = a.shell
	}
	sim.AlignDefense(w, a.defense, look, a.match.LineOfScrimmage)
	return w
}

func (a *App) defLabel() string {
	if !a.showCall {
		return "look"
	}
	return playbook.Package{Front: a.defense, Shell: a.shell, Look: a.look, Disguised: a.disguised}.String()
}

func (a *App) currentPlay() playbook.Play {
	if a.slotI < 0 || a.slotI >= len(a.slots) {
		return playbook.DefaultOffense()[0]
	}
	plays := a.slots[a.slotI].Plays
	if len(plays) == 0 {
		return playbook.DefaultOffense()[0]
	}
	i := a.varI
	if i < 0 || i >= len(plays) {
		i = 0
	}
	return plays[i]
}

func (a *App) selectedLabel() string {
	if a.slotI < 0 || a.slotI >= len(a.slots) {
		return a.currentPlay().Name
	}
	s := a.slots[a.slotI]
	return fmt.Sprintf("%s · %s", s.Name, a.currentPlay().Name)
}

func (a *App) lineCtx() sim.LineContext {
	snap := a.tracker.Snapshot()
	return sim.LineContext{
		ObviousPass: a.match.LongDown(),
		RunThreat:   snap.RunThreat,
		PassThreat:  snap.PassThreat,
		RunPct:      snap.RunPct,
		PassPct:     snap.PassPct,
		KeepThreat:  snap.KeepThreat,
		KeepN:       snap.KeepN,
		Samples:     snap.Samples,
	}
}

func (a *App) Update() error {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		if a.plays != nil {
			fmt.Fprintln(os.Stderr, a.plays.FormatSummary())
			a.plays.Close()
		}
		return ebiten.Termination
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyD) {
		a.showCall = !a.showCall
		if a.showCall {
			a.tip = "Call shown: " + a.defLabel()
		} else {
			a.tip = "Call hidden — read safeties and corners. D to name it."
		}
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
			a.world = a.placeLook()
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
	shift := ebiten.IsKeyPressed(ebiten.KeyShift) || ebiten.IsKeyPressed(ebiten.KeyShiftLeft)
	for i, key := range []ebiten.Key{ebiten.Key1, ebiten.Key2, ebiten.Key3, ebiten.Key4} {
		if !inpututil.IsKeyJustPressed(key) || i >= len(a.slots) {
			continue
		}
		plays := a.slots[i].Plays
		if len(plays) == 0 {
			continue
		}
		if shift && len(plays) > 1 {
			if a.slotI == i {
				a.varI = (a.varI + 1) % len(plays)
			} else {
				a.slotI = i
				a.varI = 1 % len(plays)
			}
		} else {
			a.slotI = i
			a.varI = 0
		}
		p := a.currentPlay()
		a.tip = fmt.Sprintf("Selected: %s — %s (D: %s)", a.selectedLabel(), p.Description, a.defLabel())
	}
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		a.snap()
	}
}

func (a *App) snap() {
	p := a.currentPlay()

	// Freeze pre-snap context for the play log
	a.snapDown = a.match.Down
	a.snapDist = a.match.Distance
	a.snapBall = a.match.BallY
	a.snapDef = a.defense
	a.snapShell = a.shell
	a.snapLook = a.look
	a.snapDisguised = a.disguised
	a.snapPlay = p
	a.snapSit = a.match.Situation()
	a.loggedPlay = false

	look := a.look
	if look.ID == "" {
		look = a.shell
	}
	a.play = sim.StartSnapLook(a.match.LineOfScrimmage, p, a.defense, a.shell, look, a.rng, a.fatigue, a.lineCtx())
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
	case "post":
		a.tip = "POST — drop, SPACE when primary breaks ~16 · green ring"
	case "pa_post":
		a.tip = "PA POST — mesh first (SPACE buffers) · SHIFT aborts the fake"
	case "pa_glance":
		a.tip = "PA GLANCE — mesh, then SPACE in the WINDOW over the Mike"
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
				y, gained, a.defLabel(), int(stam*100))
			if a.play.PlayAction && a.play.Mesh == sim.MeshLive {
				a.tip = "PA FAKE — SPACE buffers the throw · SHIFT aborts (no bite)"
			} else if a.play.InLeftoverWindow() {
				a.tip = "PA WINDOW — leftover bite, SPACE now"
			}
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

	relTip := ""
	if r.Thrown && r.ReleaseAt >= 0 {
		relTip = fmt.Sprintf("  |  rel %.2fs", r.ReleaseAt)
	}
	a.tip = fmt.Sprintf("%s  |  %+.1f yds%s  |  STA %d%%  |  was %s",
		r.Message, r.YardsGained, relTip, int(a.fatigue.Display(sim.RoleRB)*100), a.snapDef.Name)
}

func (a *App) recordPlay(r sim.Result, tend ai.Snapshot) {
	if a.loggedPlay || a.plays == nil {
		return
	}
	a.loggedPlay = true
	e := logplay.Entry{
		OffPlay:     a.snapPlay.ID,
		OffName:     a.snapPlay.Name,
		DefCall:     a.snapDef.ID,
		Shell:       a.snapShell.ID,
		Look:        a.snapLook.ID,
		Disguised:   a.snapDisguised,
		Outcome:     r.Outcome.String(),
		Yards:       r.YardsGained,
		DownBefore:  a.snapDown,
		DistBefore:  a.snapDist,
		BallBefore:  a.snapBall,
		DownAfter:   a.match.Down,
		DistAfter:   a.match.Distance,
		BallAfter:   a.match.BallY,
		RunPct:      tend.RunPct,
		PassPct:     tend.PassPct,
		RightPct:    tend.RightPct,
		Stamina:     a.fatigue.Display(sim.RoleRB),
		Thrown:      r.Thrown,
		Carrier:     string(r.Carrier),
		QBKeep:      r.QBKeep,
		KeepThreat:  tend.KeepThreat,
		KeepN:       tend.KeepN,
		Message:     r.Message,
		RunThreat:   r.RunThreat,
		BiteSec:     r.BiteSec,
		LeftoverSec: r.LeftoverSec,
		ReleaseAt:   r.ReleaseAt,
		Mesh:        r.Mesh,
		BiterN:      r.BiterN,
		Biters:      r.Biters,
		SepAtThrow:  r.SepAtThrow,
	}
	e = a.plays.Record(e)
	keep := ""
	if e.QBKeep {
		keep = " KEEP"
	}
	rel := "rel=—"
	if e.Thrown && e.ReleaseAt >= 0 {
		rel = fmt.Sprintf("rel=%.2fs", e.ReleaseAt)
	}
	pa := ""
	if e.Mesh != "" && e.Mesh != "none" {
		pa = fmt.Sprintf(" mesh=%s left=%.2f", e.Mesh, e.LeftoverSec)
	}
	vs := e.DefCall + "/" + e.Shell
	if e.Disguised && e.Look != "" && e.Look != e.Shell {
		vs = e.DefCall + "/" + e.Look + "→" + e.Shell
	}
	fmt.Fprintf(os.Stderr, "play#%d%s %s (%s) vs %s → %s %+.1f yds (%s) %s tend run=%.0f%% pass=%.0f%% runT=%.1f keepT=%.1f n=%d%s\n",
		e.N, keep, e.OffPlay, e.Carrier, vs, e.Outcome, e.Yards, e.Message,
		rel, e.RunPct*100, e.PassPct*100, e.RunThreat, e.KeepThreat, e.KeepN, pa)
}

func (a *App) finishDeadBall() {
	r := a.play.Result
	incomplete := r.Outcome == sim.OutcomeIncomplete
	td := r.Outcome == sim.OutcomeTouchdown

	a.match.ApplyPlayResult(r.BallY, r.YardsGained, incomplete, td)
	depth := 0
	switch a.snapPlay.DepthLabel {
	case "intermediate":
		depth = 1
	case "deep":
		depth = 2
	}
	a.tracker.Observe(ai.PlayObservation{
		Type:      a.snapPlay.Type,
		Side:      a.snapPlay.Side,
		Depth:     depth,
		Situation: a.snapSit,
		Yards:     r.YardsGained,
		Outcome:   r.Outcome.String(),
		QBKeep:    r.QBKeep,
	})
	a.recordPlay(r, a.tracker.Snapshot())

	relTip := ""
	if r.Thrown && r.ReleaseAt >= 0 {
		relTip = fmt.Sprintf("  rel %.2fs", r.ReleaseAt)
	}

	if a.match.Phase == game.PhaseScore {
		a.tip = fmt.Sprintf("TOUCHDOWN! Home %d — next drive loading…%s", a.match.HomeScore, relTip)
		a.deadBallTimer = 1.4
		a.play = nil
		return
	}

	a.repickDefense()
	a.world = a.placeLook()
	a.play = nil
	a.match.Phase = game.PhasePlaySelect
	snap := a.tracker.Snapshot()
	a.tip = fmt.Sprintf("%s (%.1f yds%s) → %d & %.0f | D [%s] | run %.0f%% | STA %d%%",
		r.Message, r.YardsGained, relTip, a.match.Down, a.match.Distance, a.defLabel(),
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
	if a.play != nil && a.play.PlayAction && a.play.Mesh == sim.MeshLive &&
		a.play.QBIdx >= 0 && a.play.RBIdx >= 0 {
		render.DrawPlayAction(screen, &a.cam,
			a.world.Units[a.play.QBIdx].Pos, a.world.Units[a.play.RBIdx].Pos)
	}
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
	shown := a.currentPlay()
	shown.Name = a.selectedLabel()
	render.DrawHUD(screen, a.match, shown, a.defense, a.shell, a.look, a.disguised, a.tip, phase, stam, jukeCD, a.showCall)
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
