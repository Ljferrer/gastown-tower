package tower

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// Event is one record from the town's .events.jsonl flow log — the same single
// clean JSONL stream that `gt feed` consumes.
type Event struct {
	TS         time.Time
	Source     string
	Type       string
	Actor      string
	Payload    map[string]any
	Visibility string
}

// eventRecord is the on-disk JSON shape of an Event.
type eventRecord struct {
	TS         string         `json:"ts"`
	Source     string         `json:"source"`
	Type       string         `json:"type"`
	Actor      string         `json:"actor"`
	Payload    map[string]any `json:"payload"`
	Visibility string         `json:"visibility"`
}

const (
	// eventsCapacity bounds the in-memory ring; the on-disk log grows unbounded
	// in a busy town, so we keep only the most recent slice.
	eventsCapacity = 500
	// eventsCutoff drops anything older than this regardless of capacity.
	eventsCutoff = 24 * time.Hour
)

// curatedTypes is the default-flow allowlist of meaningful event types. The
// high-volume session_start (today ~half of all events) is deliberately absent;
// 't' in the TUI shows everything. escalation_* is matched by prefix in
// Curated, not listed here.
var curatedTypes = map[string]bool{
	"sling":   true,
	"done":    true,
	"handoff": true,
	"mail":    true,
	"spawn":   true,
	"unhook":  true,
}

// Curated reports whether the event belongs in the default curated flow.
func (e Event) Curated() bool {
	if strings.HasPrefix(e.Type, "escalation_") {
		return true
	}
	return curatedTypes[e.Type]
}

// eventRing tails an append-only JSONL flow log into a bounded, time-windowed
// in-memory buffer. Each tail reads only the bytes appended since the last call
// (mirroring how transcript.go streams JSONL), so a growing log is cheap to
// follow. The zero value is ready to use.
type eventRing struct {
	mu     sync.Mutex
	buf    []Event // chronological (oldest first)
	offset int64   // bytes of complete lines already consumed
}

// tail reads any newly-appended complete lines from path and returns the current
// window newest-first. A missing file, an unreadable file, or a partial trailing
// line all degrade gracefully — they return the events already buffered. A file
// that shrank since the last read (rotation/truncation) is re-read from the top.
func (r *eventRing) tail(path string, now time.Time) []Event {
	r.mu.Lock()
	defer r.mu.Unlock()

	f, err := os.Open(path)
	if err != nil {
		return r.window(now)
	}
	defer f.Close()

	if fi, err := f.Stat(); err == nil && fi.Size() < r.offset {
		r.offset = 0 // truncated or rotated — start over
		r.buf = nil
	}
	if _, err := f.Seek(r.offset, io.SeekStart); err != nil {
		return r.window(now)
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return r.window(now)
	}

	// Consume only up to the last complete line; a trailing partial line (the
	// log may be mid-write) is left for the next tail.
	lastNL := bytes.LastIndexByte(data, '\n')
	if lastNL < 0 {
		return r.window(now)
	}
	complete := data[:lastNL+1]
	r.offset += int64(len(complete))

	for _, line := range bytes.Split(complete, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var rec eventRecord
		if json.Unmarshal(line, &rec) != nil {
			continue // skip malformed lines, like the transcript parser
		}
		ts, err := time.Parse(time.RFC3339, rec.TS)
		if err != nil {
			continue
		}
		r.buf = append(r.buf, Event{
			TS:         ts,
			Source:     rec.Source,
			Type:       rec.Type,
			Actor:      rec.Actor,
			Payload:    rec.Payload,
			Visibility: rec.Visibility,
		})
	}
	return r.window(now)
}

// window trims the buffer to the retention policy and returns a newest-first
// copy. Caller must hold r.mu.
func (r *eventRing) window(now time.Time) []Event {
	r.trim(now)
	out := make([]Event, len(r.buf))
	for i := range r.buf {
		out[len(r.buf)-1-i] = r.buf[i] // reverse → newest first
	}
	return out
}

// trim enforces the 24h cutoff then the capacity cap, keeping the newest events.
// The buffer is chronological, so expired events sit at the front. Caller must
// hold r.mu.
func (r *eventRing) trim(now time.Time) {
	cutoff := now.Add(-eventsCutoff)
	drop := 0
	for drop < len(r.buf) && r.buf[drop].TS.Before(cutoff) {
		drop++
	}
	if drop > 0 {
		r.buf = append(r.buf[:0], r.buf[drop:]...)
	}
	if len(r.buf) > eventsCapacity {
		r.buf = append(r.buf[:0], r.buf[len(r.buf)-eventsCapacity:]...)
	}
}
