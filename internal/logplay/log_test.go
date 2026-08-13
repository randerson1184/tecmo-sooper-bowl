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
	l.Record(Entry{OffPlay: "slant", OffName: "Slant", DefCall: "base", Outcome: "incomplete", Yards: 0})

	sum := l.SummarizeByPlay()
	if len(sum) != 2 {
		t.Fatalf("expected 2 plays, got %d", len(sum))
	}
	if sum[0].Play != "sweep" || sum[0].N != 2 {
		t.Fatalf("sweep summary: %+v", sum[0])
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
