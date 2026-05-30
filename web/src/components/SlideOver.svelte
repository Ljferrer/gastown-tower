<script>
  // Reusable right-edge slide-over shell: dimmed backdrop + a panel that flies
  // in from the right. Both mailbox and Wastelands compose it so the open/close
  // motion, focus trap entry, and Escape handling live in one place.
  import { fly, fade } from 'svelte/transition';

  let { open, title, onClose, children } = $props();

  const reduce =
    typeof matchMedia === 'function' &&
    matchMedia('(prefers-reduced-motion: reduce)').matches;
  const dur = reduce ? 0 : 280;
</script>

{#if open}
  <button
    class="scrim"
    aria-label="Close"
    transition:fade={{ duration: dur }}
    onclick={onClose}
  ></button>

  <div
    class="panel"
    role="dialog"
    aria-modal="true"
    aria-label={title}
    transition:fly={{ x: 360, duration: dur }}
  >
    <header class="phead">
      <h2 class="stamp">{title}</h2>
      <button class="close" onclick={onClose} aria-label="Close">✕</button>
    </header>
    <div class="pbody">
      {@render children?.()}
    </div>
  </div>
{/if}

<style>
  .scrim {
    position: fixed;
    inset: 0;
    border: 0;
    padding: 0;
    cursor: pointer;
    background: oklch(8% 0.01 80 / 0.62);
    backdrop-filter: blur(2px);
    z-index: 40;
  }
  .panel {
    position: fixed;
    top: 0;
    right: 0;
    height: 100dvh;
    width: min(420px, 92vw);
    z-index: 50;
    display: flex;
    flex-direction: column;
    background: var(--surface);
    border-left: 1px solid var(--hairline-strong);
    box-shadow: var(--shadow-float);
  }
  .phead {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--space-4);
    border-bottom: 1px solid var(--hairline);
  }
  .phead h2 {
    margin: 0;
    font-size: var(--text-sm);
    color: var(--accent);
    letter-spacing: var(--tracking-stamp);
  }
  .close {
    background: none;
    border: 1px solid var(--hairline);
    border-radius: var(--radius);
    color: var(--text-dim);
    width: 28px;
    height: 28px;
    cursor: pointer;
    transition:
      color var(--dur-fast) var(--ease),
      border-color var(--dur-fast) var(--ease);
  }
  .close:hover {
    color: var(--text);
    border-color: var(--hairline-strong);
  }
  .pbody {
    flex: 1;
    overflow-y: auto;
    padding: var(--space-4);
  }
</style>
