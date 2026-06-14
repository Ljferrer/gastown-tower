package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/Ljferrer/gastown-tower/pkg/tower"
	tea "github.com/charmbracelet/bubbletea"
)

func fixtureSnapshot() tower.Snapshot {
	return tower.Snapshot{
		GeneratedAt: time.Date(2026, 5, 30, 15, 4, 5, 0, time.UTC),
		Stats: tower.TownStats{
			Active:   2,
			Churning: 1,
			Rigs: []tower.RigStat{
				{Name: "town", Active: 1, Churning: 1},
				{Name: "GigaClip", Active: 1, Churning: 0},
			},
		},
		Groups: []tower.Group{
			{Name: "town", Agents: []tower.Agent{{
				AgentRef: tower.AgentRef{Name: "mayor", Role: "mayor", Group: "town"},
				Churning: true,
				Stats:    tower.TranscriptStats{Model: "claude-opus-4-8", ContextPct: 0.45, Turns: 10, ToolCalls: 5, FileReads: 1, NowDoing: "running go test"},
			}}},
			{Name: "GigaClip", Agents: []tower.Agent{{
				AgentRef: tower.AgentRef{Name: "witness", Role: "witness", Rig: "GigaClip", Group: "GigaClip"},
				Churning: false,
				Idle:     5 * time.Minute,
				Stats:    tower.TranscriptStats{Model: "claude-haiku-4-5", ContextPct: 0.2},
			}}},
		},
	}
}

func newWithSnap() Model {
	m := New(nil)
	m.snap = fixtureSnapshot()
	m.flatten()
	return m
}

func TestView(t *testing.T) {
	out := newWithSnap().View()
	for _, want := range []string{
		"GAS TOWN ACTIVITY TOWER", "2 active",
		"town", "mayor", "running go test",
		"GigaClip", "witness", "idle 5m",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("View() missing %q", want)
		}
	}
}

func TestNavigateAndExpand(t *testing.T) {
	m := newWithSnap()
	if len(m.agents) != 2 {
		t.Fatalf("flatten: got %d agents", len(m.agents))
	}
	// move cursor down to the witness
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	if m.cursor != 1 {
		t.Fatalf("cursor=%d, want 1", m.cursor)
	}
	// expand it
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if !m.expanded["GigaClip/witness"] {
		t.Fatalf("witness should be expanded")
	}
	if !strings.Contains(m.View(), "role witness") {
		t.Errorf("expanded details not rendered")
	}
}

func TestQuit(t *testing.T) {
	m := newWithSnap()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("ctrl+c should return a quit command")
	}
}

func TestTownStatsHeaderAndRollup(t *testing.T) {
	out := newWithSnap().View()
	// Header carries the town-wide busy rollup alongside the active count.
	for _, want := range []string{"2 active", "1 busy"} {
		if !strings.Contains(out, want) {
			t.Errorf("View() missing header stat %q:\n%s", want, out)
		}
	}
	// Per-rig breakdown line: each group with its active count, churn tally on
	// the working group.
	for _, want := range []string{"town 1", "GigaClip 1", "1●"} {
		if !strings.Contains(out, want) {
			t.Errorf("View() missing per-rig rollup %q:\n%s", want, out)
		}
	}
}

// The per-rig line is town-wide; a search filter narrows the AGENTS list but
// must not change the header rollup.
func TestTownStatsIgnoreSearchFilter(t *testing.T) {
	m := newWithSnap()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = next.(Model)
	for _, r := range "wit" {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(Model)
	}
	out := m.View()
	if !strings.Contains(out, "2 active") {
		t.Errorf("filtered view changed town-wide active count:\n%s", out)
	}
}

// A single-group town has nothing to break down, so the rollup line is omitted.
func TestRenderRigsSingleGroup(t *testing.T) {
	if got := renderRigs(tower.TownStats{Active: 1, Rigs: []tower.RigStat{{Name: "town", Active: 1}}}); got != "" {
		t.Errorf("single-group rollup should be empty, got %q", got)
	}
}

func TestRenderTown(t *testing.T) {
	if got := renderTown(tower.TownStatus{}); got != "" {
		t.Errorf("empty town status should render nothing, got %q", got)
	}
	full := tower.TownStatus{
		Reputation: tower.Reputation{Tier: "Bronze", Stamps: 7},
		Mail:       tower.Mail{Total: 5, Unread: 2},
	}
	out := renderTown(full)
	for _, want := range []string{"✉ 2/5", "Bronze", "7 stamps"} {
		if !strings.Contains(out, want) {
			t.Errorf("renderTown() missing %q in %q", want, out)
		}
	}
}

