// Command tower renders a Gas Town's live agent activity.
//
// MVP-1 subcommands:
//
//	tower snapshot   one-shot text snapshot of active agents (this file)
//
// Later: `tower tui` (Bubble Tea) and `tower serve` (HTTP+SSE + embedded web).
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Ljferrer/gastown-tower/pkg/tower"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "snapshot":
		runSnapshot(os.Args[2:])
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: tower snapshot [--town <dir>]")
	os.Exit(2)
}

func runSnapshot(args []string) {
	fs := flag.NewFlagSet("snapshot", flag.ExitOnError)
	town := fs.String("town", defaultTown(), "town root directory")
	_ = fs.Parse(args)

	snap, err := tower.NewCollector(*town).Snapshot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "snapshot:", err)
		os.Exit(1)
	}
	printSnapshot(snap)
}

func defaultTown() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "gt")
}

func printSnapshot(s tower.Snapshot) {
	fmt.Printf("\nGAS TOWN ACTIVITY TOWER — %s\n", s.GeneratedAt.Format("2006-01-02 15:04:05"))
	if len(s.Groups) == 0 {
		fmt.Println("  (no active agents)")
		return
	}
	for _, g := range s.Groups {
		fmt.Printf("\n▌ %s\n", g.Name)
		for _, a := range g.Agents {
			dot := "○"
			if a.Churning {
				dot = "●"
			}
			st := a.Stats
			activity := st.NowDoing
			if !a.Churning {
				activity = fmt.Sprintf("idle %s", roundDur(a.Idle))
			}
			fmt.Printf("  %s %-10s %s %3.0f%%  · %s · %d turns %d tools %d reads · %s\n",
				dot, a.Name, bar(st.ContextPct), st.ContextPct*100,
				short(st.Model), st.Turns, st.ToolCalls, st.FileReads, activity)
		}
	}
	fmt.Println()
}

// bar renders a 5-cell context-fill meter.
func bar(pct float64) string {
	const cells = 5
	filled := int(pct*cells + 0.5)
	if filled > cells {
		filled = cells
	}
	return strings.Repeat("▓", filled) + strings.Repeat("░", cells-filled)
}

func short(model string) string {
	model = strings.TrimPrefix(model, "claude-")
	if model == "" {
		return "?"
	}
	return model
}

func roundDur(d time.Duration) time.Duration {
	if d < time.Minute {
		return d.Round(time.Second)
	}
	return d.Round(time.Minute)
}
