package tower

import (
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Agent is a discovered agent session with its derived stats.
type Agent struct {
	AgentRef
	TranscriptPath string
	LastActivity   time.Time
	Status         AgentStatus   // idle / churning / awaiting-overseer
	Churning       bool          // web-compat shim: == (Status == StatusChurning)
	Idle           time.Duration // since last activity
	Stats          TranscriptStats
	Hook           string       // active hooked bead ("id: title"), "" when none
	Turn           TurnProgress // current-turn elapsed/tokens (set while churning)
}

// Group is a set of agents sharing a grouping key (town or a rig).
type Group struct {
	Name   string
	Agents []Agent
}

// RigStat summarizes one group's agent counts for the town-level rollup:
// how many sessions are present and how many of them are currently churning.
type RigStat struct {
	Name     string
	Active   int // agents present (discovered within ActiveWindow)
	Churning int // subset currently working
}

// TownStats are the town-wide aggregates derived from a snapshot's agents:
// total active sessions, how many are busy (churning), and a per-group
// breakdown. Computed each snapshot (cheap, no shell-out) so the TUI header,
// the web view, and the text snapshot share one source of truth.
type TownStats struct {
	Active   int
	Churning int
	Rigs     []RigStat
}

// Snapshot is a point-in-time view of all active agents in the town.
type Snapshot struct {
	GeneratedAt time.Time
	Town        TownStatus
	Stats       TownStats
	Groups      []Group
	Events      []Event        // town flow log, newest-first (ring-windowed)
	Convoys     []Convoy       // in-progress + recently-landed town work
	MergeQueue  []MergeRequest // pending merges across all rigs
}

// enrichment bundles the slow-changing town-level data (reputation, mail,
// per-assignee hooks, convoys, and merge queue) fetched by shelling out to
// gt/bd. It is TTL-cached so the fast transcript refresh never pays the
// shell-out cost.
type enrichment struct {
	town       TownStatus
	hooks      map[string]string // assignee -> "id: title"
	prefixes   map[string]string // rig path -> tmux session prefix (from routes.jsonl)
	convoys    []Convoy
	mergeQueue []MergeRequest
}

// Collector reads the live town's artifacts (transcripts, and later gt/bd) and
// assembles snapshots. Construct with NewCollector.
type Collector struct {
	TownRoot     string
	ProjectsDir  string
	ChurnWindow  time.Duration // mtime-fallback quiet window after a completed turn
	TurnWindow   time.Duration // mtime-fallback window while the transcript is mid-turn
	ActiveWindow time.Duration // ignore sessions idle longer than this
	EnrichTTL    time.Duration // how long town/hook enrichment is cached
	now          func() time.Time

	// Injectable loaders (overridable in tests); default to the real gt/bd
	// shell-outs. Each is best-effort — errors leave its data empty.
	loadReputation func(townRoot string) (Reputation, error)
	loadMail       func(townRoot string) (Mail, error)
	loadHooks      func(townRoot string) (map[string]string, error)
	loadPrefixes   func(townRoot string) (map[string]string, error)
	loadConvoys    func(townRoot string) ([]Convoy, error)
	loadMergeQueue func(townRoot string, rigs []string) ([]MergeRequest, error)

	// Liveness probes (injectable in tests); default to real tmux shell-outs.
	// Best-effort — failures degrade churn detection to the mtime heuristic.
	listSessions func() (map[string]struct{}, error)
	capturePane  func(session string) (string, error)

	// overseer gates which agents can read as awaiting-overseer. The zero value
	// is the role default (mayor + crew); NewCollector layers tower.toml on top,
	// and SetOverseerAgents applies an ad-hoc CLI override.
	overseer overseerPolicy

	mu       sync.Mutex
	enrich   enrichment
	enrichAt time.Time

	events eventRing // tails <TownRoot>/.events.jsonl across polls
}

// NewCollector returns a Collector for the given town root with sane defaults.
func NewCollector(townRoot string) *Collector {
	home, _ := os.UserHomeDir()
	// gt runs agent sessions on a per-town named tmux socket. Bind the liveness
	// probes to that socket so pane lookups hit the right server; querying the
	// default socket (the old behavior) finds no sessions and makes working
	// agents read as idle.
	socket := gtSocketName(townRoot)
	// Best-effort: a missing tower.toml leaves the zero policy (role default);
	// a malformed one is ignored here so a bad config never blocks the cockpit.
	cfg, _ := LoadConfig(townRoot)
	return &Collector{
		TownRoot:       townRoot,
		overseer:       newOverseerPolicy(cfg),
		ProjectsDir:    filepath.Join(home, ".claude", "projects"),
		ChurnWindow:    8 * time.Second,
		TurnWindow:     5 * time.Minute,
		ActiveWindow:   30 * time.Minute,
		EnrichTTL:      15 * time.Second,
		now:            time.Now,
		loadReputation: loadReputation,
		loadMail:       loadMail,
		loadHooks:      loadHooks,
		loadPrefixes:   loadSessionPrefixes,
		loadConvoys:    loadConvoys,
		loadMergeQueue: loadMergeQueue,
		listSessions:   func() (map[string]struct{}, error) { return listTmuxSessions(socket) },
		capturePane:    func(session string) (string, error) { return capturePane(socket, session) },
	}
}

// SetOverseerAgents applies an ad-hoc CLI override (--overseer-agents): exactly
// the given addresses become awaiting-overseer eligible, ignoring tower.toml and
// the role default. An empty slice is a no-op, leaving the configured policy in
// place. Call before Snapshot; not safe to race with a live poll.
func (c *Collector) SetOverseerAgents(addrs []string) {
	c.overseer = c.overseer.withOverride(addrs)
}

// Snapshot discovers active agent sessions under the town and derives stats.
func (c *Collector) Snapshot() (Snapshot, error) {
	entries, err := os.ReadDir(c.ProjectsDir)
	if err != nil {
		return Snapshot{}, err
	}
	townSlug := slugify(c.TownRoot)
	now := c.now()
	byGroup := map[string][]Agent{}

	// Enrichment (incl. session prefixes) is cached; the live session set is
	// fetched fresh each pass since liveness is exactly what churn tracks.
	en := c.fetchEnrichment(now)
	sessions, _ := c.listSessions() // best-effort; nil => mtime fallback everywhere

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		segs, ok := segmentsBelow(e.Name(), townSlug)
		if !ok {
			continue
		}
		ref, ok := classifyPath(segs)
		if !ok {
			continue
		}
		tx, mt, ok := latestTranscript(filepath.Join(c.ProjectsDir, e.Name()))
		if !ok || now.Sub(mt) > c.ActiveWindow {
			continue
		}
		stats, err := ParseTranscript(tx)
		if err != nil {
			continue
		}
		status, turn := c.agentStatus(ref, en.prefixes, sessions, mt, now, stats)
		byGroup[ref.Group] = append(byGroup[ref.Group], Agent{
			AgentRef:       ref,
			TranscriptPath: tx,
			LastActivity:   mt,
			Status:         status,
			Churning:       status == StatusChurning,
			Turn:           turn,
			Idle:           now.Sub(mt),
			Stats:          stats,
		})
	}

	snap := assemble(byGroup, now)
	snap.Town = en.town
	snap.Convoys = en.convoys
	snap.MergeQueue = en.mergeQueue
	for gi := range snap.Groups {
		for ai := range snap.Groups[gi].Agents {
			a := &snap.Groups[gi].Agents[ai]
			if a.Addr != "" {
				a.Hook = en.hooks[a.Addr]
			}
		}
	}
	snap.Events = c.events.tail(filepath.Join(c.TownRoot, ".events.jsonl"), now)
	return snap, nil
}

