package tower

import (
	"errors"
	"os"
	"path/filepath"
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

	c := &Collector{ChurnWindow: 8 * time.Second, ActiveWindow: 30 * time.Minute}

	// Working pane overrides a stale mtime.
	c.capturePane = func(string) (string, error) { return working, nil }
	status, tp := c.agentStatus(ref, prefixes, sessions, staleMtime, now, TranscriptStats{})
	if status != StatusChurning {
		t.Errorf("working pane with stale mtime should read churning, got %v", status)
	}
	if tp.Tokens != 475 || tp.Elapsed != 9*time.Second {
		t.Errorf("turn progress not parsed: %+v", tp)
	}

	// Idle pane (no awaiting signals) overrides a fresh mtime.
	c.capturePane = func(string) (string, error) { return idle, nil }
	if status, _ := c.agentStatus(ref, prefixes, sessions, freshMtime, now, TranscriptStats{}); status != StatusIdle {
		t.Errorf("idle pane with fresh mtime should read idle, got %v", status)
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
		ChurnWindow:  8 * time.Second,
		TurnWindow:   5 * time.Minute,
		ActiveWindow: 30 * time.Minute,
		capturePane:  func(string) (string, error) { return "", errors.New("no pane") },
	}

	// Session not in the live set -> mtime fallback (fresh = churning).
	if status, _ := c.agentStatus(ref, prefixes, map[string]struct{}{}, fresh, now, TranscriptStats{}); status != StatusChurning {
		t.Errorf("missing session with fresh mtime should fall back to churning, got %v", status)
	}
	// Capture error -> mtime fallback (stale = idle).
	sessions := map[string]struct{}{"hq-mayor": {}}
	if status, _ := c.agentStatus(ref, prefixes, sessions, stale, now, TranscriptStats{}); status == StatusChurning {
		t.Errorf("capture error with stale mtime should fall back to idle, got %v", status)
	}
	// Unknown rig (no prefix) -> mtime fallback.
	ghost := AgentRef{Name: "x", Rig: "Nope", Group: "Nope"}
	if status, _ := c.agentStatus(ghost, prefixes, sessions, fresh, now, TranscriptStats{}); status != StatusChurning {
		t.Errorf("unknown rig with fresh mtime should fall back to churning, got %v", status)
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
		ChurnWindow:  8 * time.Second,
		TurnWindow:   5 * time.Minute,
		ActiveWindow: 30 * time.Minute,
		capturePane:  func(string) (string, error) { return "", errors.New("no pane") },
	}

	// Quiet for 1 minute: past ChurnWindow but mid-turn -> churning.
	midGen := now.Add(-1 * time.Minute)
	if status, _ := c.agentStatus(ref, prefixes, sessions, midGen, now, TranscriptStats{MidTurn: true}); status != StatusChurning {
		t.Errorf("mid-turn agent quiet within TurnWindow should read churning, got %v", status)
	}
	// Same staleness with a completed turn -> idle (only TurnWindow rescues it).
	if status, _ := c.agentStatus(ref, prefixes, sessions, midGen, now, TranscriptStats{}); status == StatusChurning {
		t.Errorf("completed turn beyond ChurnWindow should read idle, got %v", status)
	}
	// Mid-turn but quiet beyond TurnWindow (likely a dead session) -> idle.
	dead := now.Add(-6 * time.Minute)
	if status, _ := c.agentStatus(ref, prefixes, sessions, dead, now, TranscriptStats{MidTurn: true}); status == StatusChurning {
		t.Errorf("mid-turn quiet beyond TurnWindow should read idle, got %v", status)
	}
}

// Awaiting-overseer is detected for eligible roles via the live pane: an idle
// pane showing a numbered selection box reads orange, while a working pane stays
// green even when the transcript carries a pending-ask signal.
func TestAgentStatusAwaitingFromPane(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	fresh := now.Add(-2 * time.Second)
	prefixes := map[string]string{".": "hq"}
	sessions := map[string]struct{}{"hq-mayor": {}}
	ref := AgentRef{Name: "mayor", Role: "mayor", Group: townGroup}

	c := &Collector{ChurnWindow: 8 * time.Second, TurnWindow: 5 * time.Minute, ActiveWindow: 30 * time.Minute}

	// Idle pane with a selection box -> awaiting.
	c.capturePane = func(string) (string, error) {
		return "Proceed?\n❯ 1. Yes\n  2. No\n  ⏵⏵ bypass permissions on", nil
	}
	if status, _ := c.agentStatus(ref, prefixes, sessions, fresh, now, TranscriptStats{}); status != StatusAwaiting {
		t.Errorf("selection-box pane should read awaiting, got %v", status)
	}

	// Working pane wins over a pending-ask transcript signal.
	c.capturePane = func(string) (string, error) {
		return "✳ Honking… (9s · ↓ 475 tokens)\n  esc to interrupt", nil
	}
	if status, _ := c.agentStatus(ref, prefixes, sessions, fresh, now, TranscriptStats{PendingAsk: true}); status != StatusChurning {
		t.Errorf("working pane should read churning despite PendingAsk, got %v", status)
	}
}

