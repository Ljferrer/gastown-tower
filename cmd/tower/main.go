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
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Ljferrer/gastown-tower/internal/server"
	"github.com/Ljferrer/gastown-tower/internal/tui"
	"github.com/Ljferrer/gastown-tower/pkg/tower"
	tea "github.com/charmbracelet/bubbletea"
)

// defaultAddr binds to loopback only — the Tower exposes local activity and
// must not be reachable off-box without an explicit opt-in via --addr.
const defaultAddr = "127.0.0.1:8080"

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "snapshot":
		runSnapshot(os.Args[2:])
	case "tui":
		runTUI(os.Args[2:])
	case "serve":
		runServe(os.Args[2:])
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: tower <snapshot|tui|serve> [--town <dir>] [--addr <host:port>]")
	os.Exit(2)
}

func runTUI(args []string) {
	fs := flag.NewFlagSet("tui", flag.ExitOnError)
	town := fs.String("town", defaultTown(), "town root directory")
	_ = fs.Parse(args)

	c := tower.NewCollector(*town)
	p := tea.NewProgram(tui.New(c), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "tui:", err)
		os.Exit(1)
	}
}

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	town := fs.String("town", defaultTown(), "town root directory")
	addr := fs.String("addr", defaultAddr, "listen address (host:port)")
	_ = fs.Parse(args)

	srv := server.New(tower.NewCollector(*town))
	fmt.Fprintf(os.Stderr, "tower serve: listening on http://%s\n", *addr)
	if err := http.ListenAndServe(*addr, srv.Handler()); err != nil {
		fmt.Fprintln(os.Stderr, "serve:", err)
		os.Exit(1)
	}
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
	fmt.Printf("  %d active · %d busy\n", s.Stats.Active, s.Stats.Churning)
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
			} else if a.Turn.Elapsed > 0 {
				activity = fmt.Sprintf("%s %s", roundDur(a.Turn.Elapsed), st.NowDoing)
			}
			fmt.Printf("  %s %-10s %s %3.0f%%  · %s · %d turns %d tools %d reads · %s\n",
				dot, a.Name, bar(st.ContextPct), st.ContextPct*100,
				short(st.Model), st.Turns, st.ToolCalls, st.FileReads, activity)
		}
	}
	printConvoys(s)
	fmt.Println()
}

// printConvoys renders the convoys + merge-queue section of a text snapshot,
// mirroring the TUI's CONVOYS panel.
func printConvoys(s tower.Snapshot) {
	if len(s.Convoys) == 0 && len(s.MergeQueue) == 0 {
		return
	}
	fmt.Printf("\n▌ CONVOYS  (in-progress · landed 24h)\n")
	for _, cv := range s.Convoys {
		glyph := "○"
		switch {
		case cv.Status == "closed":
			glyph = "✓"
		case cv.Completed > 0:
			glyph = "◐"
		}
		fmt.Printf("  %s %d/%d  %s\n", glyph, cv.Completed, cv.Total, strings.TrimPrefix(cv.Title, "Work: "))
	}
	fmt.Printf("  merge queue · %d pending\n", len(s.MergeQueue))
	for _, mr := range s.MergeQueue {
		id := mr.SourceIssue
		if id == "" {
			id = mr.ID
		}
		fmt.Printf("    ↳ %s  %s %s %s\n", id, mr.Rig, mr.Worker, mr.Status)
	}
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
