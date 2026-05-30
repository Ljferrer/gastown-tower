// Live snapshot feed. Subscribes to the server's SSE stream (/api/stream),
// which pushes a full tower.Snapshot whenever its content changes. Falls back
// to a one-shot /api/snapshot fetch so the UI has data before the first event.
//
// Exposes plain Svelte stores so any component can react without prop drilling.
// EventSource reconnects on its own after a transient drop; we surface the
// connection state so the header can show a live/stale indicator.

import { writable } from 'svelte/store';

export const snapshot = writable(null);
export const connected = writable(false);

let source = null;

export function connect() {
  // Seed immediately so the first paint isn't empty while SSE handshakes.
  fetch('/api/snapshot')
    .then((r) => (r.ok ? r.json() : null))
    .then((s) => s && snapshot.update((cur) => cur ?? s))
    .catch(() => {});

  source = new EventSource('/api/stream');

  source.onopen = () => connected.set(true);

  source.onmessage = (event) => {
    try {
      snapshot.set(JSON.parse(event.data));
      connected.set(true);
    } catch {
      // Skip a malformed frame; the next event replaces it wholesale.
    }
  };

  source.onerror = () => {
    // EventSource auto-reconnects; reflect the gap meanwhile.
    connected.set(false);
  };

  return disconnect;
}

export function disconnect() {
  source?.close();
  source = null;
  connected.set(false);
}
