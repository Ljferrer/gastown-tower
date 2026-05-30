package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Ljferrer/gastown-tower/pkg/tower"
)

// fakeSnapshotter returns canned snapshots so handlers can be tested without
// touching the filesystem.
type fakeSnapshotter struct {
	snaps []tower.Snapshot
	err   error
	calls int
}

func (f *fakeSnapshotter) Snapshot() (tower.Snapshot, error) {
	if f.err != nil {
		return tower.Snapshot{}, f.err
	}
	i := f.calls
	f.calls++
	if i >= len(f.snaps) {
		i = len(f.snaps) - 1
	}
	return f.snaps[i], nil
}

func sampleSnapshot(at time.Time) tower.Snapshot {
	return tower.Snapshot{
		GeneratedAt: at,
		Groups: []tower.Group{{
			Name: "town",
			Agents: []tower.Agent{{
				AgentRef: tower.AgentRef{Name: "alpha", Group: "town"},
				Churning: true,
				Stats:    tower.TranscriptStats{Model: "claude-opus", Turns: 3},
			}},
		}},
	}
}

func TestSnapshotReturnsValidJSON(t *testing.T) {
	at := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	srv := New(&fakeSnapshotter{snaps: []tower.Snapshot{sampleSnapshot(at)}})

	req := httptest.NewRequest(http.MethodGet, "/api/snapshot", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}

	var got tower.Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if len(got.Groups) != 1 || got.Groups[0].Name != "town" {
		t.Fatalf("unexpected snapshot payload: %+v", got)
	}
	if !got.GeneratedAt.Equal(at) {
		t.Fatalf("GeneratedAt = %v, want %v", got.GeneratedAt, at)
	}
}

func TestSnapshotCollectorErrorIs500(t *testing.T) {
	srv := New(&fakeSnapshotter{err: errors.New("boom")})

	req := httptest.NewRequest(http.MethodGet, "/api/snapshot", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}
