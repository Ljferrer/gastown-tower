package tower

import "encoding/json"

// AgentStatus is the three-state activity of an agent session:
//   - StatusIdle: nothing in flight (waiting, between turns, or dead).
//   - StatusChurning: actively generating a turn.
//   - StatusAwaiting: paused on a question for its overseer (a structured
//     prompt, a pending AskUserQuestion/ExitPlanMode, or a trailing question).
//
// It supersedes the older Agent.Churning bool, which is retained as a
// web-compat shim (= Status == StatusChurning) so existing JSON clients keep
// working while the TUI switches on the richer enum.
type AgentStatus int

const (
	StatusIdle AgentStatus = iota
	StatusChurning
	StatusAwaiting
)

// String returns the stable lowercase token for a status, used both for the
// JSON wire form and for readable test output.
func (s AgentStatus) String() string {
	switch s {
	case StatusChurning:
		return "churning"
	case StatusAwaiting:
		return "awaiting"
	default:
		return "idle"
	}
}

// MarshalJSON serializes the status as its string token (e.g. "awaiting") so
// web clients receive a self-describing value rather than an opaque integer.
func (s AgentStatus) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

// UnmarshalJSON parses the string token form back into an AgentStatus, keeping
// the JSON representation symmetric so a snapshot survives a Go encode/decode
// round-trip. Unknown tokens decode to StatusIdle.
func (s *AgentStatus) UnmarshalJSON(b []byte) error {
	var tok string
	if err := json.Unmarshal(b, &tok); err != nil {
		return err
	}
	switch tok {
	case "churning":
		*s = StatusChurning
	case "awaiting":
		*s = StatusAwaiting
	default:
		*s = StatusIdle
	}
	return nil
}

// overseerEligible reports whether an agent's role is one that can sensibly be
// "awaiting overseer" input. By default this is the mayor and any crew agent;
// autonomous workers (polecats), infrastructure (witness/refinery/dog), and
// bare rigs are excluded so their idle/blocking states never read as awaiting.
// This is the role default; per-agent overrides (tower.toml's
// overseer_agents_extra/exclude and the --overseer-agents flag) layer on top
// via overseerPolicy in config.go.
func overseerEligible(role string) bool {
	switch role {
	case "mayor", "crew":
		return true
	default:
		return false
	}
}
