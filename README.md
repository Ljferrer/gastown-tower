# Gas Town Tower

`tower` renders a Gas Town's live agent activity — who is working, on what, and
how busy the town is right now.

## Subcommands

```bash
tower snapshot   # one-shot text snapshot of active agents
tower tui        # interactive terminal UI (Bubble Tea)
tower serve      # HTTP + SSE server with an embedded web UI
```

`tower serve` binds to `127.0.0.1:8080` by default; it is loopback-only unless you
explicitly opt in with `--addr`.

## Activity / churn detection

Each agent is shown as **churning** (actively working) or **idle**. The primary
signal is the agent's live tmux pane working-indicator, with transcript file mtime
as a fallback for sessions whose pane can't be resolved. See
[docs/CHURN_DETECTION.md](docs/CHURN_DETECTION.md) for the full behavior, the
`ChurnWindow` fallback constant, and why the indicator replaced the old
pure-mtime heuristic.

## Layout

- `cmd/tower` — CLI entrypoint and subcommand wiring.
- `pkg/tower` — collector, transcript parsing, and churn/activity detection.
- `internal/tui` — Bubble Tea terminal UI.
- `internal/server` — HTTP + SSE server and embedded static assets.
- `web` — Svelte source for the embedded web UI.
