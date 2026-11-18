<script lang="ts">
  import { resolve } from '$app/paths';
  import { backerBadges } from '#lib/backers.js';
  import { cn } from '#lib/ui/index.js';

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
  // the only way to have none is a backer with no entry in #lib/backers — and that
  // renders nothing at all.
  let {
    collections,
    withLabel = false,
    size = 'size-4',
    max = Infinity,
    class: className,
  }: {
    collections?: string[] | null;
    withLabel?: boolean;
    size?: string;
    /** Show at most this many; the rest fold into one "+N" disclosure listing them as links. */
    max?: number;
    class?: string;
  } = $props();

  const all = $derived(backerBadges(collections));
  const badges = $derived(all.slice(0, max));
  const rest = $derived(all.slice(max));
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
      <img src={badge.mark} alt="" loading="lazy" decoding="async" class="{size} shrink-0 rounded-sm object-contain" />
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
      decoding="async"
    />
  {/if}
{/each}
{#if rest.length}
  <!-- A disclosure, not a hover title: keyboard and touch readers reach the folded
       backers too, links included. -->
  <details class="relative inline-block">
    <summary
      class="inline-flex cursor-pointer list-none items-center rounded-md border border-border px-2 py-0.5 text-xs font-medium text-muted-foreground hover:bg-muted [&::-webkit-details-marker]:hidden"
    >
      +{rest.length}<span class="sr-only"> more backers</span>
    </summary>
    <div
      class="absolute left-0 z-10 mt-1 flex w-max flex-col gap-1 rounded-md border border-border bg-popover p-1.5 shadow-md"
    >
      {#each rest as badge (badge.slug)}
        <a
          href={resolve('/collections/[slug]', { slug: badge.slug })}
          class="inline-flex items-center gap-1.5 rounded px-1.5 py-0.5 text-xs text-foreground hover:bg-muted"
        >
          <img
            src={badge.mark}
            alt=""
            loading="lazy"
            decoding="async"
            class="size-4 shrink-0 rounded-sm object-contain"
          />
          {badge.label}
          <span class="sr-only">— {badge.alt}</span>
        </a>
      {/each}
    </div>
  </details>
{/if}
