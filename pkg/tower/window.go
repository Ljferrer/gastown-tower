package tower

import "strings"

// Base context window for standard Claude models (tokens).
const baseWindow = 200_000

// extendedWindow is the 1M-token window used by extended-context ("[1m]") variants.
const extendedWindow = 1_000_000

// ContextWindow returns the effective context-window size for a model.
//
// The transcript's message.model (e.g. "claude-opus-4-8") does not carry the
// "[1m]" extended-context suffix that the session model id does, so we cannot
// detect extended context from the model string alone. Heuristic: if the
// observed prompt size already exceeds the base window, the agent must be on an
// extended-context variant, so report the 1M window. This keeps the fill % from
// exceeding 100% for [1m] agents. (Known limitation: an agent that fills exactly
// to the base window edge is treated as extended.)
func ContextWindow(model string, observedTokens int) int {
	if strings.Contains(model, "[1m]") || observedTokens > baseWindow {
		return extendedWindow
	}
	return baseWindow
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
