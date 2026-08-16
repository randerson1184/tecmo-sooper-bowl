// Package logplay records each snap for balance tuning and post-drive review.
package logplay

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry is one resolved play.
type Entry struct {
	N          int       `json:"n"`
	Time       time.Time `json:"time"`
	OffPlay    string    `json:"off_play"` // inside_zone, sweep, slant, hitch
	OffName    string    `json:"off_name"`
	DefCall    string    `json:"def_call"` // base, run_fit, ... (front)
	Shell      string    `json:"shell"`    // cover3, cover2, man_free
	Outcome    string    `json:"outcome"`  // tackle, incomplete, td, ...
	Yards      float64   `json:"yards"`
	DownBefore int       `json:"down_before"`
	DistBefore float64   `json:"dist_before"`
	BallBefore float64   `json:"ball_before"`
	DownAfter  int       `json:"down_after"`
	DistAfter  float64   `json:"dist_after"`
	BallAfter  float64   `json:"ball_after"`
	RunPct     float64   `json:"run_pct"`
	PassPct    float64   `json:"pass_pct"`
	RightPct   float64   `json:"right_pct"`
	Stamina    float64   `json:"stamina"` // 0..1 display (1 = fresh)
	Thrown     bool      `json:"thrown"`
	Carrier    string    `json:"carrier,omitempty"`
	QBKeep     bool      `json:"qb_keep"`
	KeepThreat float64   `json:"keep_threat"`
	KeepN      int       `json:"keep_n"`
	Message    string    `json:"message"`

	// Play-action film. Empty mesh on non-PA snaps.
	RunThreat   float64 `json:"run_threat"`
	BiteSec     float64 `json:"bite_sec"`
	LeftoverSec float64 `json:"leftover_sec"`
	ReleaseAt   float64 `json:"release_at"`
	Mesh        string  `json:"mesh,omitempty"`
	BiterN      int     `json:"biter_n"`
	Biters      string  `json:"biters,omitempty"`
	SepAtThrow  float64 `json:"sep_at_throw"`
}

// Logger appends entries to memory and optionally a JSONL file.
type Logger struct {
	mu      sync.Mutex
	entries []Entry
	path    string
	file    *os.File
	seq     int
}

// New creates a logger. If dir is non-empty, writes playlog.jsonl there.
func New(dir string) (*Logger, error) {
	l := &Logger{}
	if dir == "" {
		return l, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return l, err
	}
	// Timestamped file so sessions don't clobber each other
	name := fmt.Sprintf("playlog_%s.jsonl", time.Now().Format("20060102_150405"))
	path := filepath.Join(dir, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return l, err
	}
	// Also maintain a stable "latest" symlink-ish copy name
	latest := filepath.Join(dir, "playlog_latest.jsonl")
	_ = os.Remove(latest)
	// Best-effort: copy path into a fixed name by opening second handle
	// (simpler: just write the same path note)
	l.path = path
	l.file = f

	// Write a one-line pointer file for tools
	_ = os.WriteFile(filepath.Join(dir, "playlog_latest_path.txt"), []byte(path+"\n"), 0o644)
	return l, nil
}

// Path returns the JSONL file path (empty if memory-only).
func (l *Logger) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

// Record appends one play result and returns it with the sequence number set.
func (l *Logger) Record(e Entry) Entry {
	if l == nil {
		return e
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.seq++
	e.N = l.seq
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	l.entries = append(l.entries, e)
	if l.file != nil {
		b, err := json.Marshal(e)
		if err == nil {
			_, _ = l.file.Write(append(b, '\n'))
		}
	}
	return e
}

// Entries returns a copy of all recorded plays this session.
func (l *Logger) Entries() []Entry {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Entry, len(l.entries))
	copy(out, l.entries)
	return out
}

// Summary is aggregate stats by offensive play ID.
type Summary struct {
	Play     string
	N        int
	AvgYards float64
	Success  int // 1st down gain proxy: yards >= dist, or TD, or complete for positive
	TD       int
	Incomp   int
	Sack     int
	Stuff    int // < 2 yards on runs / sacks
	Keep     int
	KeepYds  float64
}

// SummarizeByPlay rolls up the session.
func (l *Logger) SummarizeByPlay() []Summary {
	entries := l.Entries()
	order := []string{}
	by := map[string]*Summary{}
	for _, e := range entries {
		s, ok := by[e.OffPlay]
		if !ok {
			s = &Summary{Play: e.OffPlay}
			by[e.OffPlay] = s
			order = append(order, e.OffPlay)
		}
		s.N++
		s.AvgYards += e.Yards
		if e.QBKeep {
			s.Keep++
			s.KeepYds += e.Yards
		}
		switch e.Outcome {
		case "touchdown":
			s.TD++
			s.Success++
		case "incomplete":
			s.Incomp++
		case "sack":
			s.Sack++
			s.Stuff++
		default:
			if e.Yards >= 4 {
				s.Success++
			}
			if e.Yards < 2 {
				s.Stuff++
			}
		}
	}
	out := make([]Summary, 0, len(order))
	for _, id := range order {
		s := by[id]
		if s.N > 0 {
			s.AvgYards /= float64(s.N)
		}
		out = append(out, *s)
	}
	return out
}

// FormatSummary is a multi-line HUD / terminal report.
func (l *Logger) FormatSummary() string {
	if l == nil {
		return "no play log"
	}
	sum := l.SummarizeByPlay()
	if len(sum) == 0 {
		return "No plays logged yet."
	}
	lines := "PLAY LOG SUMMARY\n"
	for _, s := range sum {
		keepAvg := 0.0
		if s.Keep > 0 {
			keepAvg = s.KeepYds / float64(s.Keep)
		}
		lines += fmt.Sprintf("  %-12s n=%2d  avg=%+5.1f  ok=%d  td=%d  inc=%d  sack=%d  stuff=%d  keep=%d (avg=%+.1f)\n",
			s.Play, s.N, s.AvgYards, s.Success, s.TD, s.Incomp, s.Sack, s.Stuff, s.Keep, keepAvg)
	}
	if p := l.Path(); p != "" {
		lines += "file: " + p
	}
	return lines
}

// Close flushes the file handle.
func (l *Logger) Close() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
}

// Last returns the most recent entry and true, or false if empty.
func (l *Logger) Last() (Entry, bool) {
	if l == nil {
		return Entry{}, false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.entries) == 0 {
		return Entry{}, false
	}
	return l.entries[len(l.entries)-1], true
}
