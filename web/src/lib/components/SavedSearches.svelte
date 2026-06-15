<script lang="ts">
  import { ApiError } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import { canonicalQuery, filtersToParams, type FilterStore } from '$lib/filters.svelte';
  import { savedSearches } from '$lib/savedSearches.svelte';
  import { Button, Input } from '$lib/ui';

  // The "My filters" control: select a saved set (applies its filters), save the
  // current filters under a name, overwrite the selected set, or delete it. Mounted
  // at the top of the filters panel, so it rides into both the desktop sidebar and
  // the mobile drawer. Signed-out users see a sign-in prompt instead.
  let { store }: { store: FilterStore } = $props();

  // The set the user is working from (selected or just saved). Distinct from the
  // derived `activeId` so we can offer "Update <name>" after the filters are edited
  // away from it.
  let baseId = $state<number | null>(null);
  let naming = $state(false);
  let name = $state('');
  let busy = $state(false);
  let error = $state<string | null>(null);

  // The current filters as a canonical query string (filtersToParams is already
  // canonical), and the saved set — if any — that exactly matches it.
  const current = $derived(filtersToParams(store.value).toString());
  const items = $derived(savedSearches.items);
  const activeId = $derived(items.find((s) => canonicalQuery(s.query) === current)?.id ?? null);
  const base = $derived(items.find((s) => s.id === baseId) ?? null);
  // The user started from a saved set and then changed the filters.
  const dirty = $derived(base != null && canonicalQuery(base.query) !== current);

  // Load the list once the session is confirmed (boot-time /me may still be in
  // flight); the store no-ops on repeat calls and off the browser. On sign-out,
  // drop the cache so a different user signing in on the same tab can't see the
  // previous user's saved searches (the store is a per-user module singleton).
  $effect(() => {
    if (isAuthenticated()) {
      void savedSearches.ensureLoaded();
    } else {
      savedSearches.reset();
    }
  });

  function onSelect(e: Event) {
    error = null;
    naming = false;
    const value = (e.currentTarget as HTMLSelectElement).value;
    if (value === '') {
      store.clear();
      baseId = null;
      return;
    }
    const set = items.find((s) => s.id === Number(value));
    if (set) {
      store.apply(set.query);
      baseId = set.id;
    }
  }

  function startSave() {
    error = null;
    name = '';
    naming = true;
  }

  async function save() {
    const trimmed = name.trim();
    if (!trimmed || busy) return;
    busy = true;
    error = null;
    try {
      const set = await savedSearches.create(trimmed, current);
      baseId = set.id;
      naming = false;
      name = '';
    } catch (e) {
      error = e instanceof ApiError ? e.message : 'Could not save. Please try again.';
    } finally {
      busy = false;
    }
  }

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

  async function remove() {
    if (activeId == null || busy) return;
    const set = items.find((s) => s.id === activeId);
    if (!set || !window.confirm(`Delete the saved filter “${set.name}”?`)) return;
    busy = true;
    error = null;
    try {
      await savedSearches.remove(set.id);
      if (baseId === set.id) baseId = null;
    } catch (e) {
      error = e instanceof ApiError ? e.message : 'Could not delete. Please try again.';
    } finally {
      busy = false;
    }
  }

  const selectClass =
    'h-9 w-full rounded-lg border border-input bg-transparent px-3 text-sm transition-colors focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 dark:bg-input/30';
</script>

<div class="flex flex-col gap-2 border-b border-border pb-4">
  <h3 class="text-sm font-semibold tracking-tight">My filters</h3>

  {#if !isAuthenticated()}
    <button
      type="button"
      class="self-start text-sm font-medium text-primary underline-offset-4 hover:underline"
      onclick={() => openAuthDialog()}
    >
      Sign in to save filters
    </button>
  {:else}
    {#if items.length > 0}
      <select aria-label="Select a saved filter" class={selectClass} value={activeId ?? ''} onchange={onSelect}>
        <option value="">Select a saved filter…</option>
        {#each items as set (set.id)}
          <option value={set.id}>{set.name}</option>
        {/each}
      </select>
    {/if}

    {#if naming}
      <div class="flex items-center gap-2">
        <Input
          bind:value={name}
          placeholder="Filter name"
          maxlength={100}
          class="min-w-0 flex-1"
          onkeydown={(e: KeyboardEvent) => e.key === 'Enter' && save()}
        />
        <Button variant="primary" size="sm" onclick={save} disabled={!name.trim() || busy}>
          {busy ? 'Saving…' : 'Save'}
        </Button>
        <Button variant="ghost" size="sm" onclick={() => (naming = false)}>Cancel</Button>
      </div>
    {:else}
      <div class="flex flex-wrap items-center gap-2">
        {#if dirty}
          <Button variant="secondary" size="sm" onclick={update} disabled={busy}>
            Update “{base?.name}”
          </Button>
        {/if}
        <Button variant="secondary" size="sm" onclick={startSave} disabled={busy}>Save as new</Button>
        {#if activeId != null}
          <Button variant="ghost" size="sm" onclick={remove} disabled={busy}>Delete</Button>
        {/if}
      </div>
    {/if}

    {#if error}
      <p class="text-sm text-destructive">{error}</p>
    {/if}
  {/if}
</div>
