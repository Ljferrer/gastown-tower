package tower

import (
	"encoding/json"
	"os/exec"
	"regexp"
	"strconv"
)

// Reputation is the town/rig federation standing (from `gt wl charsheet`).
type Reputation struct {
	Handle   string
	Tier     string
	Stamps   int
	Clusters int
}

// Mail is the mailbox summary (from `gt mail inbox`).
type Mail struct {
	Total  int
	Unread int
}

// TownStatus is town-level info that changes slowly relative to agent churn.
type TownStatus struct {
	Reputation Reputation
	Mail       Mail
}

// parseCharsheet parses `gt wl charsheet --json` output.
func parseCharsheet(b []byte) (Reputation, error) {
	var raw struct {
		Handle         string `json:"Handle"`
		Tier           string `json:"Tier"`
		StampCount     int    `json:"StampCount"`
		ClusterBreadth int    `json:"ClusterBreadth"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return Reputation{}, err
	}
	return Reputation{Handle: raw.Handle, Tier: raw.Tier, Stamps: raw.StampCount, Clusters: raw.ClusterBreadth}, nil
}

var mailRe = regexp.MustCompile(`\((\d+) messages?, (\d+) unread\)`)

// parseMailInbox extracts counts from the `gt mail inbox` header line, e.g.
// "📬 Inbox: mayor/ (5 messages, 0 unread)".
func parseMailInbox(s string) Mail {
	m := mailRe.FindStringSubmatch(s)
	if m == nil {
		return Mail{}
	}
	total, _ := strconv.Atoi(m[1])
	unread, _ := strconv.Atoi(m[2])
	return Mail{Total: total, Unread: unread}
}

// loadReputation shells out to gt for the town's character sheet. Best-effort:
// the caller treats errors as "reputation unavailable".
func loadReputation(townRoot string) (Reputation, error) {
	cmd := exec.Command("gt", "wl", "charsheet", "--json")
	cmd.Dir = townRoot
	out, err := cmd.Output() // stdout only; deprecation warnings go to stderr
	if err != nil {
		return Reputation{}, err
	}
	return parseCharsheet(out)
}

// loadMail shells out to gt for the mailbox summary. Best-effort.
func loadMail(townRoot string) (Mail, error) {
	cmd := exec.Command("gt", "mail", "inbox")
	cmd.Dir = townRoot
	out, err := cmd.Output()
	if err != nil {
		return Mail{}, err
	}
	return parseMailInbox(string(out)), nil
}

// parseHooks builds an assignee -> hook-title map from `bd list --status=hooked
// --json` output. Beads with no assignee are skipped.
func parseHooks(b []byte) (map[string]string, error) {
	var raw []struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Assignee string `json:"assignee"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	hooks := make(map[string]string, len(raw))
	for _, r := range raw {
		if r.Assignee == "" {
			continue
		}
		hooks[r.Assignee] = r.ID + ": " + r.Title
	}
	return hooks, nil
}

// loadHooks shells out to bd for currently-hooked beads, keyed by assignee.
// Best-effort: the caller treats errors as "no hooks known".
func loadHooks(townRoot string) (map[string]string, error) {
	cmd := exec.Command("bd", "list", "--status=hooked", "--json")
	cmd.Dir = townRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return parseHooks(out)
}