// enrichment returns the cached town/hook enrichment, refetching only when the
// cache is older than EnrichTTL. Best-effort: a failing loader contributes empty
// data but never blocks a snapshot. Safe for concurrent callers.
func (c *Collector) fetchEnrichment(now time.Time) enrichment {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.enrichAt.IsZero() && now.Sub(c.enrichAt) < c.EnrichTTL {
		return c.enrich
	}
	en := enrichment{hooks: map[string]string{}, prefixes: map[string]string{}}
	if rep, err := c.loadReputation(c.TownRoot); err == nil {
		en.town.Reputation = rep
	}
	if mail, err := c.loadMail(c.TownRoot); err == nil {
		en.town.Mail = mail
	}
	if hooks, err := c.loadHooks(c.TownRoot); err == nil {
		en.hooks = hooks
	}
	if c.loadPrefixes != nil {
		if prefixes, err := c.loadPrefixes(c.TownRoot); err == nil {
			en.prefixes = prefixes
		}
	}
	if c.loadConvoys != nil {
		if convoys, err := c.loadConvoys(c.TownRoot); err == nil {
			en.convoys = recentConvoys(convoys, now, 24*time.Hour)
		}
	}
	if c.loadMergeQueue != nil {
		if mq, err := c.loadMergeQueue(c.TownRoot, rigNames(en.prefixes)); err == nil {
			en.mergeQueue = mq
		}
	}
	c.enrich, c.enrichAt = en, now
	return en
}

