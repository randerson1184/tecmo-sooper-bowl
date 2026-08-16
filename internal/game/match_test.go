package game

import "testing"

func TestApplyPlayResultTable(t *testing.T) {
	type want struct {
		down, score int
		dist, ball  float64
		phase       Phase
	}
	cases := []struct {
		name                  string
		down                  int
		dist, los             float64
		ballY, yards          float64
		incomplete, touchdown bool
		want                  want
	}{
		{
			name: "gain 4 on 1st and 10",
			down: 1, dist: 10, los: 25,
			ballY: 29, yards: 4,
			want: want{down: 2, dist: 6, ball: 29, phase: PhasePlaySelect},
		},
		{
			name: "conversion resets the chains",
			down: 1, dist: 10, los: 25,
			ballY: 36, yards: 11,
			want: want{down: 1, dist: 10, ball: 36, phase: PhasePlaySelect},
		},
		{
			name: "incomplete loses a down, ball stays",
			down: 3, dist: 5, los: 40,
			ballY: 40, yards: 0, incomplete: true,
			want: want{down: 4, dist: 5, ball: 40, phase: PhasePlaySelect},
		},
		{
			name: "failed fourth down turns over to the 40",
			down: 4, dist: 2, los: 48,
			ballY: 48, yards: 0, incomplete: true,
			want: want{down: 1, dist: 10, ball: 40, phase: PhasePlaySelect},
		},
		{
			name: "stuffed fourth down run turns over",
			down: 4, dist: 2, los: 48,
			ballY: 49, yards: 1,
			want: want{down: 1, dist: 10, ball: 40, phase: PhasePlaySelect},
		},
		{
			name: "fourth down conversion",
			down: 4, dist: 2, los: 48,
			ballY: 51, yards: 3,
			want: want{down: 1, dist: 10, ball: 51, phase: PhasePlaySelect},
		},
		{
			name: "touchdown scores and stops the drive",
			down: 2, dist: 8, los: 94,
			ballY: 100, yards: 6, touchdown: true,
			want: want{down: 2, dist: 8, ball: 100, score: 6, phase: PhaseScore},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewMatch()
			m.Down = tc.down
			m.Distance = tc.dist
			m.BallY = tc.los
			m.LineOfScrimmage = tc.los
			m.FirstDownMarker = tc.los + tc.dist
			m.ApplyPlayResult(tc.ballY, tc.yards, tc.incomplete, tc.touchdown)

			if m.Down != tc.want.down {
				t.Errorf("Down = %d, want %d", m.Down, tc.want.down)
			}
			if m.Distance != tc.want.dist {
				t.Errorf("Distance = %v, want %v", m.Distance, tc.want.dist)
			}
			if m.BallY != tc.want.ball {
				t.Errorf("BallY = %v, want %v", m.BallY, tc.want.ball)
			}
			if m.HomeScore != tc.want.score {
				t.Errorf("HomeScore = %d, want %d", m.HomeScore, tc.want.score)
			}
			if m.Phase != tc.want.phase {
				t.Errorf("Phase = %v, want %v", m.Phase, tc.want.phase)
			}
		})
	}
}

func TestSituationFourthAndLongIsObviousPass(t *testing.T) {
	m := NewMatch()
	m.Down = 4
	m.Distance = 8
	m.BallY = 40
	if !m.LongDown() {
		t.Fatal("4th & 8 should be a long down")
	}
	if m.Situation() != SitThirdLong {
		t.Fatalf("4th & 8 Situation = %v, want SitThirdLong", m.Situation())
	}
	m.Down = 3
	m.Distance = 9
	if m.Situation() != SitThirdLong {
		t.Fatalf("3rd & 9 Situation = %v, want SitThirdLong", m.Situation())
	}
	m.Down = 2
	m.Distance = 12
	if m.LongDown() {
		t.Fatal("2nd & 12 is not an obvious passing down")
	}
}
