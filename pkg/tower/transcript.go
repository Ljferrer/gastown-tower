package tower

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// TranscriptStats are the per-session metrics derived from a Claude Code
// transcript JSONL file (one agent session).
type TranscriptStats struct {
	Model         string // message.model of the latest assistant turn
	ContextTokens int    // latest prompt size: input + cache_read + cache_creation
	ContextPct    float64
	OutputTokens  int    // cumulative output tokens across the session
	ToolCalls     int    // total tool_use blocks
	FileReads     int    // tool_use blocks with name == "Read"
	Turns         int    // assistant messages
	NowDoing      string // humanized latest tool_use (or last assistant text)
	MidTurn       bool   // last conversation message awaits a reply (agent is busy)

	// Overseer-prompt signals derived from the transcript tail. PendingAsk is
	// true when the terminal tool_use is an AskUserQuestion or ExitPlanMode
	// awaiting its result; TrailingQuestion is true when the terminal turn is a
	// text-only assistant message ending in '?'. Both feed awaiting-overseer
	// detection.
	PendingAsk       bool
	TrailingQuestion bool
}

// scanLimit is the max single-line size we accept from a transcript. Attachment
// and file-history records can be large, so we read with a generous buffer.
const scanLimit = 64 << 20 // 64 MiB

type tRecord struct {
	Type    string    `json:"type"`
	Subtype string    `json:"subtype"`
	Message *tMessage `json:"message"`
}

type tMessage struct {
	Role    string          `json:"role"`
	Model   string          `json:"model"`
	Content json.RawMessage `json:"content"`
	Usage   *tUsage         `json:"usage"`
}

type tUsage struct {
	InputTokens              int `json:"input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	OutputTokens             int `json:"output_tokens"`
}

type tBlock struct {
	Type  string         `json:"type"`
	Text  string         `json:"text"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

// ParseTranscript reads a transcript JSONL file and derives session stats.
// Malformed lines are skipped (transcripts are append-only and may be mid-write).
func ParseTranscript(path string) (TranscriptStats, error) {
	f, err := os.Open(path)
	if err != nil {
		return TranscriptStats{}, err
	}
	defer f.Close()
	return parseTranscript(f), nil
}

func parseTranscript(r io.Reader) TranscriptStats {
	var s TranscriptStats
	var lastUsage *tUsage
	var lastTool *tBlock
	var lastText string

	br := bufio.NewReaderSize(r, 1<<20)
	var line []byte
	for {
		chunk, isPrefix, err := br.ReadLine()
		line = append(line, chunk...)
		if isPrefix && len(line) < scanLimit {
			continue // long line, keep accumulating
		}
		if len(line) > 0 {
			processLine(line, &s, &lastUsage, &lastTool, &lastText)
			line = line[:0]
		}
		if err != nil {
			break
		}
	}

	if lastUsage != nil {
		s.ContextTokens = lastUsage.InputTokens + lastUsage.CacheReadInputTokens + lastUsage.CacheCreationInputTokens
	}
	s.ContextPct = ContextPct(s.Model, s.ContextTokens)
	s.NowDoing = humanizeActivity(lastTool, lastText)
	// A trailing question is a terminal text-only assistant turn (not mid-turn)
	// whose last text block ends with '?'. When the turn is mid-turn (a pending
	// tool_use, or a user message awaiting reply), lastText belongs to an earlier
	// turn and must not be read as the agent's open question.
	s.TrailingQuestion = !s.MidTurn && strings.HasSuffix(strings.TrimSpace(lastText), "?")
	return s
}

func processLine(line []byte, s *TranscriptStats, lastUsage **tUsage, lastTool **tBlock, lastText *string) {
	var rec tRecord
	if err := json.Unmarshal(line, &rec); err != nil {
		return
	}
	// A compaction boundary discards the conversation that preceded it: every
	// prior usage record measures a context window that no longer exists. Drop
	// the running usage so ContextTokens reflects the post-compact window — the
	// next assistant turn's usage if one follows, or zero while the agent sits
	// idle right after /compact (never the stale pre-compact peak). gtt-1wj.
	if rec.Type == "system" && rec.Subtype == "compact_boundary" {
		*lastUsage = nil
		return
	}
	if rec.Message == nil {
		return
	}
	m := rec.Message
	// Track whether the latest conversation message leaves the agent mid-turn.
	// A user message (prompt or tool_result) always awaits an assistant reply.
	// An assistant message is mid-turn only when it dispatched a tool and awaits
	// the result — i.e. it carries a tool_use block (set in the block scan
	// below); a terminal text-only turn ends it. Metadata records (attachment,
	// mode, …) carry no message and never reach here.
	switch rec.Type {
	case "user":
		// A user message (prompt or tool_result) answers any pending ask and
		// leaves the agent awaiting an assistant reply.
		s.MidTurn = true
		s.PendingAsk = false
		return
	case "assistant":
		s.MidTurn = false    // set true below if this turn dispatched a tool
		s.PendingAsk = false // set true below if the terminal tool is an ask
	default:
		return
	}
	s.Turns++
	if m.Model != "" {
		s.Model = m.Model
	}
	if m.Usage != nil {
		*lastUsage = m.Usage
		s.OutputTokens += m.Usage.OutputTokens
	}
	var blocks []tBlock
	if json.Unmarshal(m.Content, &blocks) != nil {
		return // content may be a bare string; no tool blocks to count
	}
	for i := range blocks {
		b := blocks[i]
		switch b.Type {
		case "tool_use":
			s.ToolCalls++
			s.MidTurn = true // awaiting this tool's result
			// The terminal tool_use determines PendingAsk: an overseer prompt
			// (AskUserQuestion/ExitPlanMode) sets it, any other tool clears it,
			// so the last block in the latest assistant turn wins.
			s.PendingAsk = b.Name == "AskUserQuestion" || b.Name == "ExitPlanMode"
			if b.Name == "Read" {
				s.FileReads++
			}
			*lastTool = &blocks[i]
		case "text":
			if strings.TrimSpace(b.Text) != "" {
				*lastText = b.Text
			}
		}
	}
}

// humanizeActivity turns the latest tool_use into a short "now doing" line.
func humanizeActivity(tool *tBlock, lastText string) string {
	if tool == nil {
		return firstLine(lastText, 60)
	}
	in := tool.Input
	str := func(k string) string {
		if v, ok := in[k].(string); ok {
			return v
		}
		return ""
	}
	switch tool.Name {
	case "Bash":
		if d := str("description"); d != "" {
			return d
		}
		return firstLine(str("command"), 50)
	case "Read", "Edit", "Write", "NotebookEdit":
		if p := str("file_path"); p != "" {
			return strings.ToLower(tool.Name) + " " + filepath.Base(p)
		}
	case "Grep":
		return "grep " + firstLine(str("pattern"), 40)
	case "Glob":
		return "glob " + str("pattern")
	case "Task", "Agent":
		if d := str("description"); d != "" {
			return "agent: " + d
		}
		if t := str("subagent_type"); t != "" {
			return "agent: " + t
		}
	case "Skill":
		return "skill: " + str("skill")
	}
	return tool.Name
}

func firstLine(s string, max int) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > max {
		s = s[:max-1] + "…"
	}
	return s
}
