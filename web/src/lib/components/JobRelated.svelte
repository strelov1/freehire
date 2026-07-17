<script lang="ts">
  import { resolve } from '$app/paths';
  import type { JobCopy } from '$lib/api';
  import type { Job } from '$lib/types';
  import JobRow from './JobRow.svelte';

  // The two "more like this" surfaces for a job, tabbed: semantically similar jobs and,
  // for a collapsed mass-posted role, the other cities it is open in. `copies` is a small
  // preview (the full list lives at /jobs/{slug}/copies); `copiesTotal` is the whole
  // cluster size.
  let {
    similar,
    copies,
    copiesTotal,
    slug,
  }: { similar: Job[]; copies: JobCopy[]; copiesTotal: number; slug: string } = $props();

  const hasSimilar = $derived(similar.length > 0);
  const hasCopies = $derived(copiesTotal > 1);

  type Tab = 'similar' | 'copies';
  // Default to Similar when present, else Copies; a user click overrides via `selected`.
  let selected = $state<Tab | null>(null);
  const active = $derived(selected ?? (hasSimilar ? 'similar' : 'copies'));

  const preview = $derived(copies.slice(0, 10));

  const tabClass = (isActive: boolean) =>
    `-mb-px border-b-2 px-1 pb-2 text-sm font-medium transition-colors ${
      isActive
        ? 'border-brand text-foreground'
        : 'border-transparent text-muted-foreground hover:text-foreground'
    }`;
</script>

{#if hasSimilar || hasCopies}
  <section class="mt-10">
    <div class="mb-4 flex gap-5 border-b border-border">
      {#if hasSimilar}
        <button type="button" class={tabClass(active === 'similar')} onclick={() => (selected = 'similar')}>
          Similar jobs
        </button>
      {/if}
      {#if hasCopies}
        <button type="button" class={tabClass(active === 'copies')} onclick={() => (selected = 'copies')}>
          Other locations ({copiesTotal})
        </button>
      {/if}
    </div>

    {#if active === 'similar' && hasSimilar}
      <div class="flex flex-col gap-3">
        {#each similar as job (job.public_slug)}
          <JobRow {job} />
        {/each}
      </div>
    {:else if active === 'copies' && hasCopies}
      <ul class="divide-y divide-border overflow-hidden rounded-lg border border-border">
        {#each preview as copy (copy.public_slug)}
          <li>
            <a
              href={resolve('/jobs/[slug]', { slug: copy.public_slug })}
              class="flex items-center justify-between px-4 py-2.5 text-sm hover:bg-accent"
            >
              <span class="text-foreground">{copy.location || 'Location not specified'}</span>
              <span class="text-xs text-muted-foreground">View →</span>
            </a>
          </li>
        {/each}
      </ul>
      {#if copiesTotal > preview.length}
        <a
          href={resolve('/jobs/[slug]/copies', { slug })}
          class="mt-3 inline-block text-sm font-medium text-brand-strong hover:underline"
        >
          View all {copiesTotal} locations →
        </a>
      {/if}
    {/if}
  </section>
{/if}
