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

// paneMsg carries the result of an async tmux pane capture for the selected
// agent. err non-nil (no live session, capture failure) makes the preview fall
// back to its dim placeholder.
type paneMsg struct {
	text string
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
	c              *tower.Collector
	snap           tower.Snapshot
	agents         []tower.Agent // flattened (and search-filtered) display order
	cursor         int           // selected agent within the AGENTS panel
	expanded       map[string]bool
	focus          panel  // which panel receives scroll/enter
	query          string // active search filter ("" = no filter)
	searching      bool   // true while the user is typing a search query
	showAllEvents  bool   // false = curated flow; 't' toggles faithful (show-all)
	convoyScroll   int
	eventScroll    int
	previewText    string // last captured tmux pane text for the selected agent
	previewVisible bool   // 'p' toggles the live-preview region (default on)
	width          int
	height         int
	err            error
	interval       time.Duration
}

// New builds a Model that refreshes from the given collector.
func New(c *tower.Collector) Model {
	return Model{c: c, expanded: map[string]bool{}, previewVisible: true, interval: 1500 * time.Millisecond}
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

// selectedAgent returns the agent under the AGENTS cursor, or ok=false when the
// (possibly filtered) list is empty.
func (m Model) selectedAgent() (tower.Agent, bool) {
	if m.cursor >= 0 && m.cursor < len(m.agents) {
		return m.agents[m.cursor], true
	}
	return tower.Agent{}, false
}

// capturePaneCmd captures the given agent's live tmux pane off the UI thread and
// reports it back as a paneMsg. The blocking tmux exec happens inside the
// returned tea.Cmd (a goroutine), never in Update/View.
func (m Model) capturePaneCmd(a tower.Agent) tea.Cmd {
	return func() tea.Msg {
		text, err := m.c.CapturePane(a)
		return paneMsg{text: text, err: err}
	}
}

// previewCmd captures the currently-selected agent's pane, or nil when there is
// nothing selected (empty list) or the preview region is hidden. Always follows
// the AGENTS cursor, regardless of which panel currently has focus.
func (m Model) previewCmd() tea.Cmd {
	if !m.previewVisible {
		return nil
	}
	if a, ok := m.selectedAgent(); ok {
		return m.capturePaneCmd(a)
	}
	return nil
}

func agentKey(a tower.Agent) string { return a.Group + "/" + a.Name }

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tickMsg:
		// Re-capture the selected agent's pane every tick so the preview keeps
		// churning live (spinner and all) even when the cursor isn't moving.
		return m, tea.Batch(m.refresh(), m.tickCmd(), m.previewCmd())
	case paneMsg:
		// On capture error (no live session, tmux missing) blank the text so the
		// region renders its dim placeholder rather than stale output.
		if msg.err != nil {
			m.previewText = ""
		} else {
			m.previewText = msg.text
		}
	case snapMsg:
		m.snap, m.err = msg.snap, msg.err
		m.flatten()
		m.clampCursor()
		// Refresh the preview as soon as agents (re)load: the cursor may now point
		// at a different agent, and this populates the region before the first tick.
		return m, m.previewCmd()
	case tea.MouseMsg:
		// A left-button press on an agent's row focuses the AGENTS panel, moves
		// the cursor there, and toggles that agent's expanded detail — the same
		// effect as navigating to it and pressing enter. Clicks elsewhere (group
		// headers, blank space, other panels) miss and are a no-op.
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			if i := m.agentAtY(msg.Y); i >= 0 {
				m.focus = panelAgents
				m.cursor = i
				k := agentKey(m.agents[i])
				m.expanded[k] = !m.expanded[k]
				return m, m.previewCmd() // cursor moved — refresh the preview
			}
		}
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
		case "t":
			// Toggle the EVENTS panel between curated flow and show-all.
			m.showAllEvents = !m.showAllEvents
			m.eventScroll = 0
		case "up", "k":
			m.scroll(-1)
			return m, m.previewCmd() // cursor may have moved — refresh preview
		case "down", "j":
			m.scroll(+1)
			return m, m.previewCmd() // cursor may have moved — refresh preview
		case "p":
			// Toggle the live-preview region. Re-show fires an immediate
			// capture so it isn't blank until the next tick.
			m.previewVisible = !m.previewVisible
			return m, m.previewCmd()
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
		m.eventScroll = clampScroll(m.eventScroll+delta, len(m.visibleEvents()))
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

