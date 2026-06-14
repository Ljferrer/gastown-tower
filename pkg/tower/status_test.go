package tower

import (
	"encoding/json"
	"testing"
)

// AgentStatus renders to a stable string token and round-trips through JSON, so
// web clients get "idle"/"churning"/"awaiting" and a Go decode recovers the enum.
func TestAgentStatusJSON(t *testing.T) {
	cases := []struct {
		status AgentStatus
		token  string
	}{
		{StatusIdle, "idle"},
		{StatusChurning, "churning"},
		{StatusAwaiting, "awaiting"},
	}
	for _, tc := range cases {
		if got := tc.status.String(); got != tc.token {
			t.Errorf("String() = %q, want %q", got, tc.token)
		}
		b, err := json.Marshal(tc.status)
		if err != nil {
			t.Fatalf("Marshal(%v): %v", tc.status, err)
		}
		if string(b) != `"`+tc.token+`"` {
			t.Errorf("Marshal(%v) = %s, want %q", tc.status, b, tc.token)
		}
		var back AgentStatus
		if err := json.Unmarshal(b, &back); err != nil {
			t.Fatalf("Unmarshal(%s): %v", b, err)
		}
		if back != tc.status {
			t.Errorf("round-trip = %v, want %v", back, tc.status)
		}
	}
	// Unknown tokens decode to idle rather than erroring.
	var s AgentStatus = StatusChurning
	if err := json.Unmarshal([]byte(`"bogus"`), &s); err != nil || s != StatusIdle {
		t.Errorf("unknown token: got %v, err %v; want StatusIdle, nil", s, err)
	}
}

// overseerEligible gates awaiting detection to roles that answer to an overseer:
// the mayor and crew. Autonomous workers and infrastructure are excluded.
func TestOverseerEligible(t *testing.T) {
	eligible := []string{"mayor", "crew"}
	for _, r := range eligible {
		if !overseerEligible(r) {
			t.Errorf("role %q should be overseer-eligible", r)
		}
	}
	for _, r := range []string{"polecat", "witness", "refinery", "dog", "rig", ""} {
		if overseerEligible(r) {
			t.Errorf("role %q should NOT be overseer-eligible", r)
		}
	}
}
