<script lang="ts">
  import { onMount } from 'svelte';
  import ProviderIcon from './ProviderIcon.svelte';
  import { githubStars, formatStars, GITHUB_URL } from '$lib/github.svelte';
  import { cn } from '$lib/utils';

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
    {#if count != null}
      <span class="ml-auto tabular-nums text-xs">{formatStars(count)}</span>
    {/if}
  {:else if count != null}
    <span class="tabular-nums">{formatStars(count)}</span>
  {/if}
</a>