// convoyCount reports how many rows the convoys panel currently has, so the
// focus/scroll machinery can bound the viewport. The panel stacks convoy rows
// above the merge-queue rows.
func convoyCount(s tower.Snapshot) int { return len(s.Convoys) + len(s.MergeQueue) }

// visibleEvents is the EVENTS panel's display list: the snapshot's newest-first
// events with the curated filter (unless show-all) and the active search query
// applied.
func (m Model) visibleEvents() []tower.Event {
	out := make([]tower.Event, 0, len(m.snap.Events))
	for _, e := range m.snap.Events {
		if !m.showAllEvents && !e.Curated() {
			continue
		}
		if !m.matchesEvent(e) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// matchesEvent reports whether an event passes the active search filter. An empty
// query matches everything; matching is case-insensitive across the event's type,
// actor, and humanized summary.
func (m Model) matchesEvent(e tower.Event) bool {
	if m.query == "" {
		return true
	}
	hay := strings.ToLower(e.Type + " " + e.Actor + " " + eventSummary(e))
	return strings.Contains(hay, strings.ToLower(m.query))
}

// ---- styling ----

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(tower.TownColor))
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	helpStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	churnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#5ad17a")) // green
	awaitStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#e8a33d")) // orange — awaiting overseer
	selBar     = lipgloss.NewStyle().Foreground(lipgloss.Color(tower.TownColor)).Bold(true)
	panelStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("250"))
)

// View implements tea.Model. It is pure (no TTY needed) so it is unit-testable.
func (m Model) View() string {
	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString(m.renderAgentsPanel())
	b.WriteString(m.renderPreviewPanel())
	b.WriteString(m.renderConvoysPanel())
	b.WriteString(m.renderEventsPanel())

	b.WriteString("\n" + helpStyle.Render("tab focus · j/k scroll · enter/click expand · / search · t all-events · p preview · q quit") + "\n")
	return b.String()
}

// renderHeader renders everything above the AGENTS panel: the title/summary
// line plus the optional town, rigs, search, and error lines. It is the single
// source of truth for the AGENTS panel's vertical offset — the mouse hit-test
// (agentAtY) counts the lines this produces, so the two cannot drift.
func (m Model) renderHeader() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("GAS TOWN ACTIVITY TOWER"))
	b.WriteString("  " + dimStyle.Render(m.snap.GeneratedAt.Format("15:04:05")))
	b.WriteString("  " + dimStyle.Render(activeSummary(m.snap.Stats)) + "\n")

	if town := renderTown(m.snap.Town); town != "" {
		b.WriteString(town + "\n")
	}
	if rigs := renderRigs(m.snap.Stats); rigs != "" {
		b.WriteString(rigs + "\n")
	}
	if line := m.renderSearch(); line != "" {
		b.WriteString(line + "\n")
	}

	if m.err != nil {
		b.WriteString("\n  " + dimStyle.Render("error: "+m.err.Error()) + "\n")
	}
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

