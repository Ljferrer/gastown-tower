# Plan: GTT as the Town Cockpit

Status: **design of record** (agreed via design session 2026-06-13). Tracking
beads carry the implementation; this doc is the rationale and the shape.

## Vision

Gas Town Tower (`tower`) becomes the **single town cockpit** — a superset of
`gt feed`. Today GTT wins on per-agent task detail (model, ctx, turns, tools,
current-turn progress); `feed` wins on town-level flow (event stream, convoys,
merge queue). The plan folds feed's strengths into GTT so one pane shows both
*who is working now / how hard* **and** *what is happening across town*.

`gt feed` is retired once GTT reaches parity. **TUI-first**: the terminal UI is
the focus of this plan; `serve` (web) and `snapshot` get the same data via the
shared `pkg/tower` core but are not the priority.

## Layout — stacked panels with a focus model

Adopt feed's proven stacked layout, replacing today's single flat cursor view
with a panel/focus model (`tab` switches the focused panel, `j/k` scrolls within
it, `enter` expands an agent):

```
GAS TOWN TOWER   19:50  12 active   ✉2/5  🏅gold        ← header
▌ town / rigs — grouped agents, churn bar, status, expand   ← panel 1: AGENTS
─── CONVOYS  (in-progress · landed 24h) ───────────────     ← panel 2: CONVOYS+MQ
─── EVENTS   (curated flow, newest-first) ─────────────     ← panel 3: EVENTS
tab focus · j/k scroll · enter expand · / search · q quit
```

## Phases

Ordered by value-to-effort. Problems view is **deferred** — Gas Town already
self-heals stuck agents well, so it is low operator value here.

### Phase 1 — Event stream (the soul of feed)

- Source: tail `<townRoot>/.events.jsonl` on each poll. Single clean JSONL log:
  `{ts, source, type, actor, payload, visibility}`. No shell-out; mirrors how
  `transcript.go` already reads JSONL.
- Retention: in-memory ring of the last ~500 events with a 24h cutoff
  (the file grows unbounded in a busy town).
- Default filter — **curated flow**: hide low-signal `session_start` (today it is
  ~half of all events); show `sling / done / handoff / escalation_* / mail /
  spawn / unhook`. Key `t` toggles "show all" (faithful mode); `/` searches.
- Order: newest-first.

### Phase 2 — Convoys + Merge Queue

- Convoys: `gt convoy list --json` (JSON available). In-progress + recently
  landed (24h).
- MQ: `gt mq list` (text — parse). Pending merges count + detail.
- TTL-cached like the existing `enrich.go` town enrichment, so the fast
  transcript refresh never pays the shell-out cost.

### Phase 3 — Town-level stats

- Round out feed's town aggregates on top of what GTT already shows
  (`✉` mail, `🏅` reputation): active count, town-busy rollups, per-rig summaries.

## Three-state status dot

Today: 🟢 churning vs ⚪ idle. Add a third state:

- 🟢 **churning** — pane shows `esc to interrupt` (unchanged primary signal).
- 🟠 **awaiting overseer** — the agent is blocked on the human's answer/approval.
- ⚪ **idle** — neither.

### Detecting 🟠 (B + C)

An agent is *awaiting overseer* when any of:

1. **Structured pane prompt** — captured pane shows a permission prompt /
   selection box / `AskUserQuestion` UI (new `paneAwaiting(pane)` in `pane.go`;
   GTT already captures the pane in `churnState`, so this is cheap).
2. **Pending structured ask in transcript** — tail record is a `tool_use` of
   `AskUserQuestion` / `ExitPlanMode` with no following result.
3. **Plain-text question heuristic** — idle AND the last assistant text ends with
   `?`. (Lower precision; included per decision B.)

…subject to a **freshness gate** (decision C): only mark orange while the wait is
recent (within `ActiveWindow`); a long-abandoned prompt decays back to grey.

### Role gating (who can ever be orange)

Orange is **role-gated**. Eligible by default: **`mayor` + every `crew` member**
(any crew that exists is automatically overseer-facing). Other roles
(`polecat / witness / refinery / deacon / dog / rig`) are never orange unless
explicitly added. Note: most polecats run `bypass permissions on` and never
prompt, so this gate also avoids false oranges from auto-approved sessions.

## Config — `tower.toml` (committed)

A new committed `tower.toml` provides optional overrides; the role-based default
needs no config to start.

```toml
# Optional. Default eligibility = mayor + all crew.
# Use to add non-crew agents or exclude specific ones.
overseer_agents_extra   = []   # e.g. ["GasTownTower/polecats/special"]
overseer_agents_exclude = []   # e.g. ["GasTownTower/crew/Background"]
```

CLI `--overseer-agents` overrides ad-hoc.

## Architecture fit

- `pkg/tower` stays the **shared read-only core**; TUI / serve / snapshot reuse it.
- New: an events collector concern feeding `Snapshot`; `paneAwaiting()` in
  `pane.go`; `Churning bool` on `Agent` generalizes to a status enum
  (idle / churning / awaiting).
- `internal/tui/model.go` grows panel + focus state (replacing the single flat
  cursor).
- Keep loopback-only default for `serve`; the cockpit reads, it does not act.
