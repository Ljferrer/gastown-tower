<script>
  import { onMount, onDestroy } from 'svelte';
  import { snapshot, connected, connect } from './lib/stream.js';
  import Header from './components/Header.svelte';
  import CharacterSheet from './components/CharacterSheet.svelte';
  import Group from './components/Group.svelte';
  import MailboxSlideOver from './components/MailboxSlideOver.svelte';
  import Wastelands from './components/Wastelands.svelte';

  let panel = $state(null); // null | 'mail' | 'wastelands'

  const groups = $derived($snapshot?.Groups ?? []);
  const town = $derived($snapshot?.Town ?? null);
  const activeCount = $derived(
    groups.reduce((n, g) => n + (g.Agents?.length ?? 0), 0)
  );

  let disconnect;
  onMount(() => {
    disconnect = connect();
  });
  onDestroy(() => disconnect?.());

  function closePanel() {
    panel = null;
  }
</script>

<svelte:window
  onkeydown={(e) => e.key === 'Escape' && closePanel()}
/>

<div class="tower">
  <Header
    generatedAt={$snapshot?.GeneratedAt}
    {activeCount}
    connected={$connected}
    mail={town?.Mail}
    onMail={() => (panel = 'mail')}
    onWastelands={() => (panel = 'wastelands')}
  />

  <CharacterSheet reputation={town?.Reputation} />

  <main class="board" aria-label="Active agents">
    {#if $snapshot == null}
      <p class="empty stamp">establishing feed…</p>
    {:else if groups.length === 0}
      <p class="empty stamp">no active agents</p>
    {:else}
      {#each groups as group (group.Name)}
        <Group {group} />
      {/each}
    {/if}
  </main>

  <footer class="footer">
    <span class="stamp">Gas Town Activity Tower</span>
    <span class="stamp">live feed · server-sent events</span>
  </footer>
</div>

<MailboxSlideOver
  open={panel === 'mail'}
  mail={town?.Mail}
  onClose={closePanel}
/>
<Wastelands
  open={panel === 'wastelands'}
  reputation={town?.Reputation}
  onClose={closePanel}
/>

<style>
  .tower {
    max-width: 1180px;
    margin: 0 auto;
    padding: var(--space-section) var(--space-4) var(--space-6);
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
  }

  .board {
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
  }

  .empty {
    padding: var(--space-6);
    text-align: center;
    border: 1px dashed var(--hairline);
    border-radius: var(--radius-lg);
  }

  .footer {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding-top: var(--space-4);
    border-top: 1px solid var(--hairline);
    opacity: 0.7;
  }

  @media (max-width: 640px) {
    .footer {
      flex-direction: column;
      gap: var(--space-2);
    }
  }
</style>
