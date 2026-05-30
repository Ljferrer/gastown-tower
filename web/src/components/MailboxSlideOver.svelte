<script>
  // Mailbox slide-over. The snapshot exposes mailbox counts (the live backend
  // surfaces totals, not message bodies), so this reads the box as a dossier
  // of states: unread on top, then read, then the running total. It is built
  // to slot richer per-message rows in later without changing the shell.
  import SlideOver from './SlideOver.svelte';

  let { open, mail, onClose } = $props();

  const total = $derived(mail?.Total ?? 0);
  const unread = $derived(mail?.Unread ?? 0);
  const read = $derived(Math.max(0, total - unread));
</script>

<SlideOver {open} title="Mailbox" {onClose}>
  {#if total === 0}
    <p class="empty stamp">inbox clear</p>
  {:else}
    <ul class="list">
      <li class="item" class:hot={unread > 0}>
        <span class="env" aria-hidden="true">✉</span>
        <span class="lbl">Unread</span>
        <span class="num mono">{unread}</span>
      </li>
      <li class="item">
        <span class="env muted" aria-hidden="true">✉</span>
        <span class="lbl">Read</span>
        <span class="num mono">{read}</span>
      </li>
    </ul>

    <div class="total">
      <span class="stamp">total in box</span>
      <span class="big mono">{total}</span>
    </div>
  {/if}

  <p class="note stamp">message bodies stream from the town mail log</p>
</SlideOver>

<style>
  .empty {
    padding: var(--space-6) 0;
    text-align: center;
  }
  .list {
    list-style: none;
    margin: 0 0 var(--space-4);
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  .item {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-3);
    background: var(--surface-2);
    border: 1px solid var(--hairline);
    border-radius: var(--radius);
  }
  .item.hot {
    border-color: var(--alert);
  }
  .env {
    color: var(--text-dim);
  }
  .item.hot .env {
    color: var(--alert);
  }
  .env.muted {
    opacity: 0.5;
  }
  .lbl {
    flex: 1;
  }
  .num {
    font-size: var(--text-lg);
    font-weight: 600;
    font-variant-numeric: tabular-nums;
  }

  .total {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    padding: var(--space-3);
    border-top: 1px solid var(--hairline);
  }
  .big {
    font-size: 1.6rem;
    font-weight: 700;
    color: var(--accent);
  }
  .mono {
    font-family: var(--font-mono);
  }
  .note {
    margin-top: var(--space-5);
    opacity: 0.7;
    line-height: 1.5;
  }
</style>
