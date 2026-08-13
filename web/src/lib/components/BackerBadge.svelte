<script lang="ts">
  import { resolve } from '$app/paths';
  import { backerBadges } from '$lib/backers';
  import { cn } from '$lib/ui';

  // Renders the accelerators and funds that selected a company — Y Combinator,
  // Techstars, a16z.
  //
  // The mark carries the meaning on its own: at feed size a reader recognises the
  // orange Y long before they would read a word. `withLabel` adds the brand name for
  // the surfaces with room for it (the job and company pages), where the badge also
  // becomes a link to the collection.
  //
  // No monogram fallback, unlike EntityLogo: a letter tile standing in for the a16z
  // mark reads as a defect, not as graceful degradation. The marks are committed, so
  // the only way to have none is a backer with no entry in $lib/backers — and that
  // renders nothing at all.
  let {
    collections,
    withLabel = false,
    size = 'size-4',
    class: className,
  }: {
    collections?: string[] | null;
    withLabel?: boolean;
    size?: string;
    class?: string;
  } = $props();

  const badges = $derived(backerBadges(collections));
</script>

{#each badges as badge (badge.slug)}
  {#if withLabel}
    <a
      href={resolve('/collections/[slug]', { slug: badge.slug })}
      class={cn(
        'inline-flex items-center gap-1.5 rounded-md border border-border px-2 py-0.5 text-xs font-medium text-muted-foreground hover:bg-muted',
        className,
      )}
    >
      <img src={badge.mark} alt="" class="{size} shrink-0 rounded-sm object-contain" />
      {badge.label}
      <!-- The visible label names the brand but not the relationship. A screen
           reader gets the whole sentence, so "Y Combinator" is never announced as
           though it described the role. -->
      <span class="sr-only">— {badge.alt}</span>
    </a>
  {:else}
    <img
      src={badge.mark}
      alt={badge.alt}
      title={badge.alt}
      class={cn(size, 'shrink-0 rounded-sm object-contain', className)}
      loading="lazy"
    />
  {/if}
{/each}
