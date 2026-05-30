<script>
  // The reputation strip — a compact "character sheet" header reading the
  // town's federation standing (handle / tier / stamps / value-space breadth).
  // The full read-only board lives in the Wastelands slide-over.
  let { reputation } = $props();

  const has = $derived(!!reputation?.Tier || !!reputation?.Handle);
  const handle = $derived(reputation?.Handle || '—');
  const tier = $derived(reputation?.Tier || 'unranked');
  const stamps = $derived(reputation?.Stamps ?? 0);
  const clusters = $derived(reputation?.Clusters ?? 0);
</script>

{#if has}
  <section class="sheet" aria-label="Reputation">
    <div class="id">
      <span class="stamp lbl">handle</span>
      <span class="handle">{handle}</span>
    </div>
    <div class="divider" aria-hidden="true"></div>
    <dl class="fields">
      <div class="field">
        <dt class="stamp">tier</dt>
        <dd class="tier">{tier}</dd>
      </div>
      <div class="field">
        <dt class="stamp">stamps</dt>
        <dd class="num">{stamps}</dd>
      </div>
      <div class="field">
        <dt class="stamp">value-space</dt>
        <dd class="num">{clusters}</dd>
      </div>
    </dl>
  </section>
{/if}

<style>
  .sheet {
    display: flex;
    align-items: center;
    gap: var(--space-5);
    flex-wrap: wrap;
    padding: var(--space-3) var(--space-4);
    background:
      linear-gradient(
        90deg,
        var(--accent-ghost),
        transparent 40%
      ),
      var(--surface);
    border: 1px solid var(--hairline);
    border-left: 3px solid var(--accent);
    border-radius: var(--radius);
    box-shadow: var(--shadow-panel);
  }

  .id {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .lbl {
    line-height: 1;
  }
  .handle {
    font-family: var(--font-mono);
    font-size: var(--text-lg);
    font-weight: 600;
    color: var(--accent);
    letter-spacing: 0.01em;
  }

  .divider {
    width: 1px;
    align-self: stretch;
    background: var(--hairline);
  }

  .fields {
    display: flex;
    gap: var(--space-6);
    margin: 0;
  }
  .field {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .field dt {
    line-height: 1;
  }
  .field dd {
    margin: 0;
  }
  .tier {
    text-transform: capitalize;
    font-weight: 600;
    color: var(--text);
  }
  .num {
    font-family: var(--font-mono);
    font-variant-numeric: tabular-nums;
    font-size: var(--text-lg);
    font-weight: 600;
    color: var(--text);
  }

  @media (max-width: 560px) {
    .divider {
      display: none;
    }
    .fields {
      gap: var(--space-4);
    }
  }
</style>
