package tower

import (
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
