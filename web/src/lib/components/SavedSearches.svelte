<script lang="ts">
  import { Bell, Bookmark, Pencil, Trash2 } from '@lucide/svelte';
  import { ApiError } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import { canonicalQuery, savedSearchQuery } from '$lib/filters';
  import type { StagedFilters } from '$lib/stagedFilters.svelte';
  import { savedSearches } from '$lib/savedSearches.svelte';
  import type { SavedSearch } from '$lib/types';
  import { Button, Input } from '$lib/ui';
  import SaveSearchAlert from './filters/SaveSearchAlert.svelte';

  // Focus the text input a rename row reveals, so the caret is ready without a second
  // click — the row only mounts inside the {#if}, so the attachment runs exactly then.
  const focusInput = (node: Element) => void (node.querySelector('input') as HTMLInputElement | null)?.focus();

  // The "My filters" control: the first tab of the filter modal. It lists the saved
  // sets (apply / rename / delete) and offers "Update <name>" once the filters drift
  // from an applied set; the shared SaveSearchAlert drives saving the current filters
  // and its Telegram alert (the same control the sidebar and onboarding use). It works
  // on the modal's *staged* copy — applying/saving act on the staged edits and reach
  // the live list only on the modal's Show-results action. Signed-out users see a
  // sign-in prompt. Board sharing lives on the /my/searches account page.
  let { store }: { store: StagedFilters } = $props();

  // The set the user applied (the base for "Update <name>"), distinct from the derived
  // `activeId` so we can offer the update after the filters are edited away from it.
  let baseId = $state<number | null>(null);
  let renamingId = $state<number | null>(null);
  let renameValue = $state('');
  let busy = $state(false);
  let error = $state<string | null>(null);

  // The current (staged) filters as a canonical query string, and the saved set — if
  // any — that exactly matches it.
  const current = $derived(savedSearchQuery(store.value));
  const items = $derived(savedSearches.items);
  const activeId = $derived(items.find((s) => canonicalQuery(s.query) === current)?.id ?? null);
  const base = $derived(items.find((s) => s.id === baseId) ?? null);
  const dirty = $derived(base != null && canonicalQuery(base.query) !== current);

  // Load the list once the session is confirmed. Sign-out cache reset is owned centrally
  // by +layout.svelte (always mounted), so it isn't duplicated here.
  $effect(() => {
    if (isAuthenticated()) void savedSearches.ensureLoaded();
  });

  // Apply a saved set: seed the staged filters from it and remember it as the base
  // (so later edits surface an "Update <name>").
  function apply(set: SavedSearch) {
    error = null;
    renamingId = null;
    store.apply(set.query);
    baseId = set.id;
  }

  function startRename(set: SavedSearch) {
    error = null;
    renamingId = set.id;
    renameValue = set.name;
  }

  function cancelRename() {
    renamingId = null;
  }

  function onRenameKey(e: KeyboardEvent) {
    if (e.key === 'Enter') void commitRename();
    else if (e.key === 'Escape') cancelRename();
  }

  async function commitRename() {
    const set = items.find((s) => s.id === renamingId);
    const trimmed = renameValue.trim();
    if (!set || !trimmed || busy) return;
    if (trimmed === set.name) {
      renamingId = null;
      return;
    }
    busy = true;
    error = null;
    try {
      await savedSearches.update(set.id, { name: trimmed });
      renamingId = null;
    } catch (e) {
      error = e instanceof ApiError ? e.message : 'Could not rename. Please try again.';
    } finally {
      busy = false;
    }
  }

  // Overwrite the applied set's query with the current filters (the "Update <name>").
  async function update() {
    if (!base || busy) return;
    busy = true;
    error = null;
    try {
      await savedSearches.update(base.id, { query: current });
    } catch (e) {
      error = e instanceof ApiError ? e.message : 'Could not update. Please try again.';
    } finally {
      busy = false;
    }
  }

  async function remove(set: SavedSearch) {
    if (busy || !window.confirm(`Delete the saved filter “${set.name}”?`)) return;
    busy = true;
    error = null;
    try {
      await savedSearches.remove(set.id);
      if (baseId === set.id) baseId = null;
      if (renamingId === set.id) renamingId = null;
    } catch (e) {
      error = e instanceof ApiError ? e.message : 'Could not delete. Please try again.';
    } finally {
      busy = false;
    }
  }
</script>

<div class="flex flex-col gap-4">
  {#if !isAuthenticated()}
    <button
      type="button"
      class="self-start text-sm font-medium text-primary underline-offset-4 hover:underline"
      onclick={() => openAuthDialog()}
    >
      Sign in to save filters
    </button>
  {:else}
    <!-- Zone 1: alerts for the filters staged right now. Save is the primary action;
         once saved, the channel toggles (Telegram / Email) render in place. -->
    <section class="flex flex-col gap-2">
      <span class="flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
        <Bell class="size-3.5" aria-hidden="true" /> Alerts for the current filters
      </span>
      <SaveSearchAlert query={current} variant="full" />

      {#if dirty}
        <Button variant="secondary" size="sm" onclick={update} disabled={busy} class="self-start">
          Update “{base?.name}”
        </Button>
      {/if}
    </section>

    <!-- Zone 2: the saved sets — click a name to apply, pencil to rename inline, trash
         to delete. The set matching the current filters is dotted + highlighted; a set
         you applied and then edited keeps a muted dot as the "base". -->
    {#if items.length > 0}
      <section class="flex flex-col gap-1.5">
        <span class="flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
          <Bookmark class="size-3.5" aria-hidden="true" /> Your saved filters
        </span>
        <p class="-mt-0.5 text-xs text-muted-foreground">Click one to apply it to the list.</p>
        <ul class="flex flex-col gap-0.5">
        {#each items as set (set.id)}
          {#if renamingId === set.id}
            <li class="flex items-center gap-2 py-0.5" {@attach focusInput}>
              <Input bind:value={renameValue} maxlength={100} class="min-w-0 flex-1" onkeydown={onRenameKey} />
              <Button variant="primary" size="sm" onclick={commitRename} disabled={!renameValue.trim() || busy}>Save</Button>
              <Button variant="ghost" size="sm" onclick={cancelRename}>Cancel</Button>
            </li>
          {:else}
            {@const active = set.id === activeId}
            {@const isBase = dirty && set.id === baseId}
            <li class="group flex items-center gap-0.5">
              <button
                type="button"
                onclick={() => apply(set)}
                title="Apply this filter"
                class={['flex min-w-0 flex-1 items-center gap-2 rounded-lg px-2 py-2 text-left transition-colors hover:bg-accent', active && 'bg-accent']}
              >
                <span class={['size-1.5 shrink-0 rounded-full transition-colors', active ? 'bg-brand' : isBase ? 'bg-muted-foreground/50' : 'bg-transparent']}></span>
                <span class={['truncate text-sm', active && 'font-medium']}>{set.name}</span>
              </button>
              <button
                type="button"
                aria-label="Rename “{set.name}”"
                title="Rename"
                onclick={() => startRename(set)}
                class="flex size-8 shrink-0 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
              >
                <Pencil class="size-4" />
              </button>
              <button
                type="button"
                aria-label="Delete “{set.name}”"
                title="Delete"
                onclick={() => remove(set)}
                class="flex size-8 shrink-0 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive"
              >
                <Trash2 class="size-4" />
              </button>
            </li>
          {/if}
        {/each}
        </ul>
      </section>
    {/if}

    {#if error}
      <p class="text-sm text-destructive">{error}</p>
    {/if}
  {/if}
</div>
