package tower

import (
	"regexp"
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

// A numbered selection box reads as awaiting; plain prose or a working pane does
// not. The working marker always wins so an active turn never reads as awaiting.
func TestPaneAwaiting(t *testing.T) {
	selection := `Do you want to proceed?
❯ 1. Yes
  2. No, keep planning
  ⏵⏵ bypass permissions on (shift+tab to cycle)`

	plain := `❯ HEALTH_CHECK: heartbeat stale, respond to confirm responsiveness
  ⎿  ✓ Heartbeat updated: responsive; idle
❯`

	working := `· Flambéing… (44s · ↓ 2.0k tokens · thinking)
❯ 1. Yes
  esc to interrupt`

	if !paneAwaiting(selection) {
		t.Error("numbered selection box should read as awaiting")
	}
	if paneAwaiting(plain) {
		t.Error("plain prompt without a numbered box must not read as awaiting")
	}
	if paneAwaiting(working) {
		t.Error("a working pane must never read as awaiting")
	}
	if paneAwaiting("") {
		t.Error("empty pane must not read as awaiting")
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
		{AgentRef{Name: "furiosa", Role: "polecat", Rig: "GasTownTower", Group: "GasTownTower"}, "gtt-furiosa", true},
		{AgentRef{Name: "witness", Role: "witness", Rig: "GigaClip", Group: "GigaClip"}, "gc-witness", true},
		// gt qualifies crew sessions with their role: <prefix>-crew-<name>.
		{AgentRef{Name: "Quasimodo", Role: "crew", Rig: "GasTownTower", Group: "GasTownTower"}, "gtt-crew-Quasimodo", true},
		{AgentRef{Name: "ghost", Role: "polecat", Rig: "Unknown", Group: "Unknown"}, "", false},
	}
	for _, tt := range tests {
		got, ok := tmuxSession(tt.ref, prefixes)
		if ok != tt.ok || got != tt.want {
			t.Errorf("tmuxSession(%+v) = %q,%v want %q,%v", tt.ref, got, ok, tt.want, tt.ok)
		}
	}
}

// townSocketName mirrors gt's per-town socket derivation: a sanitized basename,
// a hyphen, and 6 hex chars of the canonical-path hash. It is deterministic for
// a given path and path-sensitive (two towns sharing a basename differ).
func TestTownSocketName(t *testing.T) {
	got := townSocketName("/Users/ljf/gt")
	if !regexp.MustCompile(`^[a-z0-9-]+-[0-9a-f]{6}$`).MatchString(got) {
		t.Fatalf("socket name %q does not match <base>-<hash6>", got)
	}
	if got != townSocketName("/Users/ljf/gt") {
		t.Error("socket name must be deterministic for the same path")
	}
	if townSocketName("/Users/ljf/gt") == townSocketName("/Users/ljf/work/gt") {
		t.Error("towns sharing a basename must get distinct sockets via the path hash")
	}
}

// gtSocketName honors GT_TMUX_SOCKET: a concrete value is used verbatim, while
// unset/"default"/"auto" fall through to the per-town derivation.
func TestGtSocketName(t *testing.T) {
	const town = "/Users/ljf/gt"
	derived := townSocketName(town)

	t.Setenv("GT_TMUX_SOCKET", "my-socket")
	if got := gtSocketName(town); got != "my-socket" {
		t.Errorf("explicit GT_TMUX_SOCKET = %q, want my-socket", got)
	}
	for _, v := range []string{"", "default", "auto"} {
		t.Setenv("GT_TMUX_SOCKET", v)
		if got := gtSocketName(town); got != derived {
			t.Errorf("GT_TMUX_SOCKET=%q = %q, want derived %q", v, got, derived)
		}
	}
}

// tmuxArgs prepends "-L <socket>" only when a socket is set, so an empty socket
// degrades to the default-socket behavior rather than passing a bogus flag.
func TestTmuxArgs(t *testing.T) {
	got := tmuxArgs("gt-a62b6c", "ls", "-F", "x")
	want := []string{"-L", "gt-a62b6c", "ls", "-F", "x"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("arg[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	bare := tmuxArgs("", "ls")
	if len(bare) != 1 || bare[0] != "ls" {
		t.Errorf("empty socket should not add -L, got %v", bare)
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