// agentStatus classifies an agent as idle, churning, or awaiting-overseer and,
// when churning, returns the current-turn progress. The primary signal is the
// agent's tmux pane: the working-marker means mid-turn regardless of transcript
// mtime (a long generate writes nothing to disk until it completes). When no
// pane is available — unknown rig, dead session, capture failure, or tmux
// missing — it falls back to the transcript heuristic.
//
// Awaiting-overseer (orange) is gated on role and freshness: only mayor/crew are
// eligible, and only within ActiveWindow (the signal decays so a long-dead
// session doesn't linger orange). When eligible and fresh, an agent is awaiting
// if it shows a structured pane prompt (paneAwaiting), a pending
// AskUserQuestion/ExitPlanMode (PendingAsk), or a trailing-question text turn
// (TrailingQuestion). A working turn always wins over awaiting.
//
// The churn fallback can't see a long generation or a blocking tool call
// directly: the transcript is written at turn boundaries, so mtime goes quiet
// exactly when the agent is busiest. MidTurn (the last conversation message
// still awaits a reply) recovers that signal, granting the longer TurnWindow so
// a mid-turn agent stays churning across a realistic turn/tool duration. A
// completed turn uses the shorter ChurnWindow quiet window. Either window bounds
// the lie for a session that died mid-turn; ActiveWindow is the outer cap that
// drops it entirely. In the no-pane fallback, awaiting takes precedence over
// mtime-churn so a quiet agent sitting on a question reads orange, not green.
func (c *Collector) agentStatus(ref AgentRef, prefixes map[string]string, sessions map[string]struct{}, mt, now time.Time, stats TranscriptStats) (AgentStatus, TurnProgress) {
	fresh := now.Sub(mt) <= c.ActiveWindow
	eligible := c.overseer.eligible(ref)
	// Transcript-tail awaiting signal (pane-independent), gated on eligibility
	// and freshness. The pane path adds paneAwaiting on top of this.
	txAwaiting := eligible && fresh && (stats.PendingAsk || stats.TrailingQuestion)

	if name, ok := tmuxSession(ref, prefixes); ok && c.capturePane != nil {
		if _, live := sessions[name]; live {
			if pane, err := c.capturePane(name); err == nil {
				turn, _ := parseSpinner(pane)
				switch {
				case paneWorking(pane):
					return StatusChurning, turn
				case txAwaiting || (eligible && fresh && paneAwaiting(pane)):
					return StatusAwaiting, TurnProgress{}
				default:
					return StatusIdle, TurnProgress{}
				}
			}
		}
	}

	// No-pane fallback: awaiting takes precedence over mtime-churn.
	if txAwaiting {
		return StatusAwaiting, TurnProgress{}
	}
	window := c.ChurnWindow
	if stats.MidTurn {
		window = c.TurnWindow
	}
	if now.Sub(mt) <= window {
		return StatusChurning, TurnProgress{}
	}
	return StatusIdle, TurnProgress{}
}

// latestTranscript returns the most-recently-modified *.jsonl in dir.
func latestTranscript(dir string) (path string, mod time.Time, ok bool) {
	matches, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	for _, m := range matches {
		fi, err := os.Stat(m)
		if err != nil {
			continue
		}
		if fi.ModTime().After(mod) {
			path, mod, ok = m, fi.ModTime(), true
		}
	}
	return
}

// assemble orders groups (town first, then rigs alphabetically) and agents
// within a group (churning first, then by name).
func assemble(byGroup map[string][]Agent, now time.Time) Snapshot {
	var names []string
	for g := range byGroup {
		names = append(names, g)
	}
	sort.Slice(names, func(i, j int) bool {
		if names[i] == townGroup {
			return true
		}
		if names[j] == townGroup {
			return false
		}
		return names[i] < names[j]
	})
	snap := Snapshot{GeneratedAt: now}
	for _, g := range names {
		agents := byGroup[g]
		// Stable ordering: sort by name only. Churn state must NOT affect
		// position (otherwise agents jump around as they start/stop working);
		// it is conveyed by the dot, not the row order.
		sort.Slice(agents, func(i, j int) bool {
			return agents[i].Name < agents[j].Name
		})
		churning := 0
		for _, a := range agents {
			if a.Churning {
				churning++
			}
		}
		snap.Stats.Active += len(agents)
		snap.Stats.Churning += churning
		snap.Stats.Rigs = append(snap.Stats.Rigs, RigStat{Name: g, Active: len(agents), Churning: churning})
		snap.Groups = append(snap.Groups, Group{Name: g, Agents: agents})
	}
	return snap
}
