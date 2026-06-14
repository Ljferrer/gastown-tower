package tower

import "strings"

// AgentRef identifies an agent session discovered from the filesystem,
// before its transcript is parsed.
type AgentRef struct {
	Name  string // display label, e.g. "mayor", "jasper", "deacon/alpha"
	Role  string // mayor|deacon|dog|witness|refinery|polecat|crew|rig
	Rig   string // "" for town-level agents
	Group string // "town" or the rig name — the grouping/coloring key
	Addr  string // mail/bd assignee address, e.g. "GasTownTower/polecats/jasper"
}

// townGroup is the group key for town-level (HQ) infrastructure agents.
const townGroup = "town"

// classifyPath maps the path segments *below the town root* to an AgentRef.
// e.g. ["GigaClip","witness"] -> witness of rig GigaClip;
//
//	["deacon","dogs","alpha"] -> deacon dog "alpha" (town);
//	["GigaClip","polecats","jasper","GigaClip"] -> polecat "jasper" of GigaClip.
//
// Returns ok=false for paths that are not agent sessions (e.g. the bare town root).
func classifyPath(segments []string) (AgentRef, bool) {
	if len(segments) == 0 {
		return AgentRef{}, false // the town root itself, not an agent
	}
	switch head := segments[0]; head {
	case "mayor":
		return AgentRef{Name: "mayor", Role: "mayor", Group: townGroup, Addr: "mayor"}, true
	case "deacon":
		if len(segments) >= 3 && segments[1] == "dogs" {
			return AgentRef{Name: "deacon/" + segments[2], Role: "dog", Group: townGroup, Addr: "deacon/dogs/" + segments[2]}, true
		}
		return AgentRef{Name: "deacon", Role: "deacon", Group: townGroup, Addr: "deacon"}, true
	default:
		rig := head
		if len(segments) == 1 {
			return AgentRef{Name: rig, Role: "rig", Rig: rig, Group: rig}, true
		}
		switch sub := segments[1]; sub {
		case "witness":
			return AgentRef{Name: "witness", Role: "witness", Rig: rig, Group: rig, Addr: rig + "/witness"}, true
		case "refinery":
			return AgentRef{Name: "refinery", Role: "refinery", Rig: rig, Group: rig, Addr: rig + "/refinery"}, true
		case "polecats":
			name := "polecat"
			if len(segments) >= 3 {
				name = segments[2]
			}
			return AgentRef{Name: name, Role: "polecat", Rig: rig, Group: rig, Addr: rig + "/polecats/" + name}, true
		case "crew":
			name := "crew"
			if len(segments) >= 3 {
				name = segments[2]
			}
			return AgentRef{Name: name, Role: "crew", Rig: rig, Group: rig, Addr: rig + "/crew/" + name}, true
		case "seats":
			// Nun audit seats are spawned per panel under <rig>/seats/<Name>/<repo>,
			// mirroring the polecat worktree layout. The dir keeps the Nun's proper
			// name (e.g. "Mary"); tmuxSession lowercases it for the live session
			// (gtt-seat-mary). Without this case the seat falls to the "rig" default
			// below, whose session never resolves, so the liveness gate drops it and
			// the seat stays invisible in the AGENTS panel (gtt-urs).
			name := "seat"
			if len(segments) >= 3 {
				name = segments[2]
			}
			return AgentRef{Name: name, Role: "seat", Rig: rig, Group: rig, Addr: rig + "/seats/" + name}, true
		default:
			return AgentRef{Name: rig + "/" + sub, Role: "rig", Rig: rig, Group: rig}, true
		}
	}
}

// slugify mirrors Claude Code's project-dir naming: path separators and dots
// become hyphens. "/Users/x/gt" -> "-Users-x-gt".
func slugify(path string) string {
	return strings.NewReplacer("/", "-", ".", "-").Replace(path)
}

// segmentsBelow returns the path segments of a project-dir name that fall below
// the town-root slug, or ok=false if the dir is not under the town.
func segmentsBelow(dirName, townSlug string) ([]string, bool) {
	if dirName == townSlug {
		return nil, true // the town root session
	}
	rest, ok := strings.CutPrefix(dirName, townSlug+"-")
	if !ok {
		return nil, false
	}
	var segs []string
	for _, s := range strings.Split(rest, "-") {
		if s != "" {
			segs = append(segs, s)
		}
	}
	return segs, true
}
