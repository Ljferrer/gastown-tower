package tower

import (
	"math"
	"testing"
)

func TestContextWindow(t *testing.T) {
	cases := []struct {
		name       string
		configured string // session model from settings.json
		model      string
		observed   int
		want       int
	}{
		// No extended-context configured: only the token heuristic / explicit
		// suffix can lift the window.
		{"standard model under base", "", "claude-sonnet-4-6", 50_000, baseWindow},
		{"observed exceeds base implies extended", "", "claude-opus-4-8", 419_752, extendedWindow},
		{"explicit 1m suffix", "", "claude-opus-4-8[1m]", 10_000, extendedWindow},
		{"exact base stays base", "", "claude-haiku-4-5", baseWindow, baseWindow},

		// Configured model is the authoritative signal: an agent on opus[1m]
		// reads 1M regardless of current fill (the core bug — gtt-3w3).
		{"configured opus[1m] lifts low-fill opus", "opus[1m]", "claude-opus-4-8", 12_000, extendedWindow},
		{"configured full id with suffix", "claude-opus-4-8[1m]", "claude-opus-4-8", 5_000, extendedWindow},
		// Configured extended context only applies to the matching family.
		{"configured opus[1m] does not lift sonnet", "opus[1m]", "claude-sonnet-4-6", 50_000, baseWindow},
		// Configured model without the suffix stays on the base window.
		{"configured opus without suffix stays base", "opus", "claude-opus-4-8", 50_000, baseWindow},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			setConfiguredModelForTest(t, c.configured)
			if got := ContextWindow(c.model, c.observed); got != c.want {
				t.Fatalf("ContextWindow(%q,%d) [cfg=%q]=%d, want %d", c.model, c.observed, c.configured, got, c.want)
			}
		})
	}
}

func TestContextPct(t *testing.T) {
	setConfiguredModelForTest(t, "") // no extended context configured
	if got := ContextPct("claude-sonnet-4-6", 100_000); math.Abs(got-0.5) > 1e-9 {
		t.Fatalf("pct=%v, want 0.5", got)
	}
	// extended-context agent: 420k of 1M = 0.42 (not >1)
	if got := ContextPct("claude-opus-4-8", 420_000); math.Abs(got-0.42) > 1e-9 {
		t.Fatalf("pct=%v, want 0.42", got)
	}
	// never exceeds 1
	if got := ContextPct("claude-sonnet-4-6", 999_999_999); got != 1 {
		t.Fatalf("pct=%v, want clamp to 1", got)
	}
}

// setConfiguredModelForTest pins the session's configured model id (normally
// read from settings.json) so window tests are deterministic across machines.
func setConfiguredModelForTest(t *testing.T, model string) {
	t.Helper()
	prev := lookupConfiguredModel
	lookupConfiguredModel = func() string { return model }
	t.Cleanup(func() { lookupConfiguredModel = prev })
}
