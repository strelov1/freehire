<script lang="ts">
  import { onMount } from 'svelte';
  import { ProviderIcon } from '$lib/ui';
  import { githubStars, formatStars, GITHUB_URL } from '$lib/github.svelte';
  import { cn } from '$lib/ui';

  // A link to the repo with the live star count. Two shapes from one component:
  // `inline` is the compact desktop-bar badge (icon + count); `row` is the
  // full-width drawer row on mobile (icon + "GitHub" label + count pushed right).
  // The count comes from the shared store — the first mounted instance loads it,
  // every other instance just reads it reactively.
  let {
    variant = 'inline',
    class: className = '',
  }: { variant?: 'inline' | 'row'; class?: string } = $props();

  onMount(() => {
    void githubStars.load();
  });

  const count = $derived(githubStars.count);
</script>

<!-- eslint-disable svelte/no-navigation-without-resolve -- external GitHub URL, not an internal route -->
<a
  href={GITHUB_URL}
  target="_blank"
  rel="noreferrer"
  role={variant === 'row' ? 'menuitem' : undefined}
  aria-label="freehire on GitHub"
  class={cn(
    variant === 'inline' &&
      'inline-flex min-h-9 items-center gap-1.5 rounded-md px-2 text-sm text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground',
    className,
  )}
>
  <!-- eslint-enable svelte/no-navigation-without-resolve -->
  <ProviderIcon provider="github" />
  {#if variant === 'row'}
    <span>GitHub</span>
    <span class="ml-auto tabular-nums text-xs">{count != null ? formatStars(count) : ''}</span>
  {:else}
    <!-- The count's box is held from the first paint, empty until the number lands.
         It cannot be there earlier: the store reads its localStorage cache in
         `onMount`, and rendering a cached number the server did not render would be a
         hydration mismatch. So the number always arrives a frame late, and without a
         reserved box it widens this badge, pushes the header's right-hand group, and
         drags the `flex-1` search box between them along with it — on every page load,
         signed in or out.

         `min-w-8` holds four tabular digits, which the repo will not outgrow soon; the
         next shape after that is "10.9k", one character wider. -->
    <span class="min-w-8 text-right tabular-nums">{count != null ? formatStars(count) : ''}</span>
  {/if}
</a>
