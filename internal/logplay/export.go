package logplay

import (
	"bytes"
	"encoding/json"
	"time"
)

// SessionMeta is the first line of an exported film file.
// Gameplay only — no names, emails, IPs, or key events.
type SessionMeta struct {
	Type       string    `json:"type"`
	SessionID  string    `json:"session_id"`
	Build      string    `json:"build"`
	StartedAt  time.Time `json:"started_at"`
	ExportedAt time.Time `json:"exported_at"`
	ElapsedSec float64   `json:"elapsed_sec"`
	Snaps      int       `json:"snaps"`
	LastN      int       `json:"last_n"`
}

// ExportJSONL writes a session envelope line, then one JSON object per snap.
func ExportJSONL(l *Logger, meta SessionMeta) string {
	meta.Type = "session"
	if meta.ExportedAt.IsZero() {
		meta.ExportedAt = time.Now()
	}
	entries := l.Entries()
	meta.Snaps = len(entries)
	if len(entries) > 0 {
		meta.LastN = entries[len(entries)-1].N
	}
	if !meta.StartedAt.IsZero() {
		meta.ElapsedSec = meta.ExportedAt.Sub(meta.StartedAt).Seconds()
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	_ = enc.Encode(meta)
	for _, e := range entries {
		_ = enc.Encode(e)
	}
	return buf.String()
}
