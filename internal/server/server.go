// Package server exposes a tower.Collector over HTTP: a JSON snapshot
// endpoint and a Server-Sent Events stream of live snapshot updates.
//
// It is a thin transport layer — all snapshot logic lives in pkg/tower and is
// reused here, never duplicated.
package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/Ljferrer/gastown-tower/pkg/tower"
)

// defaultPollInterval is the hybrid-cadence poll period for the SSE stream:
// the collector is sampled this often and an event is emitted only on change.
const defaultPollInterval = 1500 * time.Millisecond

// Snapshotter produces point-in-time snapshots. *tower.Collector satisfies it;
// tests substitute fakes.
type Snapshotter interface {
	Snapshot() (tower.Snapshot, error)
}

// Server wires a Snapshotter to HTTP handlers.
type Server struct {
	snap         Snapshotter
	pollInterval time.Duration
}

// New returns a Server backed by the given Snapshotter.
func New(s Snapshotter) *Server {
	return &Server{snap: s, pollInterval: defaultPollInterval}
}

// Handler builds the HTTP routes: the JSON/SSE APIs plus the embedded web
// assets at "/".
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/snapshot", s.handleSnapshot)
	mux.HandleFunc("/api/stream", s.handleStream)

	if static, err := staticHandler(); err == nil {
		mux.Handle("/", static)
	}
	return mux
}

// handleSnapshot writes the current snapshot as JSON.
func (s *Server) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	snap, err := s.snap.Snapshot()
	if err != nil {
		http.Error(w, "snapshot failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(snap); err != nil {
		// Header already sent; nothing recoverable to write.
		return
	}
}