func TestTabSwitchesFocus(t *testing.T) {
	m := newWithSnap()
	if m.focus != panelAgents {
		t.Fatalf("default focus = %v, want panelAgents", m.focus)
	}
	for _, want := range []panel{panelConvoys, panelEvents, panelAgents} {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		m = next.(Model)
		if m.focus != want {
			t.Fatalf("after tab, focus = %v, want %v", m.focus, want)
		}
	}
	// shift+tab walks the other way.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = next.(Model)
	if m.focus != panelEvents {
		t.Fatalf("after shift+tab, focus = %v, want panelEvents", m.focus)
	}
}

func TestViewRendersAllPanels(t *testing.T) {
	m := newWithSnap()
	out := m.View()
	for _, want := range []string{"AGENTS", "CONVOYS", "EVENTS"} {
		if !strings.Contains(out, want) {
			t.Errorf("View() missing panel header %q", want)
		}
	}
	// Agents panel is focused by default; its header carries the focus marker.
	if !strings.Contains(out, "▌ AGENTS") {
		t.Errorf("focused AGENTS header missing focus marker:\n%s", out)
	}
	// Focus the convoys panel and confirm the marker moves.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	out = m.View()
	if !strings.Contains(out, "▌ CONVOYS") {
		t.Errorf("focused CONVOYS header missing focus marker:\n%s", out)
	}
	if strings.Contains(out, "▌ AGENTS") {
		t.Errorf("AGENTS header should no longer be focused:\n%s", out)
	}
}

func TestScrollRoutedByFocus(t *testing.T) {
	m := newWithSnap()
	// While AGENTS is focused, j moves the agent cursor.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = next.(Model)
	if m.cursor != 1 {
		t.Fatalf("j with AGENTS focus: cursor = %d, want 1", m.cursor)
	}
	// Focus a non-agents panel; j must NOT move the agent cursor.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = next.(Model)
	if m.cursor != 1 {
		t.Fatalf("k with CONVOYS focus moved agent cursor to %d, want 1", m.cursor)
	}
}

func TestEnterExpandsOnlyWhenAgentsFocused(t *testing.T) {
	m := newWithSnap()
	// Focus events, then enter — must not expand any agent.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	if m.focus != panelEvents {
		t.Fatalf("setup: focus = %v, want panelEvents", m.focus)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if len(m.expanded) != 0 {
		t.Fatalf("enter with EVENTS focus expanded an agent: %v", m.expanded)
	}
}

func TestSearchFiltersAgents(t *testing.T) {
	m := newWithSnap()
	if len(m.agents) != 2 {
		t.Fatalf("setup: %d agents", len(m.agents))
	}
	// Enter search and type "wit" — only the witness should remain.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = next.(Model)
	if !m.searching {
		t.Fatal("'/' did not enter search mode")
	}
	for _, r := range "wit" {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(Model)
	}
	if len(m.agents) != 1 || m.agents[0].Name != "witness" {
		t.Fatalf("search 'wit' filtered to %d agents %v, want [witness]", len(m.agents), m.agents)
	}
	out := m.View()
	if !strings.Contains(out, "wit") {
		t.Errorf("search query not shown in view:\n%s", out)
	}
	// esc clears the filter and restores all agents.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.searching {
		t.Fatal("esc did not exit search mode")
	}
	if len(m.agents) != 2 {
		t.Fatalf("esc did not clear filter: %d agents", len(m.agents))
	}
}

func TestViewShowsTownAndHook(t *testing.T) {
	m := New(nil)
	snap := fixtureSnapshot()
	snap.Town = tower.TownStatus{
		Reputation: tower.Reputation{Tier: "Bronze", Stamps: 7},
		Mail:       tower.Mail{Total: 5, Unread: 2},
	}
	snap.Groups[0].Agents[0].Hook = "gtt-vsq: Slice 3"
	m.snap = snap
	m.flatten()

	out := m.View()
	if !strings.Contains(out, "✉ 2/5") || !strings.Contains(out, "Bronze") {
		t.Errorf("View() missing town status line: %q", out)
	}
	if !strings.Contains(out, "🪝") {
		t.Errorf("View() missing hook glyph for hooked agent")
	}

	// Expand the hooked agent and confirm the full hook line shows.
	m.expanded["town/mayor"] = true
	if !strings.Contains(m.View(), "hook gtt-vsq: Slice 3") {
		t.Errorf("expanded view missing hook detail")
	}
}
