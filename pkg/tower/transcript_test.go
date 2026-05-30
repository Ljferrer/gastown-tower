package tower

import (
	"strings"
	"testing"
)

const fixture = `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hi"}]}}
{"type":"assistant","message":{"role":"assistant","model":"claude-sonnet-4-6","usage":{"input_tokens":10,"cache_read_input_tokens":100,"cache_creation_input_tokens":5,"output_tokens":50},"content":[{"type":"text","text":"on it"},{"type":"tool_use","name":"Bash","input":{"command":"go test ./...","description":"Run tests"}}]}}
{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"ok"}]}}
not valid json — should be skipped
{"type":"assistant","message":{"role":"assistant","model":"claude-sonnet-4-6","usage":{"input_tokens":20,"cache_read_input_tokens":200,"cache_creation_input_tokens":0,"output_tokens":30},"content":[{"type":"tool_use","name":"Read","input":{"file_path":"/Users/x/gastown-tower/pkg/tower/window.go"}}]}}
{"type":"system","message":{"role":"system","content":"ignored"}}`

func TestParseTranscript(t *testing.T) {
	s := parseTranscript(strings.NewReader(fixture))

	if s.Turns != 2 {
		t.Errorf("Turns=%d, want 2", s.Turns)
	}
	if s.ToolCalls != 2 {
		t.Errorf("ToolCalls=%d, want 2", s.ToolCalls)
	}
	if s.FileReads != 1 {
		t.Errorf("FileReads=%d, want 1", s.FileReads)
	}
	if s.OutputTokens != 80 {
		t.Errorf("OutputTokens=%d, want 80 (cumulative)", s.OutputTokens)
	}
	if s.ContextTokens != 220 { // latest usage: 20+200+0
		t.Errorf("ContextTokens=%d, want 220 (latest turn)", s.ContextTokens)
	}
	if s.Model != "claude-sonnet-4-6" {
		t.Errorf("Model=%q", s.Model)
	}
	if s.NowDoing != "read window.go" { // latest tool_use was a Read
		t.Errorf("NowDoing=%q, want %q", s.NowDoing, "read window.go")
	}
}

func TestHumanizeActivity(t *testing.T) {
	cases := []struct {
		name string
		tool *tBlock
		text string
		want string
	}{
		{"bash uses description", &tBlock{Type: "tool_use", Name: "Bash", Input: map[string]any{"command": "ls -la", "description": "List files"}}, "", "List files"},
		{"bash falls back to command first line", &tBlock{Type: "tool_use", Name: "Bash", Input: map[string]any{"command": "echo hi\necho bye"}}, "", "echo hi"},
		{"task shows description", &tBlock{Type: "tool_use", Name: "Task", Input: map[string]any{"description": "review code"}}, "", "agent: review code"},
		{"no tool falls back to text", nil, "Thinking about the plan", "Thinking about the plan"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := humanizeActivity(c.tool, c.text); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}
