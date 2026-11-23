<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { ChevronDown } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { menuNav } from '#lib/actions/menuNav.js';
  import { announce } from '#lib/announce.svelte.js';
  import { feedSource } from '#lib/feedSource.svelte.js';
  import type { FeedSourceChoice } from '#lib/types.js';

  // The source pill: what the feed matches on, and a menu to switch it.
  let open = $state(false);
  let failed = $state(false);
  let root = $state<HTMLElement | null>(null);
  let pill = $state<HTMLButtonElement | null>(null);
  const isProfile = $derived(feedSource.current?.kind === 'profile');
  const item =
    'flex min-h-11 w-full items-center px-3 py-2 text-left text-sm hover:bg-accent focus:bg-accent focus:outline-none aria-checked:font-semibold aria-disabled:opacity-60';

  onMount(() => void feedSource.loadSearches());

  async function pick(choice: FeedSourceChoice) {
    if (feedSource.busy) return; // the previous pick is still being stored
    open = false;
    failed = false;
    pill?.focus();
    if (
      choice.source === feedSource.current?.kind &&
      (choice.source === 'profile' || choice.saved_search_id === feedSource.current.id)
    ) {
      return;
    }
    if (await feedSource.choose(choice)) {
      announce(feedSource.label);
      await tick(); // swipe remounts its own pill: focus the live (topmost) one
      [...document.querySelectorAll<HTMLElement>('[data-source-pill]')].at(-1)?.focus();
    } else {
      failed = true;
      announce("Didn't switch — try again", 'assertive');
    }
  }
</script>

<svelte:window onclick={(e) => open && !root?.contains(e.target as Node) && (open = false)} />

<div class="relative" bind:this={root}>
  <button
    bind:this={pill}
    data-source-pill
    type="button"
    aria-haspopup="menu"
    aria-expanded={open}
    aria-label={`Match source: ${feedSource.label}`}
    class="flex min-h-11 items-center gap-2 rounded-full border border-border bg-card px-4 py-2 text-sm font-medium transition hover:bg-accent"
    onclick={() => {
      open = !open;
      if (open && feedSource.status === 'failed') void feedSource.loadSearches();
    }}
  >
    <span class="max-w-48 truncate">{feedSource.label}</span>
    <ChevronDown class="size-4" aria-hidden="true" />
  </button>
  {#if open}
    <div
      role="menu"
      aria-label="Match source"
      tabindex="-1"
      class="absolute left-0 top-full z-20 mt-2 w-64 rounded-md border border-border bg-background py-1 shadow-lg"
      onkeydown={(e) => e.key === 'Escape' && ((open = false), e.stopPropagation())}
      {@attach menuNav({ onclose: () => (open = false) })}
    >
      <button
        type="button"
        role="menuitemradio"
        aria-checked={isProfile}
        class={item}
        onclick={() => pick({ source: 'profile' })}
      >
        By my profile
      </button>
      {#each feedSource.searches as s (s.id)}
        <button
          type="button"
          role="menuitemradio"
          aria-checked={feedSource.current?.kind === 'saved_search' && feedSource.current.id === s.id}
          class={item}
          onclick={() => pick({ source: 'saved_search', saved_search_id: s.id })}
        >
          <span class="truncate">{s.name}</span>
        </button>
      {/each}
      {#if feedSource.status === 'loading'}
        <p class="px-3 py-2 text-sm text-muted-foreground">Loading your searches…</p>
      {:else if feedSource.status === 'failed'}
        <p class="px-3 py-2 text-sm text-muted-foreground">Couldn't load your searches.</p>
      {:else if feedSource.searches.length === 0}
        <a role="menuitem" class={item} href={resolve('/jobs')}>Create a saved search</a>
      {/if}
      {#if failed}
        <p class="px-3 py-2 text-sm text-destructive">Didn't switch — try again.</p>
      {/if}
    </div>
  {/if}
</div>
