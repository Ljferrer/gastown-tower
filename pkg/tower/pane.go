package tower

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// workingMarker is Claude Code's interrupt hint, shown in a session's status
// line ONLY while the agent is actively generating a turn. Its presence is the
// liveness signal: unlike transcript mtime, it stays true through a long
// think/generate turn that writes nothing to disk until the turn completes.
const workingMarker = "esc to interrupt"

// paneWorking reports whether captured pane content shows the agent actively
// working (mid-turn). Pure over the captured text for straightforward testing.
func paneWorking(pane string) bool {
	return strings.Contains(pane, workingMarker)
}

// paneAwaitingRe matches Claude Code's selection-box prompt: the caret cursor on
// a numbered option, e.g. "❯ 1. Yes" — what's drawn while an AskUserQuestion /
// ExitPlanMode awaits the overseer's choice.
var paneAwaitingRe = regexp.MustCompile(`❯\s+\d+\.`)

// paneAwaiting reports whether captured pane content shows the agent paused on a
// structured prompt to its overseer (a numbered selection box). A working pane
// is never awaiting: an active turn takes precedence, so the working marker
// short-circuits to false.
func paneAwaiting(pane string) bool {
	if paneWorking(pane) {
		return false
	}
	return paneAwaitingRe.MatchString(pane)
}

// TurnProgress is the current-turn detail parsed from the spinner line: how long
// the active turn has been generating and how many output tokens have streamed.
type TurnProgress struct {
	Elapsed time.Duration // wall time the current turn has been generating
	Tokens  int           // streamed output tokens so far this turn
}

// spinnerRe matches Claude Code's spinner line, e.g.
//
//	"✳ Honking… (9s · ↓ 475 tokens)"
//	"· Flambéing… (44s · ↓ 2.0k tokens · thinking)"
//
// capturing the current-turn elapsed seconds, the token count, and a "k" suffix.
var spinnerRe = regexp.MustCompile(`\((\d+)s · ↓ ([\d.]+)(k?) tokens`)

// parseSpinner extracts current-turn elapsed + streamed tokens from pane content.
// ok=false when no spinner line is present (agent idle, or the format changed).
func parseSpinner(pane string) (TurnProgress, bool) {
	m := spinnerRe.FindStringSubmatch(pane)
	if m == nil {
		return TurnProgress{}, false
	}
	secs, _ := strconv.Atoi(m[1])
	val, _ := strconv.ParseFloat(m[2], 64)
	if m[3] == "k" {
		val *= 1000
	}
	return TurnProgress{Elapsed: time.Duration(secs) * time.Second, Tokens: int(val)}, true
}

// tmuxSession returns the expected tmux session name for an agent given the
// rig-path -> prefix map. Town agents key on "." (prefix "hq"); rig agents key
// on their rig. The leaf of a slashed name is used (deacon/boot -> hq-boot).
// ok=false when the agent's rig has no known prefix.
func tmuxSession(ref AgentRef, prefixes map[string]string) (string, bool) {
	key := ref.Rig
	if ref.Group == townGroup {
		key = "."
	}
	prefix, ok := prefixes[key]
	if !ok {
		return "", false
	}
	leaf := ref.Name
	if i := strings.LastIndex(leaf, "/"); i >= 0 {
		leaf = leaf[i+1:]
	}
	return prefix + "-" + leaf, true
}

// parseSessionPrefixes builds a rig-path -> session-prefix map from routes.jsonl
// content. Each line is {"prefix":"gtt-","path":"GasTownTower"}; the trailing
// "-" is stripped. When a path has several routes (the town has hq- and hq-cv-),
// the first wins so the town resolves to "hq", not the convoy prefix.
func parseSessionPrefixes(b []byte) map[string]string {
	prefixes := map[string]string{}
	scanner := bufio.NewScanner(bytes.NewReader(b))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var route struct {
			Prefix string `json:"prefix"`
			Path   string `json:"path"`
		}
		if err := json.Unmarshal([]byte(line), &route); err != nil {
			continue
		}
		if route.Path == "" {
			continue
		}
		if _, seen := prefixes[route.Path]; seen {
			continue // first route per path wins
		}
		prefixes[route.Path] = strings.TrimSuffix(route.Prefix, "-")
	}
	return prefixes
}

// loadSessionPrefixes reads <townRoot>/.beads/routes.jsonl. Best-effort: a
// missing or unreadable file yields an empty map, and agents fall back to the
// mtime heuristic for liveness.
func loadSessionPrefixes(townRoot string) (map[string]string, error) {
	b, err := os.ReadFile(filepath.Join(townRoot, ".beads", "routes.jsonl"))
	if err != nil {
		return nil, err
	}
	return parseSessionPrefixes(b), nil
}

// sanitizeSocketRe matches characters not allowed in a tmux socket name.
var sanitizeSocketRe = regexp.MustCompile(`[^a-z0-9-]+`)

// townSocketName derives the per-town tmux socket name exactly as gt does
// (session/registry.go): the sanitized directory basename, a hyphen, and 6 hex
// chars of the canonical path's SHA-256. The path hash keeps two towns that
// share a basename (~/gt and ~/work/gt) on distinct sockets. gt launches every
// agent session on this socket via `tmux -L <name>`; the tower must query the
// same one or it sees no panes and misreads working agents as idle.
func townSocketName(townRoot string) string {
	base := strings.Trim(sanitizeSocketRe.ReplaceAllString(strings.ToLower(filepath.Base(townRoot)), "-"), "-")
	if base == "" {
		base = "default"
	}
	canonical, err := filepath.EvalSymlinks(townRoot)
	if err != nil {
		if canonical, err = filepath.Abs(townRoot); err != nil {
			canonical = townRoot
		}
	}
	h := sha256.Sum256([]byte(canonical))
	return base + "-" + hex.EncodeToString(h[:3])
}

// gtSocketName returns the tmux socket gt uses for this town, honoring the
// GT_TMUX_SOCKET override: a concrete value is used verbatim, while unset /
// "default" / "auto" fall through to the per-town derivation.
func gtSocketName(townRoot string) string {
	switch socket := os.Getenv("GT_TMUX_SOCKET"); socket {
	case "", "default", "auto":
		return townSocketName(townRoot)
	default:
		return socket
	}
}

// tmuxArgs prepends "-L <socket>" to a tmux argument list when socket is set,
// targeting gt's named server. An empty socket yields the bare args so behavior
// degrades to the default socket rather than passing an invalid flag.
func tmuxArgs(socket string, args ...string) []string {
	if socket == "" {
		return args
	}
	return append([]string{"-L", socket}, args...)
}

// listTmuxSessions returns the set of live tmux session names on the given gt
// socket. Best-effort: when tmux is unavailable the caller treats every agent
// as having no pane.
func listTmuxSessions(socket string) (map[string]struct{}, error) {
	out, err := exec.Command("tmux", tmuxArgs(socket, "ls", "-F", "#{session_name}")...).Output()
	if err != nil {
		return nil, err
	}
	set := map[string]struct{}{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			set[line] = struct{}{}
		}
	}
	return set, nil
}

// capturePane returns the visible text of a tmux session's active pane on the
// given gt socket.
func capturePane(socket, session string) (string, error) {
	out, err := exec.Command("tmux", tmuxArgs(socket, "capture-pane", "-p", "-t", session)...).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
