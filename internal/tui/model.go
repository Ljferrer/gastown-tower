// Package tui renders the Activity Tower as a live terminal UI (Bubble Tea).
package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/Ljferrer/gastown-tower/pkg/tower"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type tickMsg time.Time

type snapMsg struct {
	snap tower.Snapshot
	err  error
}

// panel identifies one of the stacked panels in the cockpit. The focused panel
// receives j/k scroll and enter; tab cycles focus across all of them.
type panel int

const (
	panelAgents  panel = iota // grouped agents (town / rigs) — fully populated
	panelConvoys              // convoys + merge queue — data lands in a later phase
	panelEvents               // curated event stream — data lands in a later phase
	panelCount
)

// Model is the Bubble Tea model for the live Tower.
type Model struct {
	c            *tower.Collector
	snap         tower.Snapshot
	agents       []tower.Agent // flattened (and search-filtered) display order
	cursor       int           // selected agent within the AGENTS panel
	expanded     map[string]bool
	focus        panel  // which panel receives scroll/enter
	query        string // active search filter ("" = no filter)
	searching    bool   // true while the user is typing a search query
	convoyScroll int
	eventScroll  int
	width        int
	height       int
	err          error
	interval     time.Duration
}

// New builds a Model that refreshes from the given collector.
func New(c *tower.Collector) Model {
	return Model{c: c, expanded: map[string]bool{}, interval: 1500 * time.Millisecond}
}

func (m Model) Init() tea.Cmd { return tea.Batch(m.refresh(), m.tickCmd()) }

func (m Model) tickCmd() tea.Cmd {
	return tea.Tick(m.interval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) refresh() tea.Cmd {
	return func() tea.Msg {
		s, e := m.c.Snapshot()
		return snapMsg{s, e}
	}
}

func agentKey(a tower.Agent) string { return a.Group + "/" + a.Name }

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tickMsg:
		return m, tea.Batch(m.refresh(), m.tickCmd())
	case snapMsg:
		m.snap, m.err = msg.snap, msg.err
		m.flatten()
		m.clampCursor()
	case tea.KeyMsg:
		if m.searching {
			return m.updateSearch(msg)
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			m.focus = (m.focus + 1) % panelCount
		case "shift+tab":
			m.focus = (m.focus + panelCount - 1) % panelCount
		case "/":
			m.searching = true
		case "up", "k":
			m.scroll(-1)
		case "down", "j":
			m.scroll(+1)
		case "enter", " ":
			if m.focus == panelAgents && m.cursor < len(m.agents) {
				k := agentKey(m.agents[m.cursor])
				m.expanded[k] = !m.expanded[k]
			}
		case "r":
			return m, m.refresh()
		}
	}
	return m, nil
}

// updateSearch handles key input while the search prompt is active.
func (m Model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		m.searching = false // commit: keep the filter applied
	case tea.KeyEsc:
		m.searching = false
		m.query = ""
		m.flatten()
		m.clampCursor()
	case tea.KeyBackspace, tea.KeyDelete:
		if r := []rune(m.query); len(r) > 0 {
			m.query = string(r[:len(r)-1])
			m.flatten()
			m.clampCursor()
		}
	case tea.KeyRunes, tea.KeySpace:
		m.query += string(msg.Runes)
		m.flatten()
		m.clampCursor()
	}
	return m, nil
}

// scroll moves the selection/viewport of the focused panel by delta.
func (m *Model) scroll(delta int) {
	switch m.focus {
	case panelAgents:
		m.cursor = clampScroll(m.cursor+delta, len(m.agents))
	case panelConvoys:
		m.convoyScroll = clampScroll(m.convoyScroll+delta, convoyCount(m.snap))
	case panelEvents:
		m.eventScroll = clampScroll(m.eventScroll+delta, eventCount(m.snap))
	}
}

// clampScroll keeps an index within [0, n) (and at 0 when the list is empty).
func clampScroll(i, n int) int {
	if i < 0 || n == 0 {
		return 0
	}
	if i >= n {
		return n - 1
	}
	return i
}

