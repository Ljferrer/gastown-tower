package tower

import (
	"math"
	"testing"
)

func TestContextWindow(t *testing.T) {
	cases := []struct {
		name     string
		model    string
		observed int
		want     int
	}{
		{"standard model under base", "claude-sonnet-4-6", 50_000, baseWindow},
		{"observed exceeds base implies extended", "claude-opus-4-8", 419_752, extendedWindow},
		{"explicit 1m suffix", "claude-opus-4-8[1m]", 10_000, extendedWindow},
		{"exact base stays base", "claude-haiku-4-5", baseWindow, baseWindow},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ContextWindow(c.model, c.observed); got != c.want {
				t.Fatalf("ContextWindow(%q,%d)=%d, want %d", c.model, c.observed, got, c.want)
			}
		})
	}
}

func TestContextPct(t *testing.T) {
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
