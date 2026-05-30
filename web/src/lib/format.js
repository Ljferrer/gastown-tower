// Formatting helpers, mirroring the humanization in pkg/tower (TUI/CLI) so the
// web reads identically: short model names, compact token counts, rounded idle
// durations. All pure functions — trivially testable, no side effects.

// human compacts a token count: 1_500_000 -> "1.5M", 90_912 -> "91k".
export function human(n) {
  if (n >= 1_000_000) return `${(n / 1e6).toFixed(1)}M`;
  if (n >= 1_000) return `${Math.round(n / 1e3)}k`;
  return `${n}`;
}

// short drops the "claude-" prefix from a model id.
export function short(model) {
  if (!model) return '?';
  return model.replace(/^claude-/, '');
}

// pctLabel renders a 0..1 fraction as a whole-percent label.
export function pctLabel(frac) {
  return `${Math.round((frac || 0) * 100)}%`;
}

// idleLabel humanizes a Go time.Duration (nanoseconds) the way roundDur does:
// seconds under a minute, then minutes, then hours.
export function idleLabel(nanos) {
  const sec = Math.round((nanos || 0) / 1e9);
  if (sec < 60) return `${sec}s`;
  const min = Math.round(sec / 60);
  if (min < 60) return `${min}m`;
  const hr = Math.floor(min / 60);
  const rem = min % 60;
  return rem ? `${hr}h ${rem}m` : `${hr}h`;
}

// clockLabel renders an RFC3339 timestamp as HH:MM:SS in local time.
export function clockLabel(rfc3339) {
  if (!rfc3339) return '';
  const d = new Date(rfc3339);
  if (Number.isNaN(d.getTime())) return '';
  return d.toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  });
}
