package tower

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config is the committed tower.toml: optional overrides for the cockpit's
// role-based defaults. Every field is optional — the zero Config is the
// role-based behavior, so a town needs no tower.toml to start.
type Config struct {
	// OverseerAgentsExtra adds agent addresses to the awaiting-overseer eligible
	// set on top of the role default (mayor + crew), e.g. a special polecat that
	// genuinely answers to an overseer.
	OverseerAgentsExtra []string `toml:"overseer_agents_extra"`
	// OverseerAgentsExclude removes addresses from the eligible set, even ones
	// the role default would include (e.g. a background crew member that should
	// never read as awaiting).
	OverseerAgentsExclude []string `toml:"overseer_agents_exclude"`
}

// LoadConfig reads <townRoot>/tower.toml. A missing file is not an error: it
// returns the zero Config so the role-based default works with no config. A
// present-but-malformed file is an error so a typo surfaces rather than
// silently degrading to defaults.
func LoadConfig(townRoot string) (Config, error) {
	path := filepath.Join(townRoot, "tower.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, nil
		}
		return Config{}, err
	}
	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// overseerPolicy resolves whether an agent can read as awaiting-overseer,
// layering committed config and an ad-hoc CLI override over the role default.
// The zero value (all nil) is pure role-default, so a Collector built without
// config behaves exactly as the role gate alone.
type overseerPolicy struct {
	extra    map[string]bool // addresses added on top of the role default
	exclude  map[string]bool // addresses removed, even role-default-eligible ones
	override map[string]bool // when non-nil, the EXCLUSIVE eligible set (CLI flag)
}

// newOverseerPolicy builds a policy from committed config. The override stays
// nil until applied via withOverride.
func newOverseerPolicy(cfg Config) overseerPolicy {
	return overseerPolicy{
		extra:   addrSet(cfg.OverseerAgentsExtra),
		exclude: addrSet(cfg.OverseerAgentsExclude),
	}
}

// withOverride returns a copy whose eligible set is exactly addrs, ignoring the
// role default and committed extra/exclude (the ad-hoc --overseer-agents flag).
// An empty addrs is a no-op so an unset flag leaves the configured policy intact.
func (p overseerPolicy) withOverride(addrs []string) overseerPolicy {
	if set := addrSet(addrs); set != nil {
		p.override = set
	}
	return p
}

// eligible reports whether ref can read as awaiting-overseer. Precedence:
// CLI override > exclude > extra > role default.
func (p overseerPolicy) eligible(ref AgentRef) bool {
	if p.override != nil {
		return p.override[ref.Addr]
	}
	if p.exclude[ref.Addr] {
		return false
	}
	if p.extra[ref.Addr] {
		return true
	}
	return overseerEligible(ref.Role)
}

// addrSet builds a lookup set from a list of agent addresses, dropping blanks
// and surrounding whitespace. An empty or all-blank input yields nil so callers
// can treat "no entries" and "no config" identically.
func addrSet(addrs []string) map[string]bool {
	var set map[string]bool
	for _, a := range addrs {
		if a = strings.TrimSpace(a); a != "" {
			if set == nil {
				set = make(map[string]bool, len(addrs))
			}
			set[a] = true
		}
	}
	return set
}
