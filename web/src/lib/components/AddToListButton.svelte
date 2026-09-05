<script lang="ts">
  import { Check, ListPlus, Plus } from '@lucide/svelte';
  import { api, ApiError } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { promptSignIn } from '$lib/signin';
  import { jobLists } from '$lib/jobLists.svelte';
  import type { JobListMembership } from '$lib/types';

  // A self-contained "Add to list" control: an icon button that opens a small
  // hand-rolled popover (no shared Menu/Popover primitive exists in the design
  // system — this mirrors HeaderMenu.svelte's own bound-root + click-outside +
  // Escape recipe) listing the caller's job lists as toggles, plus an inline
  // create-new-list form. Independent of the "Save" star (savedJobs.svelte):
  // a job can be saved, listed, both, or neither.
  //
  // Membership is fetched lazily — only when the popover first opens, not for
  // every card in a feed — via GET /me/lists/membership?job_slug=, which is
  // cheap in isolation but would be wasteful eagerly.
  let { jobSlug, class: className = '' }: { jobSlug: string; class?: string } = $props();

  let open = $state(false);
  let root = $state<HTMLElement | null>(null);
  let status = $state<'idle' | 'loading' | 'error' | 'ready'>('idle');
  let membership = $state<JobListMembership[]>([]);
  let error = $state<string | null>(null);
  let busyId = $state<number | null>(null);

  let creating = $state(false);
  let newName = $state('');
  let createBusy = $state(false);

  // The trigger tints like the Save bookmark once the job is in at least one list,
  // so a glance at the corner shows both marks the same way.
  const anyInList = $derived(membership.some((m) => m.in_list));

  function onWindowClick(e: MouseEvent) {
    if (open && root && !root.contains(e.target as Node)) open = false;
  }

  async function load() {
    status = 'loading';
    error = null;
    try {
      membership = await api.listJobListMembership(jobSlug);
      status = 'ready';
    } catch {
      status = 'error';
    }
  }

  function toggleOpen(e: MouseEvent) {
    e.stopPropagation();
    if (!isAuthenticated()) {
      promptSignIn();
      return;
    }
    open = !open;
    if (open && status === 'idle') void load();
  }

  // Optimistic toggle, mirroring JobRow's save button: flip locally first, confirm
  // with the server, roll back on failure. Routed through the jobLists store (not a
  // bare api.* call) so its cached #items — read by /my/lists in the same session —
  // stay in sync (job_count updated in place).
  async function toggle(m: JobListMembership) {
    busyId = m.id;
    error = null;
    const wasIn = m.in_list;
    membership = membership.map((x) => (x.id === m.id ? { ...x, in_list: !wasIn } : x));
    try {
      if (wasIn) await jobLists.removeJob(m.id, jobSlug);
      else await jobLists.addJob(m.id, jobSlug);
    } catch (err) {
      membership = membership.map((x) => (x.id === m.id ? { ...x, in_list: wasIn } : x));
      error = err instanceof ApiError ? err.message : 'Could not update this list.';
    } finally {
      busyId = null;
    }
  }

  function startCreate() {
    creating = true;
    newName = '';
    error = null;
  }

  async function confirmCreate() {
    const name = newName.trim();
    if (!name) return;
    createBusy = true;
    error = null;
    try {
      const list = await jobLists.create(name);
      await jobLists.addJob(list.id, jobSlug);
      membership = [{ id: list.id, name: list.name, in_list: true }, ...membership];
      creating = false;
    } catch (err) {
      error = err instanceof ApiError ? err.message : 'Could not create this list.';
    } finally {
      createBusy = false;
    }
  }
</script>

<svelte:window onclick={onWindowClick} onkeydown={(e) => e.key === 'Escape' && (open = false)} />

<div class="relative inline-block {className}" bind:this={root}>
  <button
    type="button"
    onclick={toggleOpen}
    aria-haspopup="menu"
    aria-expanded={open}
    aria-label={anyInList ? 'Manage lists for this job' : 'Add to a list'}
    title={anyInList ? 'In a list' : 'Add to list'}
    class={[
      'grid size-8 place-items-center rounded-lg transition hover:bg-accent hover:text-brand',
      anyInList ? 'text-brand' : 'text-muted-foreground',
    ]}
  >
    <ListPlus class="size-[1.05rem]" aria-hidden="true" />
  </button>

  {#if open}
    <div
      role="menu"
      class="absolute right-0 top-full z-50 mt-1 w-56 rounded-xl border border-border bg-card p-1.5 text-left shadow-lg"
    >
      {#if error}
        <p class="px-2 py-1 text-xs text-destructive">{error}</p>
      {/if}
      {#if status === 'loading'}
        <p class="px-2 py-1.5 text-xs text-muted-foreground">Loading…</p>
      {:else if status === 'error'}
        <p class="px-2 py-1.5 text-xs text-muted-foreground">Couldn't load your lists.</p>
      {:else}
        {#if membership.length === 0}
          <p class="px-2 py-1.5 text-xs text-muted-foreground">No lists yet.</p>
        {:else}
          {#each membership as m (m.id)}
            <button
              type="button"
              role="menuitemcheckbox"
              aria-checked={m.in_list}
              disabled={busyId === m.id}
              onclick={() => toggle(m)}
              class="flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm hover:bg-accent disabled:opacity-50"
            >
              <span
                class={[
                  'flex size-4 shrink-0 items-center justify-center rounded border',
                  m.in_list ? 'border-brand bg-brand text-brand-foreground' : 'border-border',
                ]}
              >
                {#if m.in_list}<Check class="size-3" aria-hidden="true" />{/if}
              </span>
              <span class="min-w-0 flex-1 truncate">{m.name}</span>
            </button>
          {/each}
        {/if}
        <div class="mt-1 border-t border-border pt-1">
          {#if creating}
            <form
              class="flex items-center gap-1 px-1 py-1"
              onsubmit={(e) => {
                e.preventDefault();
                void confirmCreate();
              }}
            >
              <input
                bind:value={newName}
                placeholder="New list name"
                maxlength={100}
                class="min-w-0 flex-1 rounded-md border border-border bg-background px-2 py-1 text-xs focus:outline-none focus:ring-1 focus:ring-brand"
              />
              <button
                type="submit"
                disabled={createBusy || !newName.trim()}
                class="shrink-0 rounded-md px-1.5 py-1 text-xs font-medium text-brand disabled:opacity-50"
              >
                {createBusy ? '…' : 'Add'}
              </button>
            </form>
          {:else}
            <button
              type="button"
              onclick={startCreate}
              class="flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm text-muted-foreground hover:bg-accent"
            >
              <Plus class="size-4" aria-hidden="true" /> New list
            </button>
          {/if}
        </div>
      {/if}
    </div>
  {/if}
</div>
