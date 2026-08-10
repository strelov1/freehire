<script lang="ts">
  import { untrack } from 'svelte';
  import { Card, Skeleton } from 'freehire-design-system';
  import { HIRE_ORIGIN } from '../../lib/auth';
  import { companyLogoUrl } from '../../lib/freehire';
  import { loadJob } from '../../lib/assistant/jobCache';
  import type { DeckEntry } from '../../lib/assistant/deck';

  // One card in a recommendation deck: the vacancy's own facts, fetched by slug so
  // they are never model-authored, plus the rationale the model attached to it.
  //
  // The slug was already resolved against the catalogue by `present_jobs`, so a
  // failed fetch here means the network — not an invented slug. The card still
  // shows the rationale in that case: it is the part the user came for.
  let { entry }: { entry: DeckEntry } = $props();

  // Captured once, not re-read in the template. The chat reassigns its state on
  // every streamed token, so a `loadJob(entry.slug)` inside `{#await}` would re-run
  // per token — and the cache evicts a rejected promise so a later render can
  // retry, which would turn one failing card into a request per token. The deck
  // keys its cards by slug, so this instance's slug never changes.
  // `untrack` states the intent the compiler otherwise warns about: this reads the
  // slug once, on purpose. Re-reading it reactively is the bug being avoided.
  const jobRequest = untrack(() => loadJob(entry.slug));

  let jobUrl = $derived(`${HIRE_ORIGIN}/jobs/${encodeURIComponent(entry.slug)}`);
</script>

<Card class="card">
  {#await jobRequest}
    <Skeleton class="skeleton" />
  {:then job}
    {@const logo = companyLogoUrl(job.company)}
    <a class="head" href={jobUrl} target="_blank" rel="noreferrer">
      {#if logo}
        <img class="logo" src={logo} alt="" />
      {/if}
      <span class="meta">
        {#if job.company}<span class="company">{job.company}</span>{/if}
        <span class="title">{job.title}</span>
        {#if job.location}<span class="where">{job.location}</span>{/if}
      </span>
    </a>
  {:catch}
    <!-- The facts did not load; the slug is still good, so link out by it. -->
    <a class="head" href={jobUrl} target="_blank" rel="noreferrer">
      <span class="meta"><span class="title">{entry.slug}</span></span>
    </a>
  {/await}

  {#if entry.note}
    <p class="note">{entry.note}</p>
  {/if}
  {#if entry.whyFits.length > 0}
    <ul class="pills">
      {#each entry.whyFits as why, i (i)}
        <li class="pill fit">{why}</li>
      {/each}
    </ul>
  {/if}
  {#if entry.concerns.length > 0}
    <ul class="pills">
      {#each entry.concerns as concern, i (i)}
        <li class="pill concern">{concern}</li>
      {/each}
    </ul>
  {/if}
</Card>

<style>
  :global(.card) {
    padding: 10px 12px;
  }

  .head {
    display: flex;
    align-items: center;
    gap: 8px;
    text-decoration: none;
    color: inherit;
  }

  .logo {
    flex: none;
    width: 28px;
    height: 28px;
    border-radius: 6px;
    border: 1px solid var(--border);
    object-fit: contain;
    background: var(--muted);
  }

  .meta {
    min-width: 0;
    display: flex;
    flex-direction: column;
  }

  .company,
  .where {
    font-size: 11px;
    color: var(--muted-foreground);
  }

  .title {
    font-size: 14px;
    font-weight: 650;
    line-height: 1.25;
  }

  .note {
    margin: 8px 0 0;
    font-size: 12px;
    line-height: 1.45;
  }

  .pills {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
    margin: 6px 0 0;
    padding: 0;
    list-style: none;
  }

  .pill {
    font-size: 11px;
    border-radius: 999px;
    padding: 2px 8px;
  }

  .fit {
    background: var(--brand-muted);
    color: var(--brand-strong);
  }

  .concern {
    background: var(--warning-muted);
    color: var(--warning-strong);
  }

  :global(.skeleton) {
    height: 34px;
  }
</style>
