package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Ljferrer/gastown-tower/pkg/tower"
)

// handleStream serves snapshot updates as Server-Sent Events. It emits the
// current snapshot immediately, then polls on the hybrid cadence and emits a
// new event only when the snapshot's meaningful content changes. The loop ends
// when the client disconnects (request context cancellation).
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ctx := r.Context()
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	var last string // fingerprint of the last emitted snapshot
	if err := s.emit(w, flusher, &last); err != nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.emit(w, flusher, &last); err != nil {
				return
			}
		}
	}
}

// emit samples the collector and writes an SSE event when the snapshot's
// content has changed since the last fingerprint in last. A write error
// (typically a disconnected client) is returned so the caller can stop;
// transient snapshot errors are skipped without tearing down the stream.
func (s *Server) emit(w http.ResponseWriter, f http.Flusher, last *string) error {
	snap, err := s.snap.Snapshot()
	if err != nil {
		return nil
	}
	fp := fingerprint(snap)
	if fp == *last {
		return nil
	}
	payload, err := json.Marshal(snap)
	if err != nil {
		return nil
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
		return err
	}
	*last = fp
	f.Flush()
	return nil
}

// fingerprint returns a content signature of the snapshot that ignores the
// GeneratedAt timestamp, so the every-poll clock tick alone does not count as a
// change. It operates on a copy and never mutates the caller's snapshot.
func fingerprint(s tower.Snapshot) string {
	s.GeneratedAt = time.Time{}
	b, err := json.Marshal(s)
	if err != nil {
		return ""
	}
	return string(b)
}
