package logplay

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRecordAndSummary(t *testing.T) {
	dir := t.TempDir()
	l, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	l.Record(Entry{OffPlay: "sweep", OffName: "Toss Sweep", DefCall: "base", Outcome: "tackle", Yards: 8})
	l.Record(Entry{OffPlay: "sweep", OffName: "Toss Sweep", DefCall: "run_fit", Outcome: "tackle", Yards: 1})
	got := l.Record(Entry{OffPlay: "slant", OffName: "Slant", DefCall: "base", Outcome: "incomplete", Yards: 0})
	if got.N != 3 {
		t.Fatalf("Record should return the sequence number, got %d", got.N)
	}
	keep := l.Record(Entry{OffPlay: "slant", OffName: "Slant", DefCall: "base", Outcome: "tackle", Yards: 11, QBKeep: true, Carrier: "QB", Thrown: false})
	if !keep.QBKeep || keep.N != 4 {
		t.Fatalf("keep entry: %+v", keep)
	}

	sum := l.SummarizeByPlay()
	if len(sum) != 2 {
		t.Fatalf("expected 2 plays, got %d", len(sum))
	}
	if sum[0].Play != "sweep" || sum[0].N != 2 {
		t.Fatalf("sweep summary: %+v", sum[0])
	}
	if sum[1].Play != "slant" || sum[1].Keep != 1 {
		t.Fatalf("slant keep summary: %+v", sum[1])
	}
	if l.Path() == "" {
		t.Fatal("expected file path")
	}
	if _, err := os.Stat(l.Path()); err != nil {
		t.Fatal(err)
	}
	// latest pointer
	b, err := os.ReadFile(filepath.Join(dir, "playlog_latest_path.txt"))
	if err != nil || len(b) == 0 {
		t.Fatalf("latest path file: %v %q", err, b)
	}
}
