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
				Status:   tower.StatusChurning,
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

// An agent in the awaiting-overseer state renders the "awaiting overseer"
// activity label instead of its now-doing line or an idle marker.
func TestViewAwaitingOverseer(t *testing.T) {
	m := New(nil)
	m.snap = tower.Snapshot{
		GeneratedAt: time.Date(2026, 5, 30, 15, 4, 5, 0, time.UTC),
		Stats:       tower.TownStats{Active: 1},
		Groups: []tower.Group{{Name: "town", Agents: []tower.Agent{{
			AgentRef: tower.AgentRef{Name: "mayor", Role: "mayor", Group: "town"},
			Status:   tower.StatusAwaiting,
			Stats:    tower.TranscriptStats{Model: "claude-opus-4-8", ContextPct: 0.3, NowDoing: "asking a question"},
		}}}},
	}
	m.flatten()

	out := m.View()
	if !strings.Contains(out, "awaiting overseer") {
		t.Errorf("awaiting agent should show 'awaiting overseer':\n%s", out)
	}
	if strings.Contains(out, "idle ") {
		t.Errorf("awaiting agent must not render as idle:\n%s", out)
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

// eventsFixture returns a snapshot whose flow log mixes a curated event with a
// noisy session_start (newest-first, as the collector delivers it).
func eventsFixture() tower.Snapshot {
	base := time.Date(2026, 5, 30, 15, 4, 5, 0, time.UTC)
	return tower.Snapshot{
		GeneratedAt: base,
		Events: []tower.Event{
			{TS: base, Type: "sling", Actor: "GasTownTower/crew/Quasimodo",
				Payload: map[string]any{"bead": "gtt-975.4", "target": "GasTownTower/polecats/furiosa"}},
			{TS: base.Add(-1 * time.Minute), Type: "session_start", Actor: "GasTownTower/polecats/slit",
				Payload: map[string]any{"role": "GasTownTower/polecats/slit"}},
		},
	}
}

func TestEventsPanelCuratedHidesSessionStart(t *testing.T) {
	m := New(nil)
	m.snap = eventsFixture()
	m.flatten()

	out := m.View()
	if !strings.Contains(out, "sling") {
		t.Errorf("curated view should show sling event:\n%s", out)
	}
	if strings.Contains(out, "session_start") {
		t.Errorf("curated view must hide session_start:\n%s", out)
	}
	// The curated summary renders the bead → target with shortened address.
	if !strings.Contains(out, "gtt-975.4") || !strings.Contains(out, "polecats/furiosa") {
		t.Errorf("sling summary missing bead/target:\n%s", out)
	}
}

func TestEventsToggleShowsAll(t *testing.T) {
	m := New(nil)
	m.snap = eventsFixture()
	m.flatten()
	if m.showAllEvents {
		t.Fatal("default should be curated (showAllEvents=false)")
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	m = next.(Model)
	if !m.showAllEvents {
		t.Fatal("'t' did not toggle show-all")
	}
	out := m.View()
	if !strings.Contains(out, "session_start") {
		t.Errorf("show-all view should include session_start:\n%s", out)
	}
	if !strings.Contains(out, "all events") {
		t.Errorf("show-all header subtitle missing:\n%s", out)
	}

	// Toggling again returns to curated.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	m = next.(Model)
	if m.showAllEvents {
		t.Fatal("second 't' did not toggle back to curated")
	}
}

func TestEventsVisibleNewestFirst(t *testing.T) {
	m := New(nil)
	m.snap = eventsFixture()
	m.showAllEvents = true // include both events
	vis := m.visibleEvents()
	if len(vis) != 2 {
		t.Fatalf("want 2 visible events, got %d", len(vis))
	}
	if vis[0].Type != "sling" || vis[1].Type != "session_start" {
		t.Fatalf("not newest-first: %s, %s", vis[0].Type, vis[1].Type)
	}
}

func TestEventsSearchFilters(t *testing.T) {
	m := New(nil)
	m.snap = eventsFixture()
	m.showAllEvents = true
	m.query = "session" // matches the session_start actor/type
	vis := m.visibleEvents()
	if len(vis) != 1 || vis[0].Type != "session_start" {
		t.Fatalf("search 'session' filtered to %d events, want [session_start]", len(vis))
	}
}

func convoyFixture() tower.Snapshot {
	s := fixtureSnapshot()
	s.Convoys = []tower.Convoy{
		{ID: "hq-cv-1", Title: "Work: Phase 2: convoys panel", Status: "open", Completed: 0, Total: 1,
			Tracked: []tower.ConvoyItem{{ID: "gtt-975.5", Assignee: "GasTownTower/polecats/slit"}}},
		{ID: "hq-cv-2", Title: "Work: Layout", Status: "closed", Completed: 1, Total: 1,
			Tracked: []tower.ConvoyItem{{ID: "gtt-975.1", Assignee: "GasTownTower/polecats/furiosa"}}},
	}
	s.MergeQueue = []tower.MergeRequest{
		{ID: "mr-1", SourceIssue: "gtt-975.1", Worker: "furiosa", Rig: "GasTownTower", Status: "open"},
	}
	return s
}

func TestConvoysPanelRenders(t *testing.T) {
	m := New(nil)
	m.snap = convoyFixture()
	m.flatten()
	out := m.View()
	for _, want := range []string{
		"CONVOYS",
		"Phase 2: convoys panel", // "Work: " prefix stripped
		"0/1", "1/1",
		"slit", "furiosa", // assignee leaf names
		"merge queue · 1 pending",
		"gtt-975.1", // MR source issue
	} {
		if !strings.Contains(out, want) {
			t.Errorf("convoys panel missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "no active convoys") {
		t.Errorf("should not show empty state with convoys present:\n%s", out)
	}
}

func TestConvoysPanelEmpty(t *testing.T) {
	m := newWithSnap() // fixtureSnapshot has no convoys/MQ
	out := m.View()
	if !strings.Contains(out, "no active convoys") {
		t.Errorf("empty convoys panel should show empty state:\n%s", out)
	}
	if !strings.Contains(out, "merge queue · 0 pending") {
		t.Errorf("empty MQ should show 0 pending:\n%s", out)
	}
}

func TestConvoyCount(t *testing.T) {
	if n := convoyCount(convoyFixture()); n != 3 { // 2 convoys + 1 MR
		t.Errorf("convoyCount = %d, want 3", n)
	}
	if n := convoyCount(fixtureSnapshot()); n != 0 {
		t.Errorf("convoyCount (empty) = %d, want 0", n)
	}
}

// Scrolling the focused CONVOYS panel must stay within the row count and never
// move the agent cursor.
func TestConvoyScrollBounds(t *testing.T) {
	m := New(nil)
	m.snap = convoyFixture()
	m.flatten()
	// Focus CONVOYS.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	if m.focus != panelConvoys {
		t.Fatalf("focus = %v, want panelConvoys", m.focus)
	}
	// Scroll down well past the end; index must clamp to count-1 (3 rows → max 2).
	for i := 0; i < 10; i++ {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		m = next.(Model)
	}
	if m.convoyScroll != 2 {
		t.Errorf("convoyScroll = %d, want clamped to 2", m.convoyScroll)
	}
	if m.cursor != 0 {
		t.Errorf("CONVOYS scroll moved agent cursor to %d, want 0", m.cursor)
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

// lineOf returns the 0-based row index of the first View line containing sub,
// or -1. The mouse handler is fed absolute Y, so tests locate an agent's row by
// the unique text it renders — this keeps the tests independent of the panel's
// dynamic vertical offset and validates the real "click where you see it"
// contract rather than re-deriving agentAtY's arithmetic.
func lineOf(view, sub string) int {
	for i, ln := range strings.Split(view, "\n") {
		if strings.Contains(ln, sub) {
			return i
		}
	}
	return -1
}

func leftClick(y int) tea.MouseMsg {
	return tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, Y: y}
}

// A left-click on an agent's row toggles its expanded state and moves the
// AGENTS cursor (and focus) to it — the same effect as enter on that row.
// Clicking again collapses it.
func TestMouseClickTogglesAgent(t *testing.T) {
	m := newWithSnap()
	// Click the witness row (second agent), located by its unique idle label.
	y := lineOf(m.View(), "idle 5m")
	if y < 0 {
		t.Fatalf("could not locate witness row in view:\n%s", m.View())
	}
	next, _ := m.Update(leftClick(y))
	m = next.(Model)
	if !m.expanded["GigaClip/witness"] {
		t.Fatalf("click did not expand witness (cursor=%d):\n%s", m.cursor, m.View())
	}
	if m.cursor != 1 {
		t.Errorf("click did not move cursor to witness: cursor=%d, want 1", m.cursor)
	}
	if m.focus != panelAgents {
		t.Errorf("click did not focus AGENTS panel: focus=%v", m.focus)
	}
	if !strings.Contains(m.View(), "role witness") {
		t.Errorf("expanded detail not rendered after click")
	}
	// Clicking the same row again collapses it (row stays put; detail is below).
	y = lineOf(m.View(), "idle 5m")
	next, _ = m.Update(leftClick(y))
	m = next.(Model)
	if m.expanded["GigaClip/witness"] {
		t.Errorf("second click did not collapse witness")
	}
}

// Clicks that miss agent rows — group headers, the blank panel-header line, and
// rows below the AGENTS panel — must not toggle anything and must not panic.
func TestMouseClickNoOpOffAgents(t *testing.T) {
	for _, tc := range []struct {
		name string
		sub  string // text identifying the row to click; "" means a fixed Y
		y    int
	}{
		{name: "group header", sub: "▌ town"},
		{name: "agents panel header", sub: "▌ AGENTS"},
		{name: "blank/top line", y: 0},
		{name: "convoys panel", sub: "CONVOYS"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newWithSnap()
			y := tc.y
			if tc.sub != "" {
				if y = lineOf(m.View(), tc.sub); y < 0 {
					t.Fatalf("could not locate %q:\n%s", tc.sub, m.View())
				}
			}
			next, _ := m.Update(leftClick(y))
			m = next.(Model)
			if len(m.expanded) != 0 {
				t.Errorf("click on %s toggled an agent: %v", tc.name, m.expanded)
			}
		})
	}
}

// Non-left buttons and motion/release events must not toggle agents even when
// they land on an agent's row.
func TestMouseNonLeftIgnored(t *testing.T) {
	m := newWithSnap()
	y := lineOf(m.View(), "idle 5m")
	for _, msg := range []tea.MouseMsg{
		{Action: tea.MouseActionPress, Button: tea.MouseButtonRight, Y: y},
		{Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft, Y: y},
		{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown, Y: y},
	} {
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	if len(m.expanded) != 0 {
		t.Errorf("non-left mouse events toggled an agent: %v", m.expanded)
	}
}

// threeAgentFixture is one group of three churning agents with distinct
// activity labels, so a test can locate any row by text.
func threeAgentFixture() tower.Snapshot {
	agent := func(name, doing string) tower.Agent {
		return tower.Agent{
			AgentRef: tower.AgentRef{Name: name, Role: "polecat", Rig: "GasTownTower", Group: "GasTownTower"},
			Status:   tower.StatusChurning,
			Churning: true,
			Stats:    tower.TranscriptStats{Model: "claude-opus-4-8", ContextPct: 0.3, NowDoing: doing},
		}
	}
	return tower.Snapshot{
		GeneratedAt: time.Date(2026, 5, 30, 15, 4, 5, 0, time.UTC),
		Stats:       tower.TownStats{Active: 3, Churning: 3},
		Groups: []tower.Group{{Name: "GasTownTower", Agents: []tower.Agent{
			agent("nux", "doing-aaa"),
			agent("slit", "doing-bbb"),
			agent("rictus", "doing-ccc"),
		}}},
	}
}

// A click must hit the right agent even when earlier rows are expanded and have
// pushed later rows down by a variable number of detail lines.
func TestMouseClickBelowExpandedAgents(t *testing.T) {
	m := New(nil)
	m.snap = threeAgentFixture()
	m.flatten()
	// Expand the first two agents so the third row is shifted down.
	m.expanded["GasTownTower/nux"] = true
	m.expanded["GasTownTower/slit"] = true

	y := lineOf(m.View(), "doing-ccc")
	if y < 0 {
		t.Fatalf("could not locate third agent row:\n%s", m.View())
	}
	next, _ := m.Update(leftClick(y))
	m = next.(Model)
	if !m.expanded["GasTownTower/rictus"] {
		t.Fatalf("click below expanded agents hit the wrong row (cursor=%d):\n%s", m.cursor, m.View())
	}
	if m.cursor != 2 {
		t.Errorf("cursor=%d, want 2", m.cursor)
	}
	// The earlier agents' state is untouched.
	if !m.expanded["GasTownTower/nux"] || !m.expanded["GasTownTower/slit"] {
		t.Errorf("click disturbed earlier agents' expanded state: %v", m.expanded)
	}
}

// agentAtY must respect the dynamic header offset: the same agent is clickable
// regardless of which optional header lines (town, rigs, search, error) are
// present. Each case renders, finds the agent's row by text, and asserts the
// click toggles exactly that agent.
func TestMouseClickAcrossHeaderOffsets(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*Model)
	}{
		{name: "minimal (no town/rigs/search/error)", setup: func(m *Model) {
			m.snap = threeAgentFixture() // single group → rigs line suppressed
		}},
		{name: "town+rigs lines present", setup: func(m *Model) {
			s := fixtureSnapshot() // two rigs → rigs line present
			s.Town = tower.TownStatus{Mail: tower.Mail{Total: 3, Unread: 1}}
			m.snap = s
		}},
		{name: "search line present", setup: func(m *Model) {
			m.snap = fixtureSnapshot()
			m.query = "wit" // filter line renders; witness still matches
		}},
		{name: "error line present", setup: func(m *Model) {
			m.snap = fixtureSnapshot()
			m.err = errTest
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := New(nil)
			tc.setup(&m)
			m.flatten()
			// Click the witness row (idle) — present in every fixture above
			// except the minimal one, which uses doing-ccc instead.
			label, key := "idle 5m", "GigaClip/witness"
			if lineOf(m.View(), label) < 0 {
				label, key = "doing-ccc", "GasTownTower/rictus"
			}
			y := lineOf(m.View(), label)
			if y < 0 {
				t.Fatalf("could not locate target row %q:\n%s", label, m.View())
			}
			next, _ := m.Update(leftClick(y))
			m = next.(Model)
			if !m.expanded[key] {
				t.Errorf("click at Y=%d did not toggle %s:\n%s", y, key, m.View())
			}
		})
	}
}

var errTest = errString("collector unavailable")

type errString string

func (e errString) Error() string { return string(e) }

// The preview region renders by default (previewVisible), showing its dim
// placeholder when no pane text has been captured, and sits between the AGENTS
// and CONVOYS panels.
func TestPreviewPanelDefaultPlaceholder(t *testing.T) {
	m := newWithSnap()
	if !m.previewVisible {
		t.Fatal("preview should be visible by default")
	}
	out := m.View()
	if !strings.Contains(out, "PREVIEW") {
		t.Errorf("View() missing PREVIEW header:\n%s", out)
	}
	if !strings.Contains(out, "— no live tmux session —") {
		t.Errorf("empty preview should show placeholder:\n%s", out)
	}
	// Ordering: PREVIEW between AGENTS and CONVOYS.
	a, p, c := lineOf(out, "AGENTS"), lineOf(out, "PREVIEW"), lineOf(out, "CONVOYS")
	if !(a < p && p < c) {
		t.Errorf("panel order wrong: AGENTS=%d PREVIEW=%d CONVOYS=%d", a, p, c)
	}
}

// 'p' toggles the whole preview region off and on.
func TestPreviewToggle(t *testing.T) {
	m := newWithSnap()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = next.(Model)
	if m.previewVisible {
		t.Fatal("'p' did not hide the preview")
	}
	if strings.Contains(m.View(), "PREVIEW") {
		t.Errorf("hidden preview should not render its header:\n%s", m.View())
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = next.(Model)
	if !m.previewVisible || !strings.Contains(m.View(), "PREVIEW") {
		t.Errorf("'p' did not re-show the preview")
	}
}

// A paneMsg with text populates the preview; the tail (bottom lines, spinner
// included) renders and the placeholder is gone. An error blanks it back to the
// placeholder.
func TestPreviewPaneMsg(t *testing.T) {
	m := newWithSnap()
	m.width, m.height = 80, 30
	text := "old top line\nmiddle line\n✳ Honking… (3s · ↓ 12 tokens)"
	next, _ := m.Update(paneMsg{text: text})
	m = next.(Model)
	out := m.View()
	if !strings.Contains(out, "✳ Honking…") {
		t.Errorf("preview should show captured spinner line:\n%s", out)
	}
	if strings.Contains(out, "— no live tmux session —") {
		t.Errorf("preview with text must not show placeholder:\n%s", out)
	}
	// An error reverts to the placeholder.
	next, _ = m.Update(paneMsg{err: errTest})
	m = next.(Model)
	if !strings.Contains(m.View(), "— no live tmux session —") {
		t.Errorf("errored capture should fall back to placeholder:\n%s", m.View())
	}
}

// previewBody returns exactly n rows regardless of content, keeping the region's
// height stable so the layout never jumps between agents. It shows the tail when
// content exceeds n.
func TestPreviewBodyStableHeight(t *testing.T) {
	m := newWithSnap()
	m.width = 80
	// Empty: still n rows (placeholder + blanks).
	if got := m.previewBody(5); len(got) != 5 {
		t.Errorf("empty previewBody returned %d rows, want 5", len(got))
	}
	// More content than n: exactly n rows, and the tail (last lines) is kept.
	m.previewText = "l1\nl2\nl3\nl4\nl5\nl6\nl7"
	got := m.previewBody(3)
	if len(got) != 3 {
		t.Fatalf("previewBody returned %d rows, want 3", len(got))
	}
	if !strings.Contains(got[2], "l7") {
		t.Errorf("tail not kept: last row %q, want to contain l7", got[2])
	}
	if strings.Contains(strings.Join(got, "\n"), "l1") {
		t.Errorf("top lines should be dropped when tailing: %v", got)
	}
}

// Cursor movement (j/k) issues a preview-refresh command so the region follows
// the selection. The command is non-nil whenever the preview is visible and an
// agent is selected.
func TestPreviewRefreshOnCursorMove(t *testing.T) {
	m := newWithSnap()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if cmd == nil {
		t.Error("j should issue a preview-refresh command")
	}
	// When the preview is hidden, no refresh command is issued.
	m.previewVisible = false
	if c := m.previewCmd(); c != nil {
		t.Error("hidden preview should not issue a refresh command")
	}
}

// clip truncates over-wide lines (no ellipsis) and leaves short lines and the
// no-width case untouched, so a long pane line can never wrap and break the
// fixed preview height.
func TestClip(t *testing.T) {
	if got := clip("hello world", 5); got != "hello" {
		t.Errorf("clip width 5 = %q, want %q", got, "hello")
	}
	if got := clip("short", 80); got != "short" {
		t.Errorf("clip under width changed string: %q", got)
	}
	if got := clip("anything", 0); got != "anything" {
		t.Errorf("clip with no width should be a no-op: %q", got)
	}
}

// '+' grows and '-' shrinks the preview region by one row per press, applied as
// an offset on top of the proportional base height. The chosen offset persists.
func TestPreviewResize(t *testing.T) {
	m := newWithSnap()
	m.width, m.height = 80, 30
	base := m.previewHeight() // proportional base, no offset yet

	press := func(k string) {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
		m = next.(Model)
	}

	press("+")
	if got := m.previewHeight(); got != base+1 {
		t.Errorf("'+' grew height to %d, want %d", got, base+1)
	}
	press("+")
	if got := m.previewHeight(); got != base+2 {
		t.Errorf("second '+' grew height to %d, want %d", got, base+2)
	}
	press("-")
	if got := m.previewHeight(); got != base+1 {
		t.Errorf("'-' shrank height to %d, want %d", got, base+1)
	}
	// '=' is a shift-less convenience alias for '+'.
	press("=")
	if got := m.previewHeight(); got != base+2 {
		t.Errorf("'=' grew height to %d, want %d", got, base+2)
	}
}

// The offset is clamped: '-' never shrinks below the 3-line floor and '+' never
// grows past ~80% of the terminal height, and repeated presses at a bound don't
// build up dead offset (one press back off the bound moves exactly one row).
func TestPreviewResizeClamp(t *testing.T) {
	m := newWithSnap()
	m.width, m.height = 80, 30

	press := func(k string) {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
		m = next.(Model)
	}

	// Shrink hard against the floor.
	for i := 0; i < 50; i++ {
		press("-")
	}
	if got := m.previewHeight(); got != 3 {
		t.Errorf("preview floored at %d, want 3", got)
	}
	// One '+' must move off the floor by exactly one row (no dead offset).
	press("+")
	if got := m.previewHeight(); got != 4 {
		t.Errorf("'+' off the floor gave %d, want 4", got)
	}

	// Grow hard against the ceiling (~80% of height).
	max := m.height * 8 / 10
	for i := 0; i < 50; i++ {
		press("+")
	}
	if got := m.previewHeight(); got != max {
		t.Errorf("preview capped at %d, want %d", got, max)
	}
	// One '-' must move off the ceiling by exactly one row.
	press("-")
	if got := m.previewHeight(); got != max-1 {
		t.Errorf("'-' off the ceiling gave %d, want %d", got, max-1)
	}
}

// The chosen preview size survives moving the cursor between agents and ticks.
func TestPreviewSizePersists(t *testing.T) {
	m := newWithSnap()
	m.width, m.height = 80, 30
	base := m.previewHeight()

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("+")})
	m = next.(Model)
	grown := m.previewHeight()
	if grown != base+1 {
		t.Fatalf("setup: '+' gave %d, want %d", grown, base+1)
	}

	// Arrow to another agent, then a tick fires.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	next, _ = m.Update(tickMsg(time.Now()))
	m = next.(Model)

	if got := m.previewHeight(); got != grown {
		t.Errorf("preview size %d after nav/tick, want %d", got, grown)
	}
}