// agentAtY maps an absolute terminal row (a mouse click's Y, 0-based) to the
// index into m.agents of the agent whose row occupies that line, or -1 if the
// click did not land on an agent row (header lines, group headers, expanded
// detail lines, or anything below the AGENTS panel).
//
// It deliberately replays renderAgentsPanel's exact layout so the two cannot
// drift: it counts the header lines renderHeader emits, skips the two AGENTS
// panel-header lines (the blank line + "▌ AGENTS" that panelHeader produces),
// then walks the same group/agent iteration, advancing one line per agent row
// and renderExpanded's line count for each expanded agent above the target.
func (m Model) agentAtY(y int) int {
	if len(m.agents) == 0 {
		return -1
	}
	// Lines above the AGENTS panel, then its blank + "▌ AGENTS" header lines.
	line := strings.Count(m.renderHeader(), "\n") + 2
	idx := 0
	for _, g := range m.snap.Groups {
		visible := 0
		for _, a := range g.Agents {
			if m.matches(a) {
				visible++
			}
		}
		if visible == 0 {
			continue // group with no visible rows is not rendered (no header)
		}
		line++ // the group header line ("▌ <group>")
		for _, a := range g.Agents {
			if !m.matches(a) {
				continue
			}
			if y == line {
				return idx
			}
			line++ // the agent's own row
			if m.expanded[agentKey(a)] {
				line += strings.Count(renderExpanded(a), "\n") // expanded detail
			}
			idx++
		}
	}
	return -1
}

// previewHeight is the number of pane-text rows the preview region reserves:
// proportional (~30%) to the terminal height and fixed regardless of how much
// pane content exists, so the region never grows/shrinks as the operator arrows
// between agents (the "stable, no layout jump" requirement). Floors keep it
// usable on tiny terminals and before the first WindowSizeMsg arrives.
func (m Model) previewHeight() int {
	if m.height <= 0 {
		return 8 // no size yet; a sane default until WindowSizeMsg lands
	}
	h := m.height * 3 / 10
	if h < 3 {
		h = 3
	}
	return h
}

// renderPreviewPanel renders the live tmux pane preview for the selected agent,
// between the AGENTS and CONVOYS panels. Hidden entirely when previewVisible is
// off ('p'). The body is a fixed previewHeight() rows — the tail (bottom lines)
// of the captured pane, spinner line included — padded with blanks to keep the
// layout height stable. A dim placeholder shows when there is no pane text.
func (m Model) renderPreviewPanel() string {
	if !m.previewVisible {
		return ""
	}
	sub := ""
	if a, ok := m.selectedAgent(); ok {
		if sub = shortAddr(a.Addr); sub == "" {
			sub = a.Name
		}
	}
	var b strings.Builder
	b.WriteString(panelHeader("PREVIEW", sub, false))
	for _, ln := range m.previewBody(m.previewHeight()) {
		b.WriteString(ln + "\n")
	}
	return b.String()
}

// previewBody returns exactly n rendered rows for the preview region: the bottom
// n lines (tail) of the captured pane text, each clipped to the terminal width
// so a long line never wraps and breaks the fixed height. When there is no pane
// text it returns the dim placeholder; the result is always padded with blank
// lines to n rows so the region's height stays constant.
func (m Model) previewBody(n int) []string {
	out := make([]string, 0, n)
	text := strings.TrimRight(m.previewText, "\n")
	if strings.TrimSpace(text) == "" {
		out = append(out, "  "+dimStyle.Render("— no live tmux session —"))
	} else {
		lines := strings.Split(text, "\n")
		if len(lines) > n {
			lines = lines[len(lines)-n:] // tail: keep the bottom (spinner included)
		}
		for _, ln := range lines {
			out = append(out, "  "+clip(ln, m.width-2))
		}
	}
	for len(out) < n {
		out = append(out, "")
	}
	return out
}

// clip truncates s to at most w runes (no ellipsis — preview rows are raw pane
// text). A non-positive width disables clipping (no size known yet).
func clip(s string, w int) string {
	if w <= 0 {
		return s
	}
	if r := []rune(s); len(r) > w {
		return string(r[:w])
	}
	return s
}

