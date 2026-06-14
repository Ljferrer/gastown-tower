package tower

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A town with no tower.toml yields the zero Config and no error: the role-based
// default must work with no config present.
func TestLoadConfigMissing(t *testing.T) {
	cfg, err := LoadConfig(t.TempDir())
	if err != nil {
		t.Fatalf("missing tower.toml should not error, got %v", err)
	}
	if len(cfg.OverseerAgentsExtra) != 0 || len(cfg.OverseerAgentsExclude) != 0 {
		t.Errorf("missing config should be zero, got %+v", cfg)
	}
}

// A committed tower.toml's overseer keys parse into the Config.
func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	body := `
# tower.toml
overseer_agents_extra   = ["GasTownTower/polecats/special"]
overseer_agents_exclude = ["GasTownTower/crew/Background", "GasTownTower/crew/Other"]
`
	if err := os.WriteFile(filepath.Join(dir, "tower.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if got := cfg.OverseerAgentsExtra; len(got) != 1 || got[0] != "GasTownTower/polecats/special" {
		t.Errorf("extra = %v", got)
	}
	if got := cfg.OverseerAgentsExclude; len(got) != 2 {
		t.Errorf("exclude = %v", got)
	}
}

// Malformed TOML surfaces as an error rather than silently parsing as empty.
func TestLoadConfigMalformed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tower.toml"), []byte("overseer_agents_extra = [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(dir); err == nil {
		t.Error("malformed tower.toml should error")
	}
}

// The zero policy (no config) is pure role-default: mayor + crew eligible, other
// roles not. This keeps a Collector built without config behaving as before.
func TestOverseerPolicyDefault(t *testing.T) {
	var p overseerPolicy
	if !p.eligible(AgentRef{Role: "mayor", Addr: "GasTownTower/mayor"}) {
		t.Error("mayor should be eligible by default")
	}
	if !p.eligible(AgentRef{Role: "crew", Addr: "GasTownTower/crew/Quasimodo"}) {
		t.Error("crew should be eligible by default")
	}
	if p.eligible(AgentRef{Role: "polecat", Addr: "GasTownTower/polecats/furiosa"}) {
		t.Error("polecat should not be eligible by default")
	}
}

// overseer_agents_extra promotes a non-default role; exclude demotes even a
// role-default-eligible agent. Exclude wins over extra for the same address.
func TestOverseerPolicyExtraExclude(t *testing.T) {
	cfg := Config{
		OverseerAgentsExtra:   []string{"GasTownTower/polecats/special", "GasTownTower/crew/Both"},
		OverseerAgentsExclude: []string{"GasTownTower/crew/Background", "GasTownTower/crew/Both"},
	}
	p := newOverseerPolicy(cfg)

	if !p.eligible(AgentRef{Role: "polecat", Addr: "GasTownTower/polecats/special"}) {
		t.Error("extra address should be eligible despite non-default role")
	}
	if p.eligible(AgentRef{Role: "crew", Addr: "GasTownTower/crew/Background"}) {
		t.Error("excluded crew should not be eligible")
	}
	if p.eligible(AgentRef{Role: "crew", Addr: "GasTownTower/crew/Both"}) {
		t.Error("exclude must win over extra for the same address")
	}
	// An unmentioned crew member keeps the role default.
	if !p.eligible(AgentRef{Role: "crew", Addr: "GasTownTower/crew/Normal"}) {
		t.Error("unmentioned crew should stay eligible by role default")
	}
}

// withOverride makes exactly the listed addresses eligible, ignoring both the
// role default and committed extra/exclude. An empty override leaves the
// configured policy untouched.
func TestOverseerPolicyOverride(t *testing.T) {
	cfg := Config{OverseerAgentsExtra: []string{"GasTownTower/polecats/special"}}
	p := newOverseerPolicy(cfg).withOverride([]string{"GasTownTower/crew/Only"})

	if !p.eligible(AgentRef{Role: "polecat", Addr: "GasTownTower/crew/Only"}) {
		t.Error("override address should be eligible regardless of role")
	}
	// Role default is ignored under override.
	if p.eligible(AgentRef{Role: "mayor", Addr: "GasTownTower/mayor"}) {
		t.Error("mayor must not be eligible when an override excludes it")
	}
	// Configured extra is ignored under override.
	if p.eligible(AgentRef{Role: "polecat", Addr: "GasTownTower/polecats/special"}) {
		t.Error("configured extra must not survive an override")
	}

	// Empty override is a no-op: the configured policy still applies.
	noop := newOverseerPolicy(cfg).withOverride(nil)
	if !noop.eligible(AgentRef{Role: "mayor", Addr: "GasTownTower/mayor"}) {
		t.Error("empty override should leave the role default intact")
	}
}

// The Collector's status path honors the policy: an extra-eligible polecat reads
// awaiting on a fresh pending-ask, where the bare role default would not.
func TestAgentStatusHonorsOverseerPolicy(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	fresh := now.Add(-2 * time.Second)
	prefixes := map[string]string{"GigaClip": "gc"}
	noSessions := map[string]struct{}{}
	special := AgentRef{
		Name: "special", Role: "polecat", Rig: "GigaClip", Group: "GigaClip",
		Addr: "GasTownTower/polecats/special",
	}

	c := &Collector{
		ChurnWindow:  8 * time.Second,
		TurnWindow:   5 * time.Minute,
		ActiveWindow: 30 * time.Minute,
		overseer:     newOverseerPolicy(Config{OverseerAgentsExtra: []string{"GasTownTower/polecats/special"}}),
	}
	if status, _ := c.agentStatus(special, prefixes, noSessions, fresh, now, TranscriptStats{PendingAsk: true}); status != StatusAwaiting {
		t.Errorf("extra-eligible polecat should read awaiting, got %v", status)
	}

	// Without the policy, the same polecat is never awaiting.
	bare := &Collector{ChurnWindow: 8 * time.Second, TurnWindow: 5 * time.Minute, ActiveWindow: 30 * time.Minute}
	if status, _ := bare.agentStatus(special, prefixes, noSessions, fresh, now, TranscriptStats{PendingAsk: true}); status == StatusAwaiting {
		t.Errorf("bare role default must not make a polecat awaiting, got %v", status)
	}
}
