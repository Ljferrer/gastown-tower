<script>
  // Wastelands — the read-only federation board. It reads the town's standing
  // from the snapshot's reputation (handle / tier / stamps / value-space) and
  // presents it as a posted dossier. Read-only by design: the Tower observes,
  // it does not mutate town state.
  import SlideOver from './SlideOver.svelte';

  let { open, reputation, onClose } = $props();

  const handle = $derived(reputation?.Handle || '—');
  const tier = $derived(reputation?.Tier || 'unranked');
  const stamps = $derived(reputation?.Stamps ?? 0);
  const clusters = $derived(reputation?.Clusters ?? 0);
</script>

<SlideOver {open} title="Wastelands · Board" {onClose}>
  <div class="poster">
    <p class="stamp kicker">federation standing</p>
    <p class="handle mono">{handle}</p>
    <p class="tier">{tier}</p>
  </div>

  <dl class="rows">
    <div class="row">
      <dt class="stamp">stamps earned</dt>
      <dd class="mono">{stamps}</dd>
    </div>
    <div class="row">
      <dt class="stamp">value-space breadth</dt>
      <dd class="mono">{clusters}</dd>
    </div>
  </dl>

  <p class="ledger stamp">
    read-only · the board reflects the ledger, it does not write to it
  </p>
</SlideOver>

<style>
  .poster {
    padding: var(--space-5) var(--space-4);
    text-align: center;
    background:
      radial-gradient(120% 80% at 50% 0%, var(--accent-ghost), transparent 65%),
      var(--surface-2);
    border: 1px solid var(--hairline);
    border-top: 3px solid var(--accent);
    border-radius: var(--radius);
  }
  .kicker {
    margin: 0 0 var(--space-2);
  }
  .handle {
    margin: 0;
    font-size: 1.5rem;
    font-weight: 700;
    color: var(--accent);
    word-break: break-word;
  }
  .tier {
    margin: var(--space-2) 0 0;
    text-transform: capitalize;
    color: var(--text-dim);
    letter-spacing: 0.04em;
  }

  .rows {
    margin: var(--space-4) 0 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  .row {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    padding: var(--space-3);
    background: var(--surface-2);
    border: 1px solid var(--hairline);
    border-radius: var(--radius);
  }
  .row dt {
    line-height: 1.2;
  }
  .row dd {
    margin: 0;
    font-size: var(--text-lg);
    font-weight: 600;
    color: var(--text);
  }
  .mono {
    font-family: var(--font-mono);
    font-variant-numeric: tabular-nums;
  }
  .ledger {
    margin-top: var(--space-5);
    opacity: 0.7;
    line-height: 1.5;
  }
</style>
