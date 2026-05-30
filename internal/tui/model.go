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

// Model is the Bubble Tea model for the live Tower.
type Model struct {
	c        *tower.Collector
	snap     tower.Snapshot
	agents   []tower.Agent // flattened in display order, for cursor navigation
	cursor   int
	expanded map[string]bool
	width    int
	height   int
	err      error
	interval time.Duration
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
		if m.cursor >= len(m.agents) {
			m.cursor = max(0, len(m.agents)-1)
		}
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.agents)-1 {
				m.cursor++
			}
		case "enter", " ":
			if m.cursor < len(m.agents) {
				k := agentKey(m.agents[m.cursor])
				m.expanded[k] = !m.expanded[k]
			}
		case "r":
			return m, m.refresh()
		}
	}
	return m, nil
}

func (m *Model) flatten() {
	m.agents = m.agents[:0]
	for _, g := range m.snap.Groups {
		m.agents = append(m.agents, g.Agents...)
	}
}

// ---- styling ----

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(tower.TownColor))
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	helpStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	churnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#5ad17a")) // green
	selBar     = lipgloss.NewStyle().Foreground(lipgloss.Color(tower.TownColor)).Bold(true)
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

	if m.err != nil {
		b.WriteString("\n  " + dimStyle.Render("error: "+m.err.Error()) + "\n")
	}
	if len(m.agents) == 0 {
		b.WriteString("\n  " + dimStyle.Render("no active agents") + "\n")
	}

	idx := 0
	for _, g := range m.snap.Groups {
		gc := lipgloss.Color(tower.GroupColor(g.Name))
		head := lipgloss.NewStyle().Foreground(gc).Bold(true).Render("▌ " + g.Name)
		b.WriteString("\n" + head + "\n")
		for _, a := range g.Agents {
			b.WriteString(renderAgent(a, idx == m.cursor, gc))
			if m.expanded[agentKey(a)] {
				b.WriteString(renderExpanded(a))
			}
			idx++
		}
	}
	b.WriteString("\n" + helpStyle.Render("↑/↓ move · enter expand · r refresh · q quit") + "\n")
	return b.String()
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