func (m Model) renderConvoysPanel() string {
	var b strings.Builder
	b.WriteString(panelHeader("CONVOYS", "in-progress · landed 24h", m.focus == panelConvoys))
	if len(m.snap.Convoys) == 0 {
		b.WriteString("  " + dimStyle.Render("no active convoys") + "\n")
	} else {
		for _, cv := range m.snap.Convoys {
			b.WriteString(renderConvoy(cv))
		}
	}
	b.WriteString(m.renderMergeQueue())
	return b.String()
}

func renderConvoy(cv tower.Convoy) string {
	glyph := dimStyle.Render("○")
	switch {
	case cv.Status == "closed":
		glyph = churnStyle.Render("✓")
	case cv.Completed > 0:
		glyph = lipgloss.NewStyle().Foreground(lipgloss.Color("221")).Render("◐") // partial
	}
	title := strings.TrimPrefix(cv.Title, "Work: ")
	line := fmt.Sprintf("  %s %-4s %s", glyph, fmt.Sprintf("%d/%d", cv.Completed, cv.Total), title)
	if who := convoyWho(cv); who != "" {
		line += "  " + dimStyle.Render(who)
	}
	return line + "\n"
}

// convoyWho lists the distinct assignees of a convoy's tracked beads (leaf names
// only), so the operator sees who is carrying the work.
func convoyWho(cv tower.Convoy) string {
	var names []string
	seen := map[string]bool{}
	for _, t := range cv.Tracked {
		if t.Assignee == "" {
			continue
		}
		n := leafName(t.Assignee)
		if seen[n] {
			continue
		}
		seen[n] = true
		names = append(names, n)
	}
	return strings.Join(names, ", ")
}

