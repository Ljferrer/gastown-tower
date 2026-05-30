<script>
  import { groupColor } from '../lib/color.js';
  import AgentCard from './AgentCard.svelte';

  let { group } = $props();

  const color = $derived(groupColor(group.Name));
  const agents = $derived(group.Agents ?? []);
</script>

<section class="group" style:--group={color} aria-label={`Group ${group.Name}`}>
  <header class="ghead">
    <span class="bar" aria-hidden="true"></span>
    <h2>{group.Name}</h2>
    <span class="count stamp">{agents.length} agent{agents.length === 1 ? '' : 's'}</span>
  </header>

  <div class="cards">
    {#each agents as agent (agent.Group + '/' + agent.Name)}
      <AgentCard {agent} {color} />
    {/each}
  </div>
</section>

<style>
  .group {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .ghead {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }
  .bar {
    width: 10px;
    height: 1.1rem;
    background: var(--group);
    border-radius: 2px;
    box-shadow: 0 0 12px color-mix(in oklch, var(--group), transparent 55%);
  }
  .ghead h2 {
    margin: 0;
    font-size: var(--text-lg);
    font-weight: 700;
    color: var(--group);
    letter-spacing: 0.01em;
  }
  .count {
    margin-left: auto;
  }

  .cards {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(min(100%, 320px), 1fr));
    gap: var(--space-3);
  }
</style>
