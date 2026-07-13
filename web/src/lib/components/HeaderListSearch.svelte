<script lang="ts">
  import { page } from '$app/state';
  import { Search, X } from '@lucide/svelte';
  import { listSearchTarget } from '$lib/listSearch.svelte';
  import HeaderLocationFilter from './HeaderLocationFilter.svelte';

  // The header's list-mode input: on /jobs and /companies it IS the page's text
  // search, proxying straight into the active list store (registered by the view).
  // No dropdown — the page's own list is the live result. Reuses the store's URL
  // sync + debounce, so this stays a thin remote control.
  let { placeholder }: { placeholder: string } = $props();

  let inputEl = $state<HTMLInputElement | null>(null);

  const target = $derived(listSearchTarget());
  // Fall back to the URL's `q` before the view registers (SSR + first paint), so a
  // shared /jobs?q=… link shows its query immediately.
  const q = $derived(target?.value.q ?? page.url.searchParams.get('q') ?? '');

  // Same global hotkeys as the launcher: Cmd/Ctrl+K always, `/` unless typing.
  function onWindowKeydown(e: KeyboardEvent) {
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
      e.preventDefault();
      inputEl?.focus();
      return;
    }
    if (e.key === '/' && !e.metaKey && !e.ctrlKey) {
      const tag = (document.activeElement as HTMLElement | null)?.tagName;
      if (tag !== 'INPUT' && tag !== 'TEXTAREA') {
        e.preventDefault();
        inputEl?.focus();
      }
    }
  }
</script>

<svelte:window onkeydown={onWindowKeydown} />

<div class="relative flex-1">
  <div
    class="flex h-11 items-center gap-2 rounded-md border border-border bg-background px-3 text-sm focus-within:ring-2 focus-within:ring-ring"
  >
    <!-- List pages expose a filter scope: surface the Location quick-filter as a
         scope-prefix, divided from the search icon. `variant` picks the popover body
         (jobs work-format+location vs the company region/remote-hiring pills). -->
    {#if target?.filterScope}
      <HeaderLocationFilter
        variant={target.filterScope.variant}
        store={target.filterScope.store}
        counts={target.filterScope.counts()}
      />
      <div class="h-5 w-px shrink-0 bg-border"></div>
    {/if}
    <Search class="size-4 shrink-0 text-muted-foreground" />
    <input
      bind:this={inputEl}
      value={q}
      oninput={(e) => target?.setQuery(e.currentTarget.value)}
      type="text"
      {placeholder}
      aria-label={placeholder}
      autocomplete="off"
      spellcheck="false"
      class="min-w-0 flex-1 bg-transparent outline-none placeholder:text-muted-foreground"
    />
    {#if q}
      <button
        type="button"
        onclick={() => target?.setQuery('')}
        aria-label="Clear search"
        class="shrink-0 text-muted-foreground transition-colors hover:text-foreground"
      >
        <X class="size-4" />
      </button>
    {:else}
      <kbd
        class="hidden shrink-0 rounded border border-border px-1.5 text-xs text-muted-foreground sm:inline"
        >/</kbd
      >
    {/if}
  </div>
</div>
