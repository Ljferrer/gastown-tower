<script>
  import { fly, fade, slide } from 'svelte/transition';
  import AgentDetail from './AgentDetail.svelte';
  import { pctLabel, idleLabel } from '../lib/format.js';

  let { agent, color } = $props();

  let expanded = $state(false);

  const reduce =
    typeof matchMedia === 'function' &&
    matchMedia('(prefers-reduced-motion: reduce)').matches;
  const dur = reduce ? 0 : 220;

  const pct = $derived(agent.Stats?.ContextPct ?? 0);
  const churning = $derived(!!agent.Churning);
  const activity = $derived(
    churning
      ? agent.Stats?.NowDoing || 'working'
      : `idle ${idleLabel(agent.Idle)}`
  );
  const hooked = $derived(!!agent.Hook);
</script>

<article
  class="card"
  class:churning
  style:--group={color}
  in:fly={{ y: 10, duration: dur }}
  out:fade={{ duration: dur }}
>
  <button
    class="row"
    aria-expanded={expanded}
    onclick={() => (expanded = !expanded)}
  >
    <span class="dot" class:live={churning} aria-hidden="true"></span>

    <span class="name">
      {agent.Name}
      {#if hooked}<span class="hook" title={agent.Hook} aria-label="hooked">🪝</span>{/if}
    </span>

    <span class="role stamp">{agent.Role}</span>

    <span class="ctx">
      <span class="track" aria-hidden="true">
        <span class="fill" style:transform={`scaleX(${pct})`}></span>
      </span>
      <span class="pct mono">{pctLabel(pct)}</span>
    </span>

    <span class="doing" class:idle={!churning}>{activity}</span>
  </button>

  {#if expanded}
    <div class="detail" transition:slide={{ duration: dur }}>
      <AgentDetail {agent} />
    </div>
  {/if}
</article>

<style>
  .card {
    background: var(--surface);
    border: 1px solid var(--hairline);
    border-left: 3px solid color-mix(in oklch, var(--group), transparent 35%);
    border-radius: var(--radius);
    box-shadow: var(--shadow-panel);
    overflow: hidden;
    transition: border-color var(--dur-fast) var(--ease);
  }
  .card.churning {
    border-left-color: var(--group);
  }
  .card:hover {
    border-color: var(--hairline-strong);
  }

  .row {
    width: 100%;
    display: grid;
    grid-template-columns: auto 1fr auto;
    grid-template-areas:
      'dot name role'
      'dot ctx ctx'
      'dot doing doing';
    align-items: center;
    gap: var(--space-1) var(--space-3);
    padding: var(--space-3);
    background: none;
    border: 0;
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
  }

  .dot {
    grid-area: dot;
    align-self: start;
    margin-top: 4px;
    width: 10px;
    height: 10px;
    border-radius: 50%;
    border: 1.5px solid var(--text-faint);
    background: transparent;
  }
  .dot.live {
    border-color: var(--churn);
    background: var(--churn);
    box-shadow: 0 0 0 0 var(--churn-ghost);
    animation: pulse 1.6s var(--ease) infinite;
  }
  @keyframes pulse {
    0% {
      box-shadow: 0 0 0 0 var(--churn-ghost);
    }
    70% {
      box-shadow: 0 0 0 7px transparent;
    }
    100% {
      box-shadow: 0 0 0 0 transparent;
    }
  }

  .name {
    grid-area: name;
    font-weight: 600;
    font-size: var(--text-base);
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }
  .hook {
    font-size: 0.85em;
    filter: saturate(0.8);
  }

  .role {
    grid-area: role;
    justify-self: end;
  }

  .ctx {
    grid-area: ctx;
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }
  .track {
    flex: 1;
    height: 6px;
    background: var(--surface-3);
    border-radius: 99px;
    overflow: hidden;
  }
  .fill {
    display: block;
    height: 100%;
    width: 100%;
    transform-origin: left center;
    background: var(--group);
    border-radius: 99px;
    transition: transform var(--dur-slow) var(--ease);
  }
  .pct {
    font-size: var(--text-xs);
    color: var(--text-dim);
    min-width: 2.6em;
    text-align: right;
  }

  .doing {
    grid-area: doing;
    font-size: var(--text-sm);
    color: var(--text-dim);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .doing.idle {
    color: var(--text-faint);
    font-style: italic;
  }

  .detail {
    border-top: 1px solid var(--hairline);
  }
</style>
