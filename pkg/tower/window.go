package tower

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Base context window for standard Claude models (tokens).
const baseWindow = 200_000

// extendedWindow is the 1M-token window used by extended-context ("[1m]") variants.
const extendedWindow = 1_000_000

// ContextWindow returns the effective context-window size (tokens) for an agent.
//
// The transcript's message.model (e.g. "claude-opus-4-8") does not carry the
// "[1m]" extended-context suffix that the configured session model does, so the
// model string alone under-reports the window for [1m] agents that have not yet
// filled past the base window. Detection order (configured model wins over the
// token heuristic, so the window reflects the configured model rather than the
// current fill):
//
//  1. The model id itself carries the explicit "[1m]" suffix.
//  2. The session is configured (settings.json) for an extended-context model
//     whose family matches this agent's model — e.g. "opus[1m]" alongside a
//     "claude-opus-4-*" message model.
//  3. Last-resort fallback: the observed prompt already exceeds the base window,
//     so the agent must be on an extended variant (keeps fill % ≤ 100%).
func ContextWindow(model string, observedTokens int) int {
	if isExtendedContext(model) {
		return extendedWindow
	}
	if observedTokens > baseWindow {
		return extendedWindow
	}
	return baseWindow
}

// isExtendedContext reports whether an agent running the given message model is
// on the 1M extended-context window, using the explicit suffix first and the
// configured session model (settings.json) as the authoritative fallback.
func isExtendedContext(model string) bool {
	if strings.Contains(model, "[1m]") {
		return true
	}
	cfg := lookupConfiguredModel()
	if !strings.Contains(cfg, "[1m]") {
		return false
	}
	fam := modelFamily(cfg)
	return fam != "" && fam == modelFamily(model)
}

// modelFamily extracts the Claude model family ("opus"/"sonnet"/"haiku") from a
// model id or alias, or "" when none is recognized. It lets us match a
// configured alias like "opus[1m]" against a resolved id like "claude-opus-4-8".
func modelFamily(model string) string {
	switch {
	case strings.Contains(model, "opus"):
		return "opus"
	case strings.Contains(model, "sonnet"):
		return "sonnet"
	case strings.Contains(model, "haiku"):
		return "haiku"
	default:
		return ""
	}
}

// lookupConfiguredModel resolves the session's configured Claude model id
// (e.g. "opus[1m]"). Indirected through a var so tests can supply a
// deterministic value; defaults to reading the user's global settings.json.
var lookupConfiguredModel = readGlobalConfiguredModel

// readGlobalConfiguredModel reads the "model" field from the user's global
// ~/.claude/settings.json. Best-effort: any error yields "" so detection falls
// back to the token heuristic.
func readGlobalConfiguredModel() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if err != nil {
		return ""
	}
	var cfg struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(data, &cfg) != nil {
		return ""
	}
	return cfg.Model
}

// ContextPct returns prompt fill as a fraction [0,1] of the effective window.
func ContextPct(model string, observedTokens int) float64 {
	w := ContextWindow(model, observedTokens)
	if w <= 0 {
		return 0
	}
	pct := float64(observedTokens) / float64(w)
	if pct > 1 {
		pct = 1
	}
	return pct
}
