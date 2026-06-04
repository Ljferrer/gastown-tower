# Churn / Activity Detection

The Activity Tower marks each agent as **churning** (actively working) or **idle**.
This document explains how that decision is made, why it changed, and which knobs
affect it. It reflects the implementation in
[`pkg/tower/collector.go`](../pkg/tower/collector.go) and
[`pkg/tower/pane.go`](../pkg/tower/pane.go).

## TL;DR

| Signal | Role | When it applies |
| --- | --- | --- |
| tmux pane working-indicator (`esc to interrupt`) | **Primary** | Whenever the agent's live pane can be resolved and captured |
| Transcript `.jsonl` mtime within `ChurnWindow` (8s) | **Fallback** | Only when no live pane is available |

An agent whose tmux pane shows the working-indicator is reported as churning
**regardless of transcript mtime**. The mtime window is consulted only when the
pane cannot be used.

## (a) The tmux pane is the primary churning signal

Claude Code renders an interrupt hint — the literal string `esc to interrupt` —
in a session's status line **only while the agent is actively generating a
turn**. The Tower treats the presence of that marker as the authoritative
liveness signal:

- `workingMarker = "esc to interrupt"` and `paneWorking(pane)` in
  [`pkg/tower/pane.go`](../pkg/tower/pane.go) test the captured pane text for the
  marker.
- `Collector.churnState` in [`pkg/tower/collector.go`](../pkg/tower/collector.go)
  resolves the agent's expected tmux session, confirms it is live, captures the
  pane, and returns `paneWorking(pane)` as the churn state.

Because the marker stays present for the entire turn — including long stretches
where nothing is written to disk — an agent mid-generation is correctly shown as
churning.

### How the pane is located

`tmuxSession` maps an agent to its tmux session name using the rig-path →
session-prefix table parsed from `<townRoot>/.beads/routes.jsonl`:

- **Town agents** key on `.` and resolve to `hq-<leaf>`.
- **Rig agents** resolve to `<rig-prefix>-<leaf>`.
- The leaf is the last path segment of the agent name (`deacon/boot` → `boot`).

The live tmux session set is fetched fresh on every snapshot (`listTmuxSessions`),
while the route-derived prefix table is TTL-cached with the rest of the town
enrichment (`EnrichTTL`, 15s default). Pane capture uses
`tmux capture-pane -p -t <session>`.

### Current-turn detail (bonus)

When the pane path is taken, `parseSpinner` also reads the spinner line
(e.g. `✳ Honking… (9s · ↓ 475 tokens)`) into a `TurnProgress` value — the
current turn's elapsed time and streamed output-token count. This detail is only
available on the pane path; the mtime fallback returns an empty `TurnProgress`.

## (b) Transcript mtime is the fallback

When no pane can be used, `churnState` falls back to the transcript-mtime
heuristic: the agent is churning if its newest `*.jsonl` transcript was modified
within `ChurnWindow`.

The fallback is taken whenever any of the following holds:

- **Unknown rig** — the agent's rig has no prefix in `routes.jsonl`, so no
  session name can be formed.
- **Dead session** — the resolved session name is not in the live tmux set.
- **Capture failure** — `tmux capture-pane` returns an error.
- **tmux missing** — `tmux ls` fails (tmux not installed/running), so the live
  session set is empty and every agent falls back.

The fallback is best-effort by design: a failure in any tmux probe degrades
gracefully to the mtime heuristic rather than dropping the agent.

## (c) The `ChurnWindow` constant only applies on the fallback path

`ChurnWindow` (default **8s**, set in `NewCollector` in
[`pkg/tower/collector.go`](../pkg/tower/collector.go)) is the staleness window
for the **mtime fallback only**:

```go
return now.Sub(mt) <= c.ChurnWindow, TurnProgress{}
```

On the primary pane path the churn decision comes entirely from the
working-indicator; `ChurnWindow` is never consulted there. Changing the constant
affects only sessions that have already fallen back to mtime.

> Note: a separate, larger window — `ActiveWindow` (default **30 min**) — governs
> whether a session is shown at all. Sessions idle longer than `ActiveWindow` are
> dropped from the snapshot entirely; that is distinct from the churning/idle dot.

## (d) Why the old pure-mtime approach produced false-idles

Before churn-detection v2 (commit `cce7d93`, bead `hq-j5r`), churn was inferred
purely from transcript mtime within an 8s window. A single agent turn can run far
longer than 8s without touching the transcript:

- **Model thinking / long generation** — the transcript `.jsonl` is not flushed
  until the turn completes, so mtime stays stale for the whole turn.
- **Long-running tools** — a turn blocked on a slow tool call writes nothing in
  the interim.

In both cases the mtime window elapsed mid-turn and the agent was falsely shown
as **idle** while it was, in fact, hard at work. The tmux working-indicator does
not have this blind spot: it remains present for the full turn, which is why it is
now the primary signal with mtime kept only as a fallback.

## Source of truth

- [`pkg/tower/collector.go`](../pkg/tower/collector.go) — `Collector.churnState`,
  `ChurnWindow`, `ActiveWindow`, `EnrichTTL` defaults.
- [`pkg/tower/pane.go`](../pkg/tower/pane.go) — `workingMarker`, `paneWorking`,
  `parseSpinner`, `tmuxSession`, `parseSessionPrefixes`, `listTmuxSessions`,
  `capturePane`.
