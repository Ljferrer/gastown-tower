package tower

import (
	"encoding/json"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// Convoy is a tracked unit of town work spanning one or more beads, from
// `gt convoy list`. Status is "open" (in-progress) or "closed" (landed).
type Convoy struct {
	ID        string
	Title     string
	Status    string
	Completed int
	Total     int
	CreatedAt time.Time
	Tracked   []ConvoyItem
}

// ConvoyItem is a single bead tracked by a convoy.
type ConvoyItem struct {
	ID       string
	Title    string
	Status   string
	Assignee string
	Blocked  bool
}

// MergeRequest is a pending merge in a rig's merge queue (from `gt mq list
// <rig>`). The merge queue lists pending merges by default, so a slice of these
// is the set of work awaiting the refinery.
type MergeRequest struct {
	ID          string
	Title       string
	Status      string
	SourceIssue string // the bead being merged (parsed from the MR description)
	Worker      string // the polecat that produced the branch
	Rig         string // the rig whose queue this MR sits in
	CreatedAt   time.Time
}

// parseConvoys parses `gt convoy list --json` output. The convoy JSON carries no
// closed/landed timestamp, so CreatedAt is the only time signal available.
func parseConvoys(b []byte) ([]Convoy, error) {
	var raw []struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		Status    string `json:"status"`
		Completed int    `json:"completed"`
		Total     int    `json:"total"`
		CreatedAt string `json:"created_at"`
		Tracked   []struct {
			ID       string `json:"id"`
			Title    string `json:"title"`
			Status   string `json:"status"`
			Assignee string `json:"assignee"`
			Blocked  bool   `json:"blocked"`
		} `json:"tracked"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	convoys := make([]Convoy, 0, len(raw))
	for _, r := range raw {
		ts, _ := time.Parse(time.RFC3339, r.CreatedAt)
		cv := Convoy{
			ID: r.ID, Title: r.Title, Status: r.Status,
			Completed: r.Completed, Total: r.Total, CreatedAt: ts,
		}
		for _, t := range r.Tracked {
			cv.Tracked = append(cv.Tracked, ConvoyItem{
				ID: t.ID, Title: t.Title, Status: t.Status,
				Assignee: t.Assignee, Blocked: t.Blocked,
			})
		}
		convoys = append(convoys, cv)
	}
	return convoys, nil
}

// parseMergeQueue parses `gt mq list <rig> --json` output, tagging each MR with
// its rig. An empty queue serializes as JSON "null", which unmarshals to an
// empty slice (not an error).
func parseMergeQueue(b []byte, rig string) ([]MergeRequest, error) {
	var raw []struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Status      string `json:"status"`
		Description string `json:"description"`
		CreatedAt   string `json:"created_at"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	mrs := make([]MergeRequest, 0, len(raw))
	for _, r := range raw {
		ts, _ := time.Parse(time.RFC3339, r.CreatedAt)
		mrs = append(mrs, MergeRequest{
			ID:          r.ID,
			Title:       r.Title,
			Status:      r.Status,
			SourceIssue: descField(r.Description, "source_issue"),
			Worker:      descField(r.Description, "worker"),
			Rig:         rig,
			CreatedAt:   ts,
		})
	}
	return mrs, nil
}

// descField extracts a "key: value" field from a merge-request description (a
// multi-line block of metadata). Returns "" when the key is absent.
func descField(desc, key string) string {
	for _, line := range strings.Split(desc, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), key+":"); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// recentConvoys keeps the cockpit-relevant convoys: all in-progress (open) ones,
// plus closed ones landed within the window. Ordered open-first, then
// newest-first by creation, so active work sits at the top of the panel.
func recentConvoys(all []Convoy, now time.Time, within time.Duration) []Convoy {
	out := make([]Convoy, 0, len(all))
	for _, c := range all {
		if c.Status == "closed" && now.Sub(c.CreatedAt) > within {
			continue
		}
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		oi, oj := out[i].Status == "open", out[j].Status == "open"
		if oi != oj {
			return oi // open before closed
		}
		return out[i].CreatedAt.After(out[j].CreatedAt) // newest first
	})
	return out
}

// rigNames extracts the unique rig identifiers from a routes prefix map. Routes
// key on path ("." for the town, "GasTownTower", "Lokust/mayor/rig"); the rig is
// the first path segment. The town route is excluded. `gt mq list` keys on this
// rig name.
func rigNames(prefixes map[string]string) []string {
	seen := map[string]struct{}{}
	var out []string
	for path := range prefixes {
		if path == "." || path == "" {
			continue
		}
		rig := path
		if i := strings.IndexByte(rig, '/'); i >= 0 {
			rig = rig[:i]
		}
		if _, ok := seen[rig]; ok {
			continue
		}
		seen[rig] = struct{}{}
		out = append(out, rig)
	}
	sort.Strings(out)
	return out
}

// loadConvoys shells out to gt for all convoys (open + closed); the caller
// filters to the recent window. Best-effort: errors mean "no convoys known".
func loadConvoys(townRoot string) ([]Convoy, error) {
	cmd := exec.Command("gt", "convoy", "list", "--all", "--json")
	cmd.Dir = townRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return parseConvoys(out)
}

// loadMergeQueue shells out to gt for each rig's pending merges and aggregates
// them. Best-effort per rig: a failing rig is skipped, not fatal.
func loadMergeQueue(townRoot string, rigs []string) ([]MergeRequest, error) {
	var all []MergeRequest
	for _, rig := range rigs {
		cmd := exec.Command("gt", "mq", "list", rig, "--json")
		cmd.Dir = townRoot
		out, err := cmd.Output()
		if err != nil {
			continue
		}
		mrs, err := parseMergeQueue(out, rig)
		if err != nil {
			continue
		}
		all = append(all, mrs...)
	}
	return all, nil
}