func (m Model) renderMergeQueue() string {
	mq := m.snap.MergeQueue
	head := "  " + dimStyle.Render(fmt.Sprintf("merge queue · %d pending", len(mq))) + "\n"
	if len(mq) == 0 {
		return head
	}
	var b strings.Builder
	b.WriteString(head)
	for _, mr := range mq {
		id := mr.SourceIssue
		if id == "" {
			id = mr.ID
		}
		meta := joinNonEmpty([]string{mr.Rig, mr.Worker, mr.Status}, " · ")
		line := "    ↳ " + id
		if meta != "" {
			line += "  " + dimStyle.Render(meta)
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

func leafName(addr string) string {
	if i := strings.LastIndex(addr, "/"); i >= 0 {
		return addr[i+1:]
	}
	return addr
}

func joinNonEmpty(parts []string, sep string) string {
	var kept []string
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, sep)
}

// eventsViewport caps how many event rows render at once; eventScroll indexes
// the topmost visible (newest-first) event so j/k pages through the flow.
const eventsViewport = 12

func (m Model) renderEventsPanel() string {
	sub := "curated flow, newest-first"
	if m.showAllEvents {
		sub = "all events, newest-first"
	}
	b := panelHeader("EVENTS", sub, m.focus == panelEvents)

	evs := m.visibleEvents()
	if len(evs) == 0 {
		return b + "  " + dimStyle.Render("(no events)") + "\n"
	}

	top := clampScroll(m.eventScroll, len(evs))
	end := top + eventsViewport
	if end > len(evs) {
		end = len(evs)
	}
	var sb strings.Builder
	sb.WriteString(b)
	for _, e := range evs[top:end] {
		sb.WriteString(renderEvent(e))
	}
	if len(evs) > eventsViewport {
		sb.WriteString("  " + helpStyle.Render(fmt.Sprintf("%d–%d of %d", top+1, end, len(evs))) + "\n")
	}
	return sb.String()
}

// renderEvent formats one flow-log line: "HH:MM type actor  summary".
func renderEvent(e tower.Event) string {
	ts := dimStyle.Render(e.TS.Format("15:04"))
	typ := eventTypeStyle(e.Type).Render(fmt.Sprintf("%-16s", e.Type))
	line := fmt.Sprintf("  %s %s %s", ts, typ, shortAddr(e.Actor))
	if sum := eventSummary(e); sum != "" {
		line += "  " + dimStyle.Render(trunc(sum, 48))
	}
	return line + "\n"
}

// eventSummary pulls a short human-readable detail from an event's payload,
// keyed by type. Returns "" when nothing useful is present.
func eventSummary(e tower.Event) string {
	s := func(k string) string {
		if v, ok := e.Payload[k].(string); ok {
			return v
		}
		return ""
	}
	switch e.Type {
	case "sling":
		return arrow(s("bead"), shortAddr(s("target")))
	case "done":
		return s("bead")
	case "unhook":
		return s("bead")
	case "handoff":
		return s("subject")
	case "mail":
		return arrow(s("subject"), shortAddr(s("to")))
	case "spawn":
		return arrow(s("rig"), s("polecat"))
	case "nudge":
		return s("reason")
	case "session_start":
		return s("role")
	}
	if strings.HasPrefix(e.Type, "escalation_") {
		if r := s("reason"); r != "" {
			return r
		}
		return s("escalation_id")
	}
	return ""
}

// eventTypeStyle accents a handful of event types so escalations and completions
// stand out in the flow; everything else uses the neutral panel style.
func eventTypeStyle(t string) lipgloss.Style {
	switch {
	case strings.HasPrefix(t, "escalation_"):
		return lipgloss.NewStyle().Foreground(lipgloss.Color("203")) // red
	case t == "done":
		return churnStyle
	default:
		return panelStyle
	}
}

// arrow joins two fields as "a → b", or returns whichever is non-empty.
func arrow(a, b string) string {
	switch {
	case a != "" && b != "":
		return a + " → " + b
	case a != "":
		return a
	default:
		return b
	}
}

// shortAddr keeps the last two path segments of an agent address for readability
// ("GasTownTower/polecats/furiosa" → "polecats/furiosa").
func shortAddr(s string) string {
	parts := strings.Split(strings.TrimSuffix(s, "/"), "/")
	if len(parts) >= 2 {
		return strings.Join(parts[len(parts)-2:], "/")
	}
	return s
}

// trunc shortens s to max runes with an ellipsis, collapsing newlines first.
func trunc(s string, max int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	r := []rune(s)
	if len(r) > max {
		return string(r[:max-1]) + "…"
	}
	return s
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

// activeSummary renders the town-wide vital signs for the header: total active
// sessions and how many are busy (churning). These are town aggregates and are
// deliberately independent of the AGENTS-panel search filter, which scopes only
// the list below.
func activeSummary(s tower.TownStats) string {
	out := fmt.Sprintf("%d active", s.Active)
	if s.Churning > 0 {
		out += fmt.Sprintf(" · %d busy", s.Churning)
	}
	return out
}

// renderRigs renders the per-group breakdown line: each group's name with its
// active count and, when any are working, a green churning tally. Returns "" for
// a town with a single group (nothing to break down) so the line is suppressed.
func renderRigs(s tower.TownStats) string {
	if len(s.Rigs) <= 1 {
		return ""
	}
	var parts []string
	for _, r := range s.Rigs {
		seg := dimStyle.Render(fmt.Sprintf("%s %d", r.Name, r.Active))
		if r.Churning > 0 {
			seg += churnStyle.Render(fmt.Sprintf(" %d●", r.Churning))
		}
		parts = append(parts, seg)
	}
	return "  " + strings.Join(parts, dimStyle.Render("   "))
}

func renderAgent(a tower.Agent, selected bool, gc lipgloss.Color) string {
	dot := dimStyle.Render("○")
	switch a.Status {
	case tower.StatusChurning:
		dot = churnStyle.Render("●")
	case tower.StatusAwaiting:
		dot = awaitStyle.Render("●")
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
	switch a.Status {
	case tower.StatusAwaiting:
		activity = awaitStyle.Render("awaiting overseer")
	case tower.StatusChurning:
		if a.Turn.Elapsed > 0 {
			activity = dimStyle.Render(roundDur(a.Turn.Elapsed).String()+" ") + activity
		}
	default: // idle
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
