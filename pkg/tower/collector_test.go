package tower

import (
	"errors"
	"testing"
	"time"
)

// Agents must keep a stable position regardless of churn state, and town must
// sort before rigs.
func TestAssembleStableOrder(t *testing.T) {
	now := time.Now()
	byGroup := map[string][]Agent{
		"town": {
			{AgentRef: AgentRef{Name: "zeta", Group: "town"}, Churning: true},   // churning, name-last
			{AgentRef: AgentRef{Name: "alpha", Group: "town"}, Churning: false}, // idle, name-first
		},
		"GigaClip": {
			{AgentRef: AgentRef{Name: "witness", Group: "GigaClip"}},
		},
	}
	snap := assemble(byGroup, now)

	if len(snap.Groups) != 2 || snap.Groups[0].Name != "town" || snap.Groups[1].Name != "GigaClip" {
		t.Fatalf("groups not ordered town-first: %+v", snap.Groups)
	}
	town := snap.Groups[0].Agents
	if town[0].Name != "alpha" || town[1].Name != "zeta" {
		t.Fatalf("agents reordered by churn; got %s,%s want alpha,zeta", town[0].Name, town[1].Name)
	}
}

// assemble rolls up town-wide stats: total active, churning subset, and a
// per-group breakdown that mirrors the (town-first) group ordering.
func TestAssembleTownStats(t *testing.T) {
	now := time.Now()
	byGroup := map[string][]Agent{
		"town": {
			{AgentRef: AgentRef{Name: "mayor", Group: "town"}, Churning: true},
			{AgentRef: AgentRef{Name: "deacon", Group: "town"}, Churning: false},
		},
		"GigaClip": {
			{AgentRef: AgentRef{Name: "witness", Group: "GigaClip"}, Churning: true},
			{AgentRef: AgentRef{Name: "furiosa", Group: "GigaClip"}, Churning: true},
		},
	}
	snap := assemble(byGroup, now)

	if snap.Stats.Active != 4 {
		t.Errorf("Active = %d, want 4", snap.Stats.Active)
	}
	if snap.Stats.Churning != 3 {
		t.Errorf("Churning = %d, want 3", snap.Stats.Churning)
	}
	if len(snap.Stats.Rigs) != 2 {
		t.Fatalf("Rigs = %d, want 2", len(snap.Stats.Rigs))
	}
	// town sorts first, mirroring Groups ordering.
	if got := snap.Stats.Rigs[0]; got.Name != "town" || got.Active != 2 || got.Churning != 1 {
		t.Errorf("Rigs[0] = %+v, want {town 2 1}", got)
	}
	if got := snap.Stats.Rigs[1]; got.Name != "GigaClip" || got.Active != 2 || got.Churning != 2 {
		t.Errorf("Rigs[1] = %+v, want {GigaClip 2 2}", got)
	}
}

// Enrichment is fetched once and reused within the TTL, then refetched after it
// expires. The slow gt/bd shell-outs must not run on every 1.5s transcript pass.
func TestFetchEnrichmentTTLCache(t *testing.T) {
	clock := time.Unix(1_700_000_000, 0)
	var repCalls, mailCalls, hookCalls int
	c := &Collector{
		EnrichTTL: 15 * time.Second,
		now:       func() time.Time { return clock },
		loadReputation: func(string) (Reputation, error) {
			repCalls++
			return Reputation{Tier: "Bronze", Stamps: 7}, nil
		},
		loadMail: func(string) (Mail, error) {
			mailCalls++
			return Mail{Total: 5, Unread: 2}, nil
		},
		loadHooks: func(string) (map[string]string, error) {
			hookCalls++
			return map[string]string{"GasTownTower/polecats/furiosa": "gtt-vsq: Slice 3"}, nil
		},
	}

	// First fetch populates the cache.
	en := c.fetchEnrichment(clock)
	if en.town.Reputation.Tier != "Bronze" || en.town.Mail.Unread != 2 {
		t.Fatalf("unexpected enrichment: %+v", en.town)
	}
	if en.hooks["GasTownTower/polecats/furiosa"] != "gtt-vsq: Slice 3" {
		t.Fatalf("hook not loaded: %+v", en.hooks)
	}

	// Within TTL: no re-fetch.
	clock = clock.Add(14 * time.Second)
	c.fetchEnrichment(clock)
	if repCalls != 1 || mailCalls != 1 || hookCalls != 1 {
		t.Fatalf("re-fetched within TTL: rep=%d mail=%d hook=%d", repCalls, mailCalls, hookCalls)
	}

	// Past TTL: exactly one re-fetch.
	clock = clock.Add(2 * time.Second) // total +16s
	c.fetchEnrichment(clock)
	if repCalls != 2 || mailCalls != 2 || hookCalls != 2 {
		t.Fatalf("expected one re-fetch after TTL: rep=%d mail=%d hook=%d", repCalls, mailCalls, hookCalls)
	}
}

