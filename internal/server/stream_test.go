package server

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Ljferrer/gastown-tower/pkg/tower"
)

// TestStreamEmitsEventAndClosesOnCancel verifies the SSE handler frames an
// event correctly, then unwinds when the client disconnects.
func TestStreamEmitsEventAndClosesOnCancel(t *testing.T) {
	at := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	srv := New(&fakeSnapshotter{snaps: []tower.Snapshot{sampleSnapshot(at)}})
	srv.pollInterval = 10 * time.Millisecond

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/stream", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}

	// First event should arrive immediately and be SSE-framed.
	data, err := readSSEData(resp.Body)
	if err != nil {
		t.Fatalf("reading first event: %v", err)
	}
	if !strings.Contains(data, `"town"`) {
		t.Fatalf("event payload missing snapshot content: %q", data)
	}

	// Disconnecting must end the handler: the body read returns an error.
	cancel()
	done := make(chan error, 1)
	go func() {
		_, err := resp.Body.Read(make([]byte, 1))
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected read error after cancel, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not close after client disconnect")
	}
}

// readSSEData reads lines until a "data:" field, returning its value.
func readSSEData(r interface{ Read([]byte) (int, error) }) (string, error) {
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return "", err
		}
		if v, ok := strings.CutPrefix(line, "data:"); ok {
			return strings.TrimSpace(v), nil
		}
	}
}