func (m *Model) clampCursor() {
	if m.cursor >= len(m.agents) {
		m.cursor = max(0, len(m.agents)-1)
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *Model) flatten() {
	m.agents = m.agents[:0]
	for _, g := range m.snap.Groups {
		for _, a := range g.Agents {
			if m.matches(a) {
				m.agents = append(m.agents, a)
			}
		}
	}
}

// matches reports whether an agent passes the active search filter. An empty
// query matches everything. Matching is case-insensitive across the agent's
// identity and current activity.
func (m Model) matches(a tower.Agent) bool {
	if m.query == "" {
		return true
	}
	hay := strings.ToLower(strings.Join(
		[]string{a.Name, a.Role, a.Group, a.Rig, a.Stats.NowDoing, a.Hook}, " "))
	return strings.Contains(hay, strings.ToLower(m.query))
}

// convoyCount and eventCount report how many rows each panel currently has.
// These data sources arrive in later phases (gtt-975.4 / gtt-975.5); until then
// the panels are empty but the focus/scroll machinery is wired and tested.
func convoyCount(tower.Snapshot) int { return 0 }
func eventCount(tower.Snapshot) int  { return 0 }

// ---- styling ----

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(tower.TownColor))
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	helpStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	churnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#5ad17a")) // green
	selBar     = lipgloss.NewStyle().Foreground(lipgloss.Color(tower.TownColor)).Bold(true)
	panelStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("250"))
)

// View implements tea.Model. It is pure (no TTY needed) so it is unit-testable.
func (m Model) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("GAS TOWN ACTIVITY TOWER"))
	b.WriteString("  " + dimStyle.Render(m.snap.GeneratedAt.Format("15:04:05")))
	b.WriteString("  " + dimStyle.Render(fmt.Sprintf("%d active", len(m.agents))) + "\n")

	if town := renderTown(m.snap.Town); town != "" {
		b.WriteString(town + "\n")
	}
	if line := m.renderSearch(); line != "" {
		b.WriteString(line + "\n")
	}

	if m.err != nil {
		b.WriteString("\n  " + dimStyle.Render("error: "+m.err.Error()) + "\n")
	}

	b.WriteString(m.renderAgentsPanel())
	b.WriteString(m.renderConvoysPanel())
	b.WriteString(m.renderEventsPanel())

	b.WriteString("\n" + helpStyle.Render("tab focus · j/k scroll · enter expand · / search · q quit") + "\n")
	return b.String()
}

// panelHeader renders a stacked-panel divider. The focused panel is marked with
// a leading bar ("▌ TITLE") so the operator can see where tab/j/k land.
func panelHeader(title, subtitle string, focused bool) string {
	marker := "  "
	style := panelStyle
	if focused {
		marker = "▌ "
		style = selBar
	}
	head := style.Render(marker + title)
	if subtitle != "" {
		head += " " + dimStyle.Render(subtitle)
	}
	return "\n" + head + "\n"
}

func (m Model) renderSearch() string {
	switch {
	case m.searching:
		return "  " + dimStyle.Render("search: ") + m.query + selBar.Render("▏")
	case m.query != "":
		return "  " + dimStyle.Render(fmt.Sprintf("filter: %q (esc to clear)", m.query))
	}
	return ""
}

func (m Model) renderAgentsPanel() string {
	var b strings.Builder
	b.WriteString(panelHeader("AGENTS", "town / rigs", m.focus == panelAgents))
	if len(m.agents) == 0 {
		b.WriteString("  " + dimStyle.Render("no active agents") + "\n")
		return b.String()
	}
	idx := 0
	for _, g := range m.snap.Groups {
		gc := lipgloss.Color(tower.GroupColor(g.Name))
		var rows strings.Builder
		for _, a := range g.Agents {
			if !m.matches(a) {
				continue
			}
			rows.WriteString(renderAgent(a, idx == m.cursor, gc))
			if m.expanded[agentKey(a)] {
				rows.WriteString(renderExpanded(a))
			}
			idx++
		}
		if rows.Len() == 0 {
			continue // every agent in this group filtered out
		}
		head := lipgloss.NewStyle().Foreground(gc).Bold(true).Render("▌ " + g.Name)
		b.WriteString(head + "\n")
		b.WriteString(rows.String())
	}
	return b.String()
}

