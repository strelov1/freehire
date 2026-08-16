<script lang="ts">
  import type { Company } from '$lib/types';
  import { companyDescription } from '$lib/companyDetails';

  // The company's full description as an "About" card in the jobs sidebar, under
  // Company facts, and in the job page's Company tab. The header only shows the derived
  // tagline (first sentence); this is the whole summary. Present-only: renders nothing
  // when the company has no description, so the sidebar never leaves an empty box.
  let { company }: { company: Company } = $props();

  const description = $derived(companyDescription(company));

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
  <section class="rounded-xl border border-border bg-card p-4">
    <p class="mb-3 text-xs font-semibold uppercase tracking-wide text-muted-foreground">About</p>
    <p
      bind:this={para}
      class={[
        'whitespace-pre-wrap text-sm leading-relaxed text-muted-foreground',
        !expanded && 'line-clamp-3',
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