// In the no-pane fallback, an eligible+fresh agent with a transcript awaiting
// signal reads orange, taking precedence over the mtime churn window. The role
// gate and freshness gate both suppress it.
func TestAgentStatusAwaitingFallback(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	fresh := now.Add(-2 * time.Second)       // within ChurnWindow
	decayed := now.Add(-45 * time.Minute)    // beyond ActiveWindow
	prefixes := map[string]string{".": "hq"} // mayor resolves to hq-mayor
	noSessions := map[string]struct{}{}      // force the no-pane fallback
	mayor := AgentRef{Name: "mayor", Role: "mayor", Group: townGroup}
	polecat := AgentRef{Name: "furiosa", Role: "polecat", Rig: "GigaClip", Group: "GigaClip"}

	c := &Collector{
		ChurnWindow:  8 * time.Second,
		TurnWindow:   5 * time.Minute,
		ActiveWindow: 30 * time.Minute,
		capturePane:  func(string) (string, error) { return "", errors.New("no pane") },
	}

	// Eligible + fresh + pending ask -> awaiting, beating the churn window.
	if status, _ := c.agentStatus(mayor, prefixes, noSessions, fresh, now, TranscriptStats{PendingAsk: true}); status != StatusAwaiting {
		t.Errorf("eligible fresh pending-ask should read awaiting, got %v", status)
	}
	// Trailing question is also an awaiting signal.
	if status, _ := c.agentStatus(mayor, prefixes, noSessions, fresh, now, TranscriptStats{TrailingQuestion: true}); status != StatusAwaiting {
		t.Errorf("eligible fresh trailing-question should read awaiting, got %v", status)
	}
	// Role gate: a polecat with the same signal is never awaiting (fresh -> churning).
	if status, _ := c.agentStatus(polecat, prefixes, noSessions, fresh, now, TranscriptStats{PendingAsk: true}); status == StatusAwaiting {
		t.Errorf("ineligible polecat must not read awaiting, got %v", status)
	}
	// Freshness gate: past ActiveWindow the awaiting signal decays to idle.
	if status, _ := c.agentStatus(mayor, prefixes, noSessions, decayed, now, TranscriptStats{PendingAsk: true}); status != StatusIdle {
		t.Errorf("decayed pending-ask should read idle, got %v", status)
	}
}

// provablyDead is the liveness gate that decides presence: an agent is provably
// dead only when the tmux query SUCCEEDED (non-nil set), its session name
// RESOLVES (known rig), and that name is ABSENT from the set. A nil set (tmux
// hiccup) or an unresolvable rig leaves deadness unproven so the caller keeps
// the mtime fallback.
func TestProvablyDead(t *testing.T) {
	prefixes := map[string]string{".": "hq", "GigaClip": "gc"}
	mayor := AgentRef{Name: "mayor", Group: townGroup}                                        // -> hq-mayor
	polecat := AgentRef{Name: "furiosa", Role: "polecat", Rig: "GigaClip", Group: "GigaClip"} // -> gc-furiosa
	crew := AgentRef{Name: "Quasimodo", Role: "crew", Rig: "GigaClip", Group: "GigaClip"}     // -> gc-crew-Quasimodo
	seat := AgentRef{Name: "Mary", Role: "seat", Rig: "GigaClip", Group: "GigaClip"}          // -> gc-seat-mary
	unknown := AgentRef{Name: "x", Rig: "Nope", Group: "Nope"}                                // unresolvable

	tests := []struct {
		name     string
		ref      AgentRef
		sessions map[string]struct{}
		want     bool
	}{
		{"non-nil set omits session -> dead", polecat, map[string]struct{}{"gc-other": {}}, true},
		{"empty non-nil set -> dead", polecat, map[string]struct{}{}, true},
		{"session present -> alive", polecat, map[string]struct{}{"gc-furiosa": {}}, false},
		{"crew session present -> alive", crew, map[string]struct{}{"gc-crew-Quasimodo": {}}, false},
		{"crew misnamed (no role qualifier) -> dead", crew, map[string]struct{}{"gc-Quasimodo": {}}, true},
		{"seat session present (lowercased) -> alive", seat, map[string]struct{}{"gc-seat-mary": {}}, false},
		{"seat matched with dir case -> dead", seat, map[string]struct{}{"gc-seat-Mary": {}}, true},
		{"nil set (tmux hiccup) -> not provable", polecat, nil, false},
		{"unknown rig -> not provable", unknown, map[string]struct{}{}, false},
		{"town agent omitted -> dead", mayor, map[string]struct{}{"hq-other": {}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := provablyDead(tt.ref, prefixes, tt.sessions); got != tt.want {
				t.Errorf("provablyDead = %v, want %v", got, tt.want)
			}
		})
	}
}

