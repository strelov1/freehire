<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { ChevronDown } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { menuNav } from '#lib/actions/menuNav.js';
  import { announce } from '#lib/announce.svelte.js';
  import { feedSource, type FeedSourceChoice } from '#lib/feedSource.svelte.js';

  // The source pill of the match feed and its menu: the profile or one saved search. Shared by
  // the list and swipe modes. `compact` drops the label's min width for the swipe header.
  let open = $state(false);
  let root = $state<HTMLElement | null>(null);
  let pill = $state<HTMLButtonElement | null>(null);
  let failed = $state(false);

  const item =
    'flex min-h-11 w-full items-center gap-2 px-3 py-2 text-left text-sm hover:bg-accent focus:bg-accent focus:outline-none';

  onMount(() => {
    void feedSource.loadSearches();
    // A pick remounts this pill when the feed is keyed on it (swipe mode): focus follows.
    if (feedSource.refocus) {
      feedSource.refocus = false;
      pill?.focus();
    }
  });

  async function pick(choice: FeedSourceChoice) {
    const same =
      choice.source === feedSource.source &&
      (choice.source === 'profile' || choice.saved_search_id === feedSource.savedSearchId);
    open = false;
    if (same) return;
    failed = false;
    try {
      await feedSource.choose(choice);
    } catch {
      failed = true;
      open = true; // keep the menu so the one-line status shows
      return;
    }
    announce(`${feedSource.label}.`);
    await tick();
    if (pill?.isConnected) {
      feedSource.refocus = false;
      pill.focus();
    }
  }

  const checked = (id: number) => feedSource.source === 'saved_search' && feedSource.savedSearchId === id;
</script>

<svelte:window onclick={(e) => open && !root?.contains(e.target as Node) && (open = false)} />

<div class="relative" bind:this={root}>
  <button
    bind:this={pill}
    type="button"
    aria-label={`Match source: ${feedSource.label}`}
    aria-haspopup="menu"
    aria-expanded={open}
    class="flex min-h-11 max-w-full items-center gap-2 rounded-full border border-border bg-card px-4 py-2 text-sm font-medium transition hover:bg-accent"
    onclick={() => (open = !open)}
  >
    <span class="truncate">{feedSource.label}</span>
    <ChevronDown class="size-4 shrink-0" aria-hidden="true" />
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
        aria-checked={feedSource.source === 'profile'}
        class={item}
        onclick={() => pick({ source: 'profile' })}
      >
        By my profile
      </button>
      {#each feedSource.searches as s (s.id)}
        <button
          type="button"
          role="menuitemradio"
          aria-checked={checked(s.id)}
          class={item}
          onclick={() => pick({ source: 'saved_search', saved_search_id: s.id })}
        >
          <span class="truncate">{s.name}</span>
        </button>
      {/each}
      {#if feedSource.status === 'loading'}
        <p class="px-3 py-2 text-sm text-muted-foreground">Loading your searches…</p>
      {:else if feedSource.status === 'failed'}
        <p class="px-3 py-2 text-sm text-muted-foreground">Couldn't load your saved searches.</p>
      {:else if feedSource.searches.length === 0}
        <a role="menuitem" class={item} href={resolve('/jobs')} onclick={() => (open = false)}>Create a saved search</a>
      {/if}
      {#if failed}
        <p role="status" class="px-3 py-2 text-sm text-destructive">Couldn't switch the source — try again.</p>
      {/if}
    </div>
  {/if}
</div>
