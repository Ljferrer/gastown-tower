package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/Ljferrer/gastown-tower/pkg/tower"
	tea "github.com/charmbracelet/bubbletea"
)

func fixtureSnapshot() tower.Snapshot {
	return tower.Snapshot{
		GeneratedAt: time.Date(2026, 5, 30, 15, 4, 5, 0, time.UTC),
		Groups: []tower.Group{
			{Name: "town", Agents: []tower.Agent{{
				AgentRef: tower.AgentRef{Name: "mayor", Role: "mayor", Group: "town"},
				Churning: true,
				Stats:    tower.TranscriptStats{Model: "claude-opus-4-8", ContextPct: 0.45, Turns: 10, ToolCalls: 5, FileReads: 1, NowDoing: "running go test"},
			}}},
			{Name: "GigaClip", Agents: []tower.Agent{{
				AgentRef: tower.AgentRef{Name: "witness", Role: "witness", Rig: "GigaClip", Group: "GigaClip"},
				Churning: false,
				Idle:     5 * time.Minute,
				Stats:    tower.TranscriptStats{Model: "claude-haiku-4-5", ContextPct: 0.2},
			}}},
		},
	}
}

func newWithSnap() Model {
	m := New(nil)
	m.snap = fixtureSnapshot()
	m.flatten()
	return m
}

func TestView(t *testing.T) {
	out := newWithSnap().View()
	for _, want := range []string{
		"GAS TOWN ACTIVITY TOWER", "2 active",
		"town", "mayor", "running go test",
		"GigaClip", "witness", "idle 5m",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("View() missing %q", want)
		}
	}
}

func TestNavigateAndExpand(t *testing.T) {
	m := newWithSnap()
	if len(m.agents) != 2 {
		t.Fatalf("flatten: got %d agents", len(m.agents))
	}
	// move cursor down to the witness
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(Model)
	if m.cursor != 1 {
		t.Fatalf("cursor=%d, want 1", m.cursor)
	}
	// expand it
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if !m.expanded["GigaClip/witness"] {
		t.Fatalf("witness should be expanded")
	}
	if !strings.Contains(m.View(), "role witness") {
		t.Errorf("expanded details not rendered")
	}
}

func TestQuit(t *testing.T) {
	m := newWithSnap()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("ctrl+c should return a quit command")
	}
}