// provablyLive is the affirmative liveness check that exempts confirmed-live
// agents from the ActiveWindow mtime cull: true only when the tmux query
// SUCCEEDED (non-nil set), the session name RESOLVES (known rig), and that name
// is PRESENT. A nil set (tmux hiccup) or an unresolvable rig leaves liveness
// unproven. It is NOT the negation of provablyDead: when liveness is unprovable,
// both return false.
func TestProvablyLive(t *testing.T) {
	prefixes := map[string]string{".": "hq", "GigaClip": "gc"}
	mayor := AgentRef{Name: "mayor", Group: townGroup}                                        // -> hq-mayor
	polecat := AgentRef{Name: "furiosa", Role: "polecat", Rig: "GigaClip", Group: "GigaClip"} // -> gc-furiosa
	crew := AgentRef{Name: "Quasimodo", Role: "crew", Rig: "GigaClip", Group: "GigaClip"}     // -> gc-crew-Quasimodo
	unknown := AgentRef{Name: "x", Rig: "Nope", Group: "Nope"}                                // unresolvable

	tests := []struct {
		name     string
		ref      AgentRef
		sessions map[string]struct{}
		want     bool
	}{
		{"session present -> live", polecat, map[string]struct{}{"gc-furiosa": {}}, true},
		{"crew session present -> live", crew, map[string]struct{}{"gc-crew-Quasimodo": {}}, true},
		{"town agent present -> live", mayor, map[string]struct{}{"hq-mayor": {}}, true},
		{"non-nil set omits session -> not live", polecat, map[string]struct{}{"gc-other": {}}, false},
		{"empty non-nil set -> not live", polecat, map[string]struct{}{}, false},
		{"nil set (tmux hiccup) -> not provable", polecat, nil, false},
		{"unknown rig -> not provable", unknown, map[string]struct{}{"gc-furiosa": {}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := provablyLive(tt.ref, prefixes, tt.sessions); got != tt.want {
				t.Errorf("provablyLive = %v, want %v", got, tt.want)
			}
		})
	}
}

