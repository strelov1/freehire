<script lang="ts">
  import type { Company } from '$lib/types';

  // The company's full description as a standalone "About" card, mirroring how a job
  // renders its description as its own block. The header only shows the derived
  // tagline (first sentence); this is the whole summary. Present-only: renders nothing
  // when the company has no description, so the page never leaves an empty box.
  let { company }: { company: Company } = $props();

  const description = $derived(company.company_info?.description?.trim() ?? '');

  // Collapsed to a few lines, expandable — a long summary shouldn't dominate the page
  // above the jobs. The toggle appears only when the text actually overflows the clamp,
  // measured once the paragraph is in the DOM (scrollHeight vs the clamped clientHeight).
  let expanded = $state(false);
  let clampable = $state(false);
  let para = $state<HTMLParagraphElement>();

  $effect(() => {
    if (para && !expanded) clampable = para.scrollHeight > para.clientHeight + 1;
  });
</script>

{#if description}
  <section class="rounded-2xl border border-border bg-card p-5">
    <p class="mb-3 text-xs font-semibold uppercase tracking-wide text-muted-foreground">About</p>
    <p
      bind:this={para}
      class={[
        'whitespace-pre-wrap text-sm leading-relaxed text-muted-foreground',
        !expanded && 'line-clamp-4',
      ]}
    >
      {description}
    </p>
    {#if clampable || expanded}
      <button
        type="button"
        class="mt-2 text-sm font-medium text-primary hover:underline"
        onclick={() => (expanded = !expanded)}
      >
        {expanded ? 'Show less' : 'Show more'}
      </button>
    {/if}
  </section>
{/if}
