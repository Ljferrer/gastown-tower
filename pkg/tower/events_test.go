package tower

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeLines appends JSONL lines to path, creating it if needed.
func writeLines(t *testing.T, path string, lines ...string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	for _, ln := range lines {
		if _, err := f.WriteString(ln + "\n"); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
}

func evtLine(ts time.Time, typ, actor string) string {
	return fmt.Sprintf(`{"ts":%q,"source":"gt","type":%q,"actor":%q,"payload":{"bead":"b1"},"visibility":"feed"}`,
		ts.UTC().Format(time.RFC3339), typ, actor)
}

// The ring tails only the bytes appended since the last read and returns events
// newest-first.
func TestEventRingTailIncremental(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".events.jsonl")
	now := time.Unix(1_700_000_000, 0).UTC()
	r := &eventRing{}

	writeLines(t, path, evtLine(now.Add(-2*time.Minute), "sling", "a"))
	first := r.tail(path, now)
	if len(first) != 1 || first[0].Type != "sling" {
		t.Fatalf("first tail: %+v", first)
	}

	// Append two more; a second tail must pick up exactly the new lines.
	writeLines(t, path,
		evtLine(now.Add(-1*time.Minute), "done", "b"),
		evtLine(now.Add(-30*time.Second), "mail", "c"))
	second := r.tail(path, now)
	if len(second) != 3 {
		t.Fatalf("after append, want 3 events, got %d", len(second))
	}
	// Newest-first ordering.
	if second[0].Type != "mail" || second[1].Type != "done" || second[2].Type != "sling" {
		t.Fatalf("not newest-first: %s,%s,%s", second[0].Type, second[1].Type, second[2].Type)
	}
}

// Events older than the 24h cutoff are dropped.
func TestEventRingRetentionCutoff(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".events.jsonl")
	now := time.Unix(1_700_000_000, 0).UTC()
	r := &eventRing{}

	writeLines(t, path,
		evtLine(now.Add(-25*time.Hour), "sling", "old"), // expired
		evtLine(now.Add(-1*time.Hour), "done", "fresh"), // kept
	)
	got := r.tail(path, now)
	if len(got) != 1 || got[0].Actor != "fresh" {
		t.Fatalf("cutoff not applied: %+v", got)
	}
}

// The ring keeps only the most recent eventsCapacity events.
func TestEventRingCapacity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".events.jsonl")
	now := time.Unix(1_700_000_000, 0).UTC()
	r := &eventRing{}

	for i := 0; i < eventsCapacity+50; i++ {
		writeLines(t, path, evtLine(now.Add(-time.Duration(eventsCapacity+50-i)*time.Second), "sling", fmt.Sprintf("a%d", i)))
	}
	got := r.tail(path, now)
	if len(got) != eventsCapacity {
		t.Fatalf("capacity not enforced: got %d, want %d", len(got), eventsCapacity)
	}
	// Newest-first: the very last appended actor leads.
	if got[0].Actor != fmt.Sprintf("a%d", eventsCapacity+49) {
		t.Fatalf("newest event not retained at head: %s", got[0].Actor)
	}
}

// A partial trailing line (mid-write) is not consumed until its newline lands.
func TestEventRingPartialLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".events.jsonl")
	now := time.Unix(1_700_000_000, 0).UTC()
	r := &eventRing{}

	full := evtLine(now, "sling", "a")
	done := evtLine(now, "done", "b")
	cut := 10 // split the second line mid-token
	// Write one complete line plus a partial second line (no newline).
	f, _ := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	f.WriteString(full + "\n" + done[:cut])
	f.Close()

	got := r.tail(path, now)
	if len(got) != 1 {
		t.Fatalf("partial line consumed: got %d events", len(got))
	}

	// Complete the partial line; the next tail picks it up.
	f, _ = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString(done[cut:] + "\n")
	f.Close()

	got = r.tail(path, now)
	if len(got) != 2 {
		t.Fatalf("completed line not consumed: got %d events", len(got))
	}
}

// A file that shrinks (rotation/truncation) resets the ring rather than seeking
// past EOF.
func TestEventRingTruncationReset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".events.jsonl")
	now := time.Unix(1_700_000_000, 0).UTC()
	r := &eventRing{}

	writeLines(t, path,
		evtLine(now.Add(-3*time.Minute), "sling", "a"),
		evtLine(now.Add(-2*time.Minute), "done", "b"))
	if got := r.tail(path, now); len(got) != 2 {
		t.Fatalf("setup tail: got %d", len(got))
	}

	// Truncate and write a single fresh line.
	os.WriteFile(path, []byte(evtLine(now.Add(-1*time.Minute), "mail", "c")+"\n"), 0o644)
	got := r.tail(path, now)
	if len(got) != 1 || got[0].Actor != "c" {
		t.Fatalf("truncation not handled: %+v", got)
	}
}

// A missing file yields no events and no error.
func TestEventRingMissingFile(t *testing.T) {
	r := &eventRing{}
	got := r.tail(filepath.Join(t.TempDir(), "nope.jsonl"), time.Now())
	if len(got) != 0 {
		t.Fatalf("missing file should yield no events, got %d", len(got))
	}
}

// Curated() is the default-flow allowlist: it keeps the meaningful types and the
// escalation_* family, and drops the high-volume session_start.
func TestEventCurated(t *testing.T) {
	keep := []string{"sling", "done", "handoff", "mail", "spawn", "unhook",
		"escalation_sent", "escalation_acked", "escalation_closed"}
	for _, typ := range keep {
		if !(Event{Type: typ}).Curated() {
			t.Errorf("%q should be curated", typ)
		}
	}
	for _, typ := range []string{"session_start", "nudge", "mystery"} {
		if (Event{Type: typ}).Curated() {
			t.Errorf("%q should not be curated", typ)
		}
	}
}
