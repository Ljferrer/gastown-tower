<script>
  // The expanded card body: per-session detail mirroring the TUI's expanded
  // row (model, context, turns/tools/reads/output, role/rig/idle, hook).
  import { human, short, idleLabel } from '../lib/format.js';

  let { agent } = $props();
  const s = $derived(agent.Stats ?? {});
</script>

<dl class="grid">
  <div><dt class="stamp">model</dt><dd class="mono">{short(s.Model)}</dd></div>
  <div><dt class="stamp">context</dt><dd class="mono">{human(s.ContextTokens ?? 0)} tok</dd></div>
  <div><dt class="stamp">output</dt><dd class="mono">{human(s.OutputTokens ?? 0)} tok</dd></div>
  <div><dt class="stamp">turns</dt><dd class="mono">{s.Turns ?? 0}</dd></div>
  <div><dt class="stamp">tools</dt><dd class="mono">{s.ToolCalls ?? 0}</dd></div>
  <div><dt class="stamp">reads</dt><dd class="mono">{s.FileReads ?? 0}</dd></div>
  <div><dt class="stamp">rig</dt><dd>{agent.Rig || '—'}</dd></div>
  <div><dt class="stamp">last active</dt><dd class="mono">{idleLabel(agent.Idle)} ago</dd></div>
</dl>

{#if agent.Hook}
  <p class="hook">
    <span class="stamp">hook</span>
    <span class="hookval mono">{agent.Hook}</span>
  </p>
{/if}

<style>
  .grid {
    margin: 0;
    padding: var(--space-3);
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(7rem, 1fr));
    gap: var(--space-3) var(--space-4);
    background: var(--bg-grain);
  }
  .grid div {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  dt {
    line-height: 1;
  }
  dd {
    margin: 0;
    font-size: var(--text-sm);
    color: var(--text);
  }
  .mono {
    font-family: var(--font-mono);
    font-variant-numeric: tabular-nums;
  }

  .hook {
    margin: 0;
    padding: var(--space-2) var(--space-3) var(--space-3);
    display: flex;
    flex-direction: column;
    gap: 2px;
    background: var(--bg-grain);
  }
  .hookval {
    font-size: var(--text-sm);
    color: var(--accent);
  }
</style>
