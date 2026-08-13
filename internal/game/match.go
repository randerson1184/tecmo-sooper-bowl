// Package game is match-level state: score, clock, down & distance, possession.
// It does not simulate player physics — that lives in sim/.
package game

// TeamID is 0 = home (player), 1 = away (CPU) for MVP.
type TeamID int

const (
	TeamHome TeamID = 0
	TeamAway TeamID = 1
)

// Phase of a single play / match flow.
type Phase int

const (
	PhasePreSnap Phase = iota
	PhasePlaySelect
	PhaseInPlay
	PhaseDeadBall
	PhaseScore
	PhaseGameOver
)

// Match is the top-level football state for one game.
type Match struct {
	HomeScore int
	AwayScore int

	// Quarter 1..4 (MVP may ignore and just run unlimited drives).
	Quarter     int
	ClockSec    int // seconds remaining in quarter
	PlayClock   int

	Possession TeamID
	// BallOn is yards from the possessing team's own goal (0..100) — "ball on the 25".
	// For MVP we use absolute field Y from south; see field package.
	BallY       float64
	Down        int // 1..4
	Distance    float64 // yards to gain
	LineOfScrimmage float64
	FirstDownMarker float64

	Phase Phase

	// Drive stats (MVP telemetry).
	PlayCount int
}

// NewMatch starts a simple half: home ball at own 25, 1st & 10.
func NewMatch() *Match {
	ball := 25.0
	return &Match{
		Quarter:         1,
		ClockSec:        15 * 60,
		PlayClock:       25,
		Possession:      TeamHome,
		BallY:           ball,
		Down:            1,
		Distance:        10,
		LineOfScrimmage: ball,
		FirstDownMarker: ball + 10,
		Phase:           PhasePlaySelect,
	}
}

// YardsToGain returns remaining distance for the chains.
func (m *Match) YardsToGain() float64 {
	return m.Distance
}

// AfterRunResult updates down/distance given yards gained by offense (positive = toward opponent).
// MVP assumes home always drives north (+Y).
func (m *Match) AfterRunResult(yardsGained float64) {
	m.ApplyPlayResult(m.LineOfScrimmage+yardsGained, yardsGained, false, yardsGained >= (100-m.LineOfScrimmage)-0.01)
}

// ApplyPlayResult spots the ball and updates chains.
// incomplete: no gain, ball stays at LOS, down advances.
// touchdown: awards 6 and sets PhaseScore (caller may re-spot).
func (m *Match) ApplyPlayResult(ballY, yardsGained float64, incomplete, touchdown bool) {
	m.PlayCount++
	m.PlayClock = 25

	if touchdown || ballY >= 100 {
		if m.Possession == TeamHome {
			m.HomeScore += 6
		} else {
			m.AwayScore += 6
		}
		m.BallY = 100
		m.Phase = PhaseScore
		return
	}

	if incomplete {
		// Ball stays; lose a down
		m.Down++
		if m.Down > 4 {
			m.turnoverOnDowns()
		}
		m.Phase = PhasePlaySelect
		return
	}

	if ballY < 0 {
		ballY = 0
	}
	if ballY > 100 {
		ballY = 100
	}
	m.BallY = ballY
	m.Distance -= yardsGained

	if m.Distance <= 0 {
		m.Down = 1
		m.Distance = 10
		m.LineOfScrimmage = m.BallY
		m.FirstDownMarker = m.BallY + 10
		if m.FirstDownMarker > 100 {
			m.Distance = 100 - m.BallY
			m.FirstDownMarker = 100
		}
	} else {
		m.Down++
		m.LineOfScrimmage = m.BallY
		if m.Down > 4 {
			m.turnoverOnDowns()
		}
	}
	m.Phase = PhasePlaySelect
}

// SpotKickoffOrDrive resets offense to own 25 after a score (MVP playtesting).
func (m *Match) SpotKickoffOrDrive() {
	m.Phase = PhasePlaySelect
	m.Possession = TeamHome
	m.BallY = 25
	m.Down = 1
	m.Distance = 10
	m.LineOfScrimmage = 25
	m.FirstDownMarker = 35
	m.PlayClock = 25
}

func (m *Match) turnoverOnDowns() {
	// MVP: give ball back to home at midfield for continued playtesting.
	m.Possession = TeamHome
	m.BallY = 40
	m.Down = 1
	m.Distance = 10
	m.LineOfScrimmage = m.BallY
	m.FirstDownMarker = m.BallY + 10
}

// SituationClass is used by AI tendency / play calling.
type SituationClass int

const (
	SitNormal SituationClass = iota
	SitShortYardage  // 3rd/4th & <= 2
	SitThirdLong     // 3rd & >= 7
	SitRedZone       // inside opponent 20
	SitGoalLine      // inside opponent 5
)

func (m *Match) Situation() SituationClass {
	toGoal := 100 - m.BallY
	if toGoal <= 5 {
		return SitGoalLine
	}
	if toGoal <= 20 {
		return SitRedZone
	}
	if m.Down >= 3 && m.Distance <= 2 {
		return SitShortYardage
	}
	if m.Down == 3 && m.Distance >= 7 {
		return SitThirdLong
	}
	return SitNormal
}
