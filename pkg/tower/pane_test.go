package tower

import (
	"testing"
	"time"
)

// A pane with the working marker reads as churning; one without it reads idle.
func TestPaneWorking(t *testing.T) {
	working := `· Flambéing… (44s · ↓ 2.0k tokens · thinking)
  ⎿  Tip: Paste images into Claude Code using control+v (not cmd+v!)
❯
  ⏵⏵ bypass permissions on (shift+tab to cycle) · esc to interrupt`

	idle := `❯ HEALTH_CHECK: heartbeat stale, respond to confirm responsiveness
  ⎿  ✓ Heartbeat updated: responsive; idle
❯
  ⏵⏵ bypass permissions on (shift+tab to cycle) · ← for agents`

	if !paneWorking(working) {
		t.Error("working pane should be detected as churning")
	}
	if paneWorking(idle) {
		t.Error("idle pane must not be detected as churning")
	}
	if paneWorking("") {
		t.Error("empty pane must not be detected as churning")
	}
}

// The spinner line yields current-turn elapsed seconds and streamed token count,
// handling both bare integers (475) and k-suffixed values (2.0k).
func TestParseSpinner(t *testing.T) {
	tests := []struct {
		name        string
		pane        string
		wantElapsed time.Duration
		wantTokens  int
		wantOK      bool
	}{
		{
			name:        "bare token count",
			pane:        "✳ Honking… (9s · ↓ 475 tokens)",
			wantElapsed: 9 * time.Second,
			wantTokens:  475,
			wantOK:      true,
		},
		{
			name:        "k-suffixed tokens with trailing detail",
			pane:        "· Flambéing… (44s · ↓ 2.0k tokens · thinking)",
			wantElapsed: 44 * time.Second,
			wantTokens:  2000,
			wantOK:      true,
		},
		{
			name:   "no spinner line",
			pane:   "❯ idle\n  ⏵⏵ bypass permissions on · ← for agents",
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseSpinner(tt.pane)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if got.Elapsed != tt.wantElapsed {
				t.Errorf("elapsed = %v, want %v", got.Elapsed, tt.wantElapsed)
			}
			if got.Tokens != tt.wantTokens {
				t.Errorf("tokens = %d, want %d", got.Tokens, tt.wantTokens)
			}
		})
	}
}

// Session names follow "<prefix>-<leaf>": town agents use "hq", rig agents use
// the rig's route prefix, and a deacon dog uses its leaf name (deacon/boot -> boot).
func TestTmuxSession(t *testing.T) {
	prefixes := map[string]string{".": "hq", "GasTownTower": "gtt", "GigaClip": "gc"}
	tests := []struct {
		ref  AgentRef
		want string
		ok   bool
	}{
		{AgentRef{Name: "mayor", Group: townGroup}, "hq-mayor", true},
		{AgentRef{Name: "deacon/boot", Group: townGroup}, "hq-boot", true},
		{AgentRef{Name: "furiosa", Rig: "GasTownTower", Group: "GasTownTower"}, "gtt-furiosa", true},
		{AgentRef{Name: "witness", Rig: "GigaClip", Group: "GigaClip"}, "gc-witness", true},
		{AgentRef{Name: "ghost", Rig: "Unknown", Group: "Unknown"}, "", false},
	}
	for _, tt := range tests {
		got, ok := tmuxSession(tt.ref, prefixes)
		if ok != tt.ok || got != tt.want {
			t.Errorf("tmuxSession(%+v) = %q,%v want %q,%v", tt.ref, got, ok, tt.want, tt.ok)
		}
	}
}

// routes.jsonl maps each rig path to its session prefix (trailing "-" stripped).
// When a path has several routes (town has hq- and hq-cv-), the first wins.
func TestParseSessionPrefixes(t *testing.T) {
	routes := `{"prefix":"hq-","path":"."}
{"prefix":"hq-cv-","path":"."}
{"prefix":"gc-","path":"GigaClip"}
{"prefix":"gtt-","path":"GasTownTower"}`

	got := parseSessionPrefixes([]byte(routes))
	want := map[string]string{".": "hq", "GigaClip": "gc", "GasTownTower": "gtt"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("prefix[%q] = %q, want %q", k, got[k], v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("got %d prefixes, want %d: %+v", len(got), len(want), got)
	}
}
