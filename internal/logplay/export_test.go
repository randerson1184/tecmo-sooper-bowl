package logplay

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestExportJSONLStartsWithSessionEnvelope(t *testing.T) {
	l, err := New("")
	if err != nil {
		t.Fatal(err)
	}
	l.Record(Entry{OffPlay: "hitch", OffName: "Hitch", Outcome: "tackle", Yards: 7})
	l.Record(Entry{OffPlay: "slant", OffName: "Slant", Outcome: "incomplete", Yards: 0})
	start := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	out := ExportJSONL(l, SessionMeta{
		SessionID: "test-ses",
		Build:     "dev",
		StartedAt: start,
	})
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("want 3 lines (meta+2 snaps), got %d\n%s", len(lines), out)
	}
	var meta SessionMeta
	if err := json.Unmarshal([]byte(lines[0]), &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Type != "session" || meta.SessionID != "test-ses" || meta.Snaps != 2 || meta.LastN != 2 {
		t.Fatalf("envelope: %+v", meta)
	}
	var snap Entry
	if err := json.Unmarshal([]byte(lines[1]), &snap); err != nil {
		t.Fatal(err)
	}
	if snap.OffPlay != "hitch" || snap.N != 1 {
		t.Fatalf("first snap: %+v", snap)
	}
}