// Churn is driven by the live pane working-indicator, not transcript mtime: a
// long-generating turn (stale mtime) still reads as churning when its pane shows
// the marker, and a quiet pane reads idle even if the transcript was just touched.
func TestChurnStateFromPane(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	staleMtime := now.Add(-5 * time.Minute) // far outside ChurnWindow
	freshMtime := now.Add(-1 * time.Second) // inside ChurnWindow
	prefixes := map[string]string{".": "hq"}
	sessions := map[string]struct{}{"hq-mayor": {}}
	ref := AgentRef{Name: "mayor", Group: townGroup}

	working := "✳ Honking… (9s · ↓ 475 tokens)\n  esc to interrupt"
	idle := "❯ done\n  ⏵⏵ bypass permissions on · ← for agents"

	c := &Collector{ChurnWindow: 8 * time.Second}

	// Working pane overrides a stale mtime.
	c.capturePane = func(string) (string, error) { return working, nil }
	churning, tp := c.churnState(ref, prefixes, sessions, staleMtime, now, false)
	if !churning {
		t.Error("working pane with stale mtime should read as churning")
	}
	if tp.Tokens != 475 || tp.Elapsed != 9*time.Second {
		t.Errorf("turn progress not parsed: %+v", tp)
	}

	// Idle pane overrides a fresh mtime.
	c.capturePane = func(string) (string, error) { return idle, nil }
	if churning, _ := c.churnState(ref, prefixes, sessions, freshMtime, now, false); churning {
		t.Error("idle pane with fresh mtime should read as idle")
	}
}

// When no pane is available (unknown rig, dead session, or capture error), churn
// falls back to the transcript heuristic. A completed turn (midTurn=false) uses
// the short ChurnWindow quiet window.
func TestChurnStateFallsBackToMtime(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	fresh := now.Add(-2 * time.Second)
	stale := now.Add(-1 * time.Minute)
	prefixes := map[string]string{".": "hq"}
	ref := AgentRef{Name: "mayor", Group: townGroup}

	c := &Collector{
		ChurnWindow: 8 * time.Second,
		TurnWindow:  5 * time.Minute,
		capturePane: func(string) (string, error) { return "", errors.New("no pane") },
	}

	// Session not in the live set -> mtime fallback (fresh = churning).
	if churning, _ := c.churnState(ref, prefixes, map[string]struct{}{}, fresh, now, false); !churning {
		t.Error("missing session with fresh mtime should fall back to churning")
	}
	// Capture error -> mtime fallback (stale = idle).
	sessions := map[string]struct{}{"hq-mayor": {}}
	if churning, _ := c.churnState(ref, prefixes, sessions, stale, now, false); churning {
		t.Error("capture error with stale mtime should fall back to idle")
	}
	// Unknown rig (no prefix) -> mtime fallback.
	ghost := AgentRef{Name: "x", Rig: "Nope", Group: "Nope"}
	if churning, _ := c.churnState(ghost, prefixes, sessions, fresh, now, false); !churning {
		t.Error("unknown rig with fresh mtime should fall back to churning")
	}
}

// A mid-turn transcript (long generation, blocking tool call, inter-turn wait)
// goes quiet on disk while the agent is busy. With no pane, the fallback must
// honor the longer TurnWindow so such an agent stays churning past the short
// ChurnWindow, while a session that has been quiet beyond TurnWindow still drops.
func TestChurnStateMidTurnUsesTurnWindow(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	prefixes := map[string]string{".": "hq"}
	ref := AgentRef{Name: "mayor", Group: townGroup}
	sessions := map[string]struct{}{"hq-mayor": {}}

	c := &Collector{
		ChurnWindow: 8 * time.Second,
		TurnWindow:  5 * time.Minute,
		capturePane: func(string) (string, error) { return "", errors.New("no pane") },
	}

	// Quiet for 1 minute: past ChurnWindow but mid-turn -> churning.
	midGen := now.Add(-1 * time.Minute)
	if churning, _ := c.churnState(ref, prefixes, sessions, midGen, now, true); !churning {
		t.Error("mid-turn agent quiet within TurnWindow should read as churning")
	}
	// Same staleness with a completed turn -> idle (only TurnWindow rescues it).
	if churning, _ := c.churnState(ref, prefixes, sessions, midGen, now, false); churning {
		t.Error("completed turn beyond ChurnWindow should read as idle")
	}
	// Mid-turn but quiet beyond TurnWindow (likely a dead session) -> idle.
	dead := now.Add(-6 * time.Minute)
	if churning, _ := c.churnState(ref, prefixes, sessions, dead, now, true); churning {
		t.Error("mid-turn quiet beyond TurnWindow should read as idle")
	}
}

// A failing loader must not abort enrichment — the others still populate, and
// the timestamp still advances so we don't hammer the failing command.
func TestFetchEnrichmentBestEffort(t *testing.T) {
	clock := time.Unix(1_700_000_000, 0)
	c := &Collector{
		EnrichTTL: 15 * time.Second,
		now:       func() time.Time { return clock },
		loadReputation: func(string) (Reputation, error) {
			return Reputation{}, errors.New("gt unavailable")
		},
		loadMail: func(string) (Mail, error) {
			return Mail{Total: 3, Unread: 1}, nil
		},
		loadHooks: func(string) (map[string]string, error) {
			return nil, errors.New("bd down")
		},
	}
	en := c.fetchEnrichment(clock)
	if en.town.Reputation.Tier != "" {
		t.Errorf("reputation should be empty on error, got %+v", en.town.Reputation)
	}
	if en.town.Mail.Total != 3 {
		t.Errorf("mail should populate despite other failures, got %+v", en.town.Mail)
	}
	if en.hooks == nil {
		t.Error("hooks map should be non-nil even when loader fails")
	}
	if c.enrichAt.IsZero() {
		t.Error("cache timestamp should advance to avoid hammering failing commands")
	}
}
