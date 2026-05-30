<script>
  import { clockLabel } from '../lib/format.js';

  let { generatedAt, activeCount, connected, mail, onMail, onWastelands } =
    $props();

  const unread = $derived(mail?.Unread ?? 0);
  const total = $derived(mail?.Total ?? 0);
</script>

<header class="head">
  <div class="brand">
    <span class="mark" aria-hidden="true">▌</span>
    <div class="titles">
      <h1>Gas Town <em>Activity Tower</em></h1>
      <p class="stamp sub">field operations dossier</p>
    </div>
  </div>

  <div class="meters">
    <span class="meter">
      <span class="stamp lbl">time</span>
      <span class="val mono">{clockLabel(generatedAt)}</span>
    </span>
    <span class="meter">
      <span class="stamp lbl">active</span>
      <span class="val mono">{activeCount}</span>
    </span>
    <span class="meter feed" class:live={connected}>
      <span class="dot" aria-hidden="true"></span>
      <span class="stamp lbl">{connected ? 'live' : 'stale'}</span>
    </span>

    <button
      class="mailbtn"
      class:has-unread={unread > 0}
      onclick={onMail}
      aria-label={`Mailbox: ${unread} unread of ${total}`}
    >
      <span class="env" aria-hidden="true">✉</span>
      <span class="count mono">{unread}/{total}</span>
    </button>

    <button class="wlbtn" onclick={onWastelands}>
      <span class="stamp">Wastelands</span>
    </button>
  </div>
</header>

<style>
  .head {
    display: flex;
    justify-content: space-between;
    align-items: flex-end;
    flex-wrap: wrap;
    gap: var(--space-4);
    padding-bottom: var(--space-4);
    border-bottom: 2px solid var(--hairline-strong);
  }

  .brand {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }
  .mark {
    color: var(--accent);
    font-size: 2.2rem;
    line-height: 0.7;
    text-shadow: 0 0 18px var(--accent-ghost);
  }
  .titles h1 {
    margin: 0;
    font-size: var(--text-xl);
    font-weight: 800;
    letter-spacing: -0.01em;
  }
  .titles h1 em {
    font-style: normal;
    color: var(--accent);
  }
  .sub {
    margin: 0.15rem 0 0;
  }

  .meters {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    flex-wrap: wrap;
  }
  .meter {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 1px;
  }
  .lbl {
    line-height: 1;
  }
  .val {
    font-size: var(--text-lg);
    font-weight: 600;
    color: var(--text);
  }
  .mono {
    font-family: var(--font-mono);
    font-variant-numeric: tabular-nums;
  }

  .feed {
    flex-direction: row;
    align-items: center;
    gap: var(--space-2);
  }
  .feed .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--text-faint);
    transition: background var(--dur) var(--ease);
  }
  .feed.live .dot {
    background: var(--churn);
    box-shadow: 0 0 0 4px var(--churn-ghost);
  }

  .mailbtn,
  .wlbtn {
    font: inherit;
    cursor: pointer;
    background: var(--surface);
    border: 1px solid var(--hairline);
    border-radius: var(--radius);
    color: var(--text-dim);
    transition:
      transform var(--dur-fast) var(--ease),
      border-color var(--dur-fast) var(--ease),
      color var(--dur-fast) var(--ease);
  }
  .mailbtn {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
  }
  .mailbtn .env {
    font-size: 1.05rem;
  }
  .mailbtn .count {
    font-size: var(--text-sm);
  }
  .mailbtn:hover,
  .wlbtn:hover {
    transform: translateY(-1px);
    border-color: var(--hairline-strong);
    color: var(--text);
  }
  .mailbtn.has-unread {
    border-color: var(--alert);
    color: var(--text);
  }
  .mailbtn.has-unread .env {
    color: var(--alert);
  }

  .wlbtn {
    padding: var(--space-2) var(--space-3);
  }
  .wlbtn:hover {
    border-color: var(--accent-dim);
  }

  @media (max-width: 640px) {
    .head {
      align-items: flex-start;
    }
    .meters {
      gap: var(--space-3);
    }
  }
</style>