// writeTranscriptDir creates a project transcript dir for an agent at the given
// relative path segments (below the town slug) with a single fresh *.jsonl, so
// Snapshot discovers it. Returns nothing; fails the test on error.
func writeTranscriptDir(t *testing.T, projectsDir, townSlug, relName string) {
	t.Helper()
	dir := filepath.Join(projectsDir, townSlug+"-"+relName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "session.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Liveness gates PRESENCE: after a bulk `gt down`, transcripts keep fresh mtimes
// but the sessions are gone. A non-nil live set that omits an agent's resolved
// tmux session must drop that agent from Active/busy. A nil set (tmux hiccup) or
// an unresolvable rig preserves the pure mtime fallback so a transient failure
// never blanks the tower.
func TestSnapshotDropsDeadAgents(t *testing.T) {
	townRoot := "/town"
	townSlug := slugify(townRoot)
	prefixes := map[string]string{"GigaClip": "gc"}

	newCollector := func(projectsDir string, sessions map[string]struct{}) *Collector {
		return &Collector{
			TownRoot:       townRoot,
			ProjectsDir:    projectsDir,
			ChurnWindow:    8 * time.Second,
			TurnWindow:     5 * time.Minute,
			ActiveWindow:   30 * time.Minute,
			EnrichTTL:      15 * time.Second,
			now:            time.Now,
			loadReputation: func(string) (Reputation, error) { return Reputation{}, nil },
			loadMail:       func(string) (Mail, error) { return Mail{}, nil },
			loadHooks:      func(string) (map[string]string, error) { return nil, nil },
			loadPrefixes:   func(string) (map[string]string, error) { return prefixes, nil },
			listSessions:   func() (map[string]struct{}, error) { return sessions, nil },
			capturePane:    func(string) (string, error) { return "", errors.New("no pane") },
		}
	}

	// A live set that includes only gc-alive: gc-dead is provably gone.
	t.Run("non-nil set omitting a session drops it from Active", func(t *testing.T) {
		dir := t.TempDir()
		writeTranscriptDir(t, dir, townSlug, "GigaClip-polecats-alive")
		writeTranscriptDir(t, dir, townSlug, "GigaClip-polecats-dead")
		c := newCollector(dir, map[string]struct{}{"gc-alive": {}})
		snap, err := c.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		if snap.Stats.Active != 1 {
			t.Errorf("Active = %d, want 1 (dead agent must be dropped)", snap.Stats.Active)
		}
		if snap.Stats.Churning != 1 {
			t.Errorf("Churning = %d, want 1 (only the live agent)", snap.Stats.Churning)
		}
	})

	// gtt-qp6 regression: a live crew agent must stay present. gt names crew
	// sessions <prefix>-crew-<name>; the liveness gate must resolve to that and
	// NOT read the agent as dead when its real session is in the live set.
	t.Run("live crew agent stays present (gtt-qp6)", func(t *testing.T) {
		dir := t.TempDir()
		writeTranscriptDir(t, dir, townSlug, "GigaClip-crew-Quasimodo")
		c := newCollector(dir, map[string]struct{}{"gc-crew-Quasimodo": {}})
		snap, err := c.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		if snap.Stats.Active != 1 {
			t.Errorf("Active = %d, want 1 (live crew must not be dropped)", snap.Stats.Active)
		}
	})

	// gtt-urs regression: a live Nun audit seat must appear in the snapshot. Its
	// worktree dir keeps the proper name (seats/Mary) but gt names the session
	// gtt-seat-mary (lowercased), so the liveness gate must resolve to that — else
	// the seat is dropped as dead and stays invisible in the AGENTS panel.
	t.Run("live seat agent stays present (gtt-urs)", func(t *testing.T) {
		dir := t.TempDir()
		writeTranscriptDir(t, dir, townSlug, "GigaClip-seats-Mary-GigaClip")
		c := newCollector(dir, map[string]struct{}{"gc-seat-mary": {}})
		snap, err := c.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		if snap.Stats.Active != 1 {
			t.Fatalf("Active = %d, want 1 (live seat must not be dropped)", snap.Stats.Active)
		}
		// It must surface as a seat row grouped under its rig, named for the Nun.
		var seat *Agent
		for gi := range snap.Groups {
			for ai := range snap.Groups[gi].Agents {
				if snap.Groups[gi].Agents[ai].Role == "seat" {
					seat = &snap.Groups[gi].Agents[ai]
				}
			}
		}
		if seat == nil {
			t.Fatal("no seat agent in snapshot")
		}
		if seat.Name != "Mary" || seat.Group != "GigaClip" {
			t.Errorf("seat = {Name:%q Group:%q}, want {Mary GigaClip}", seat.Name, seat.Group)
		}
	})

	// nil session set (tmux query failed) -> both agents kept via mtime fallback.
	t.Run("nil set preserves the mtime fallback", func(t *testing.T) {
		dir := t.TempDir()
		writeTranscriptDir(t, dir, townSlug, "GigaClip-polecats-alive")
		writeTranscriptDir(t, dir, townSlug, "GigaClip-polecats-dead")
		c := newCollector(dir, nil)
		snap, err := c.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		if snap.Stats.Active != 2 {
			t.Errorf("Active = %d, want 2 (nil set must not drop anyone)", snap.Stats.Active)
		}
	})

	// Unknown rig (no prefix) -> session name unresolvable -> kept on mtime path.
	t.Run("unknown rig stays on the mtime path", func(t *testing.T) {
		dir := t.TempDir()
		writeTranscriptDir(t, dir, townSlug, "Ghost-polecats-spectre")
		c := newCollector(dir, map[string]struct{}{}) // non-nil, but rig unknown
		snap, err := c.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		if snap.Stats.Active != 1 {
			t.Errorf("Active = %d, want 1 (unknown rig must stay on mtime path)", snap.Stats.Active)
		}
	})

	// Bulk-down regression: many fresh transcripts + empty non-nil live set -> 0 active.
	t.Run("bulk down with empty live set yields 0 active", func(t *testing.T) {
		dir := t.TempDir()
		for _, name := range []string{"a", "b", "c", "d", "e"} {
			writeTranscriptDir(t, dir, townSlug, "GigaClip-polecats-"+name)
		}
		c := newCollector(dir, map[string]struct{}{}) // tmux succeeded; nothing alive
		snap, err := c.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		if snap.Stats.Active != 0 {
			t.Errorf("Active = %d, want 0 (all sessions dead after gt down --all)", snap.Stats.Active)
		}
	})
}

// gtt-m70 regression: the ActiveWindow mtime cull must NOT drop a provably-live
// session. Before the fix, an agent idling on its hook past ActiveWindow (30m)
// silently vanished from the tower even though `gt status` still showed it. A
// confirmed-live session stays present (rendered idle); the cull only applies
// when liveness is UNPROVABLE (nil set or unknown rig), preserving the original
// gt-down safety behavior.
func TestSnapshotKeepsLiveIdleAgents(t *testing.T) {
	townRoot := "/town"
	townSlug := slugify(townRoot)
	prefixes := map[string]string{"GigaClip": "gc"}

	// now is pinned 2h ahead of the freshly-written transcripts so every agent is
	// idle well beyond the 30m ActiveWindow.
	newCollector := func(projectsDir string, sessions map[string]struct{}, now time.Time) *Collector {
		return &Collector{
			TownRoot:       townRoot,
			ProjectsDir:    projectsDir,
			ChurnWindow:    8 * time.Second,
			TurnWindow:     5 * time.Minute,
			ActiveWindow:   30 * time.Minute,
			EnrichTTL:      15 * time.Second,
			now:            func() time.Time { return now },
			loadReputation: func(string) (Reputation, error) { return Reputation{}, nil },
			loadMail:       func(string) (Mail, error) { return Mail{}, nil },
			loadHooks:      func(string) (map[string]string, error) { return nil, nil },
			loadPrefixes:   func(string) (map[string]string, error) { return prefixes, nil },
			listSessions:   func() (map[string]struct{}, error) { return sessions, nil },
			capturePane:    func(string) (string, error) { return "", errors.New("no pane") },
		}
	}

	// (1) Provably-live session idle beyond ActiveWindow stays present, rendered
	// idle with its real idle duration.
	t.Run("provably-live idle beyond ActiveWindow stays present", func(t *testing.T) {
		dir := t.TempDir()
		writeTranscriptDir(t, dir, townSlug, "GigaClip-polecats-furiosa")
		now := time.Now().Add(2 * time.Hour) // ~2h past the transcript mtime
		c := newCollector(dir, map[string]struct{}{"gc-furiosa": {}}, now)
		snap, err := c.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		if snap.Stats.Active != 1 {
			t.Fatalf("Active = %d, want 1 (live-but-idle agent must stay visible)", snap.Stats.Active)
		}
		var agent *Agent
		for gi := range snap.Groups {
			for ai := range snap.Groups[gi].Agents {
				if snap.Groups[gi].Agents[ai].Name == "furiosa" {
					agent = &snap.Groups[gi].Agents[ai]
				}
			}
		}
		if agent == nil {
			t.Fatal("no furiosa agent in snapshot")
		}
		if agent.Status != StatusIdle {
			t.Errorf("Status = %v, want idle (live but quiet transcript)", agent.Status)
		}
		if agent.Idle < 90*time.Minute {
			t.Errorf("Idle = %v, want its real (large) idle duration", agent.Idle)
		}
	})

	// (2a) Liveness unprovable via nil session set + idle beyond ActiveWindow ->
	// dropped (regression guard for the original gt-down behavior).
	t.Run("nil set idle beyond ActiveWindow still drops", func(t *testing.T) {
		dir := t.TempDir()
		writeTranscriptDir(t, dir, townSlug, "GigaClip-polecats-furiosa")
		now := time.Now().Add(2 * time.Hour)
		c := newCollector(dir, nil, now) // tmux hiccup: liveness unprovable
		snap, err := c.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		if snap.Stats.Active != 0 {
			t.Errorf("Active = %d, want 0 (unprovable + stale must be culled)", snap.Stats.Active)
		}
	})

	// (2b) Liveness unprovable via unknown rig + idle beyond ActiveWindow -> dropped.
	t.Run("unknown rig idle beyond ActiveWindow still drops", func(t *testing.T) {
		dir := t.TempDir()
		writeTranscriptDir(t, dir, townSlug, "Ghost-polecats-spectre")
		now := time.Now().Add(2 * time.Hour)
		c := newCollector(dir, map[string]struct{}{"gc-furiosa": {}}, now) // non-nil, but rig unknown
		snap, err := c.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		if snap.Stats.Active != 0 {
			t.Errorf("Active = %d, want 0 (unresolvable rig + stale must be culled)", snap.Stats.Active)
		}
	})
}

// Convoys and the merge queue are part of the TTL-cached enrichment: fetched
// once, reused within the TTL, and the merge-queue loader receives the rig set
// derived from the routes prefixes loaded in the same pass.
func TestFetchEnrichmentConvoysAndMQ(t *testing.T) {
	clock := time.Unix(1_700_000_000, 0)
	var convoyCalls, mqCalls int
	var gotRigs []string
	c := &Collector{
		EnrichTTL:      15 * time.Second,
		now:            func() time.Time { return clock },
		loadReputation: func(string) (Reputation, error) { return Reputation{}, nil },
		loadMail:       func(string) (Mail, error) { return Mail{}, nil },
		loadHooks:      func(string) (map[string]string, error) { return nil, nil },
		loadPrefixes: func(string) (map[string]string, error) {
			return map[string]string{".": "hq", "GasTownTower": "gtt"}, nil
		},
		loadConvoys: func(string) ([]Convoy, error) {
			convoyCalls++
			return []Convoy{
				{ID: "cv-open", Status: "open", CreatedAt: clock.Add(-time.Hour)},
				{ID: "cv-stale", Status: "closed", CreatedAt: clock.Add(-48 * time.Hour)},
			}, nil
		},
		loadMergeQueue: func(_ string, rigs []string) ([]MergeRequest, error) {
			mqCalls++
			gotRigs = rigs
			return []MergeRequest{{ID: "mr1", Rig: "GasTownTower", SourceIssue: "gtt-9"}}, nil
		},
	}

	en := c.fetchEnrichment(clock)
	// recentConvoys must have dropped the stale closed convoy.
	if len(en.convoys) != 1 || en.convoys[0].ID != "cv-open" {
		t.Fatalf("convoys = %+v, want only cv-open", en.convoys)
	}
	if len(en.mergeQueue) != 1 || en.mergeQueue[0].SourceIssue != "gtt-9" {
		t.Fatalf("mergeQueue = %+v", en.mergeQueue)
	}
	if len(gotRigs) != 1 || gotRigs[0] != "GasTownTower" {
		t.Fatalf("mq loader rigs = %v, want [GasTownTower]", gotRigs)
	}

	// Within TTL: no re-fetch.
	clock = clock.Add(14 * time.Second)
	c.fetchEnrichment(clock)
	if convoyCalls != 1 || mqCalls != 1 {
		t.Fatalf("re-fetched within TTL: convoy=%d mq=%d", convoyCalls, mqCalls)
	}
	// Past TTL: exactly one re-fetch.
	clock = clock.Add(2 * time.Second)
	c.fetchEnrichment(clock)
	if convoyCalls != 2 || mqCalls != 2 {
		t.Fatalf("expected one re-fetch after TTL: convoy=%d mq=%d", convoyCalls, mqCalls)
	}
}

// A failing convoy/MQ loader must not abort enrichment — other data still
// populates and the cache timestamp still advances.
func TestFetchEnrichmentConvoyBestEffort(t *testing.T) {
	clock := time.Unix(1_700_000_000, 0)
	c := &Collector{
		EnrichTTL:      15 * time.Second,
		now:            func() time.Time { return clock },
		loadReputation: func(string) (Reputation, error) { return Reputation{}, nil },
		loadMail:       func(string) (Mail, error) { return Mail{Total: 2}, nil },
		loadHooks:      func(string) (map[string]string, error) { return nil, nil },
		loadConvoys: func(string) ([]Convoy, error) {
			return nil, errors.New("gt convoy down")
		},
		loadMergeQueue: func(string, []string) ([]MergeRequest, error) {
			return nil, errors.New("gt mq down")
		},
	}
	en := c.fetchEnrichment(clock)
	if en.town.Mail.Total != 2 {
		t.Errorf("mail should populate despite convoy/mq failure, got %+v", en.town.Mail)
	}
	if len(en.convoys) != 0 || len(en.mergeQueue) != 0 {
		t.Errorf("failed loaders should leave empty data, got %+v / %+v", en.convoys, en.mergeQueue)
	}
	if c.enrichAt.IsZero() {
		t.Error("cache timestamp should advance even when convoy/mq loaders fail")
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
