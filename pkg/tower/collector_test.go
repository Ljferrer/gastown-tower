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
