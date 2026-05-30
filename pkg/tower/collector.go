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
	Churning       bool
	Idle           time.Duration // since last activity
	Stats          TranscriptStats
	Hook           string // active hooked bead ("id: title"), "" when none
}

// Group is a set of agents sharing a grouping key (town or a rig).
type Group struct {
	Name   string
	Agents []Agent
}

// Snapshot is a point-in-time view of all active agents in the town.
type Snapshot struct {
	GeneratedAt time.Time
	Town        TownStatus
	Groups      []Group
}

// enrichment bundles the slow-changing town-level data (reputation, mail, and
// per-assignee hooks) fetched by shelling out to gt/bd. It is TTL-cached so the
// fast transcript refresh never pays the shell-out cost.
type enrichment struct {
	town  TownStatus
	hooks map[string]string // assignee -> "id: title"
}

// Collector reads the live town's artifacts (transcripts, and later gt/bd) and
// assembles snapshots. Construct with NewCollector.
type Collector struct {
	TownRoot     string
	ProjectsDir  string
	ChurnWindow  time.Duration // active write within this => "churning"
	ActiveWindow time.Duration // ignore sessions idle longer than this
	EnrichTTL    time.Duration // how long town/hook enrichment is cached
	now          func() time.Time

	// Injectable loaders (overridable in tests); default to the real gt/bd
	// shell-outs. Each is best-effort — errors leave its data empty.
	loadReputation func(townRoot string) (Reputation, error)
	loadMail       func(townRoot string) (Mail, error)
	loadHooks      func(townRoot string) (map[string]string, error)

	mu       sync.Mutex
	enrich   enrichment
	enrichAt time.Time
}

// NewCollector returns a Collector for the given town root with sane defaults.
func NewCollector(townRoot string) *Collector {
	home, _ := os.UserHomeDir()
	return &Collector{
		TownRoot:       townRoot,
		ProjectsDir:    filepath.Join(home, ".claude", "projects"),
		ChurnWindow:    8 * time.Second,
		ActiveWindow:   30 * time.Minute,
		EnrichTTL:      15 * time.Second,
		now:            time.Now,
		loadReputation: loadReputation,
		loadMail:       loadMail,
		loadHooks:      loadHooks,
	}
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
		byGroup[ref.Group] = append(byGroup[ref.Group], Agent{
			AgentRef:       ref,
			TranscriptPath: tx,
			LastActivity:   mt,
			Churning:       now.Sub(mt) <= c.ChurnWindow,
			Idle:           now.Sub(mt),
			Stats:          stats,
		})
	}

	en := c.fetchEnrichment(now)
	snap := assemble(byGroup, now)
	snap.Town = en.town
	for gi := range snap.Groups {
		for ai := range snap.Groups[gi].Agents {
			a := &snap.Groups[gi].Agents[ai]
			if a.Addr != "" {
				a.Hook = en.hooks[a.Addr]
			}
		}
	}
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
	en := enrichment{hooks: map[string]string{}}
	if rep, err := c.loadReputation(c.TownRoot); err == nil {
		en.town.Reputation = rep
	}
	if mail, err := c.loadMail(c.TownRoot); err == nil {
		en.town.Mail = mail
	}
	if hooks, err := c.loadHooks(c.TownRoot); err == nil {
		en.hooks = hooks
	}
	c.enrich, c.enrichAt = en, now
	return en
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
		snap.Groups = append(snap.Groups, Group{Name: g, Agents: agents})
	}
	return snap
}
