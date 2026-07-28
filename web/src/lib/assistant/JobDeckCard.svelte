<script lang="ts">
  import { resolve } from '$app/paths';
  import { TriangleAlert } from '@lucide/svelte';
  import JobRow from '$lib/components/JobRow.svelte';
  import JobCardSkeleton from './JobCardSkeleton.svelte';
  import { Badge } from '$lib/ui';
  import { loadJob } from './jobCache';
  import type { DeckEntry } from './deck';

  // One card in a recommendation deck: the vacancy's own facts, fetched by slug
  // so they are never model-authored, plus the rationale the model attached to
  // it. The rationale renders inside the card's border via `JobRow`'s footer, so
  // a recommendation is one block rather than a card followed by a loose
  // paragraph.
  //
  // The slug has already been resolved against the catalogue by `present_jobs`,
  // so a failed fetch here means the network — not an invented slug. The card
  // still shows the rationale in that case: it is the part the user came for.
  let { entry }: { entry: DeckEntry } = $props();

  // Captured once, not re-read in the template. The chat reassigns its state on
  // every streamed token, so a `loadJob(entry.slug)` call inside `{#await}` would
  // re-run per token — and `jobCache` evicts a rejected promise so a later render
  // can retry, which would turn one failing card into a request per token. The
  // deck keys its cards by slug, so this instance's slug never changes.
  const jobRequest = loadJob(entry.slug);
</script>

{#snippet rationale()}
  <div class="space-y-1.5">
    {#if entry.note}
      <p class="text-sm leading-relaxed text-foreground">{entry.note}</p>
    {/if}
    {#if entry.whyFits.length > 0}
      <div class="flex flex-wrap gap-1.5">
        {#each entry.whyFits as fit, i (i)}
          <Badge variant="brand">{fit}</Badge>
        {/each}
      </div>
    {/if}
    {#each entry.concerns as concern, i (i)}
      <p class="flex items-start gap-1.5 text-xs leading-relaxed text-muted-foreground">
        <TriangleAlert class="mt-0.5 size-3.5 shrink-0 text-destructive/60" aria-hidden="true" />
        <span>{concern}</span>
      </p>
    {/each}
  </div>
{/snippet}

{#await jobRequest}
  <JobCardSkeleton />
{:then job}
  <JobRow {job} dimViewed={false} newTab compact footer={rationale} />
{:catch}
  <div class="rounded-xl border border-border bg-card p-3">
    <a
      href={resolve('/jobs/[slug]', { slug: entry.slug })}
      target="_blank"
      rel="noopener"
      class="text-sm font-medium text-brand underline underline-offset-2"
    >
      View job ↗
    </a>
    <div class="mt-2">{@render rationale()}</div>
  </div>
{/await}