func (m Model) renderConvoysPanel() string {
	b := panelHeader("CONVOYS", "in-progress · landed 24h", m.focus == panelConvoys)
	return b + "  " + dimStyle.Render("(no convoy data yet)") + "\n"
}

func (m Model) renderEventsPanel() string {
	b := panelHeader("EVENTS", "curated flow, newest-first", m.focus == panelEvents)
	return b + "  " + dimStyle.Render("(no event data yet)") + "\n"
}

// renderTown formats the town-status line: mail envelope with unread/total and
// the reputation tier with stamp count. Returns "" when nothing is known.
func renderTown(t tower.TownStatus) string {
	var parts []string
	if t.Mail.Total > 0 || t.Mail.Unread > 0 {
		parts = append(parts, fmt.Sprintf("✉ %d/%d", t.Mail.Unread, t.Mail.Total))
	}
	if t.Reputation.Tier != "" {
		rep := "🏅 " + t.Reputation.Tier
		if t.Reputation.Stamps > 0 {
			rep += fmt.Sprintf(" · %d stamps", t.Reputation.Stamps)
		}
		parts = append(parts, rep)
	}
	if len(parts) == 0 {
		return ""
	}
	return "  " + dimStyle.Render(strings.Join(parts, "   "))
}

func renderAgent(a tower.Agent, selected bool, gc lipgloss.Color) string {
	dot := dimStyle.Render("○")
	if a.Churning {
		dot = churnStyle.Render("●")
	}
	cursor := "  "
	name := a.Name
	if selected {
		cursor = selBar.Render("▸ ")
		name = lipgloss.NewStyle().Bold(true).Render(name)
	}
	pct := a.Stats.ContextPct
	meter := lipgloss.NewStyle().Foreground(gc).Render(bar(pct))
	activity := a.Stats.NowDoing
	if !a.Churning {
		activity = dimStyle.Render("idle " + roundDur(a.Idle).String())
	} else if a.Turn.Elapsed > 0 {
		activity = dimStyle.Render(roundDur(a.Turn.Elapsed).String()+" ") + activity
	}
	hook := ""
	if a.Hook != "" {
		hook = " " + dimStyle.Render("🪝")
	}
	return fmt.Sprintf("%s%s %-12s %s %3.0f%%  %s%s\n",
		cursor, dot, name, meter, pct*100, activity, hook)
}

func renderExpanded(a tower.Agent) string {
	s := a.Stats
	win := tower.ContextWindow(s.Model, s.ContextTokens)
	lines := []string{
		fmt.Sprintf("model %s · ctx %s/%s tok", short(s.Model), human(s.ContextTokens), human(win)),
		fmt.Sprintf("%d turns · %d tools · %d reads · %s out", s.Turns, s.ToolCalls, s.FileReads, human(s.OutputTokens)),
		fmt.Sprintf("role %s · rig %s · last %s ago", a.Role, orDash(a.Rig), roundDur(a.Idle)),
	}
	if a.Hook != "" {
		lines = append(lines, "hook "+a.Hook)
	}
	var b strings.Builder
	for _, ln := range lines {
		b.WriteString("       " + dimStyle.Render(ln) + "\n")
	}
	return b.String()
}

// ---- small render helpers ----

func bar(pct float64) string {
	const cells = 6
	filled := int(pct*float64(cells) + 0.5)
	if filled > cells {
		filled = cells
	}
	return strings.Repeat("▓", filled) + strings.Repeat("░", cells-filled)
}

func short(model string) string {
	if model == "" {
		return "?"
	}
	return strings.TrimPrefix(model, "claude-")
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func human(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1_000:
		return fmt.Sprintf("%.0fk", float64(n)/1e3)
	}
	return fmt.Sprintf("%d", n)
}

func roundDur(d time.Duration) time.Duration {
	if d < time.Minute {
		return d.Round(time.Second)
	}
	return d.Round(time.Minute)
}
