<script lang="ts">
  import { Check, User } from '@lucide/svelte';
  import { api, ApiError } from '$lib/api';
  import { filtersFromProfile, filtersToParams } from '$lib/filters';
  import { savedSearches } from '$lib/savedSearches.svelte';
  import { cn } from '$lib/ui';
  import type { SavedSearch, UserProfile } from '$lib/types';

  // The built-in "notify me about jobs matching my profile" toggle: flipping it on
  // creates the exact saved search the modal's "Apply my profile" action would stage
  // (see stagedFilters.svelte.ts's applyProfile / filtersFromProfile), marks it
  // derived_from_profile so at most one exists per user (the server's partial unique
  // index is the actual invariant), and subscribes it over email — always deliverable
  // with no linking step, unlike telegram/push (which the account's notification
  // settings may list as a preferred channel without the user ever having connected
  // them — see ReminderSettings.svelte/CompanyFollowButton.svelte's `linked` gate).
  // The user can add another channel from the Search alerts tab afterward. Flipping
  // the toggle off deletes the saved search — the ON DELETE CASCADE on
  // subscriptions.saved_search_id takes the subscription with it, so there is nothing
  // else to clean up here. On/off is derived from whether such a row exists, not a
  // separate flag, so this component never drifts from the server.
  let { profile }: { profile: UserProfile } = $props();

  const profileSearch = $derived(savedSearches.items.find((s) => s.derived_from_profile) ?? null);
  const enabled = $derived(profileSearch !== null);

  let saveState = $state<'idle' | 'saving' | 'saved' | 'error'>('idle');
  let saveError = $state<string | null>(null);
  let savedTimer: ReturnType<typeof setTimeout> | undefined;

  $effect(() => {
    void savedSearches.ensureLoaded();
  });

  function flash() {
    saveState = 'saved';
    clearTimeout(savedTimer);
    savedTimer = setTimeout(() => {
      if (saveState === 'saved') saveState = 'idle';
    }, 1500);
  }

  async function enable() {
    saveState = 'saving';
    saveError = null;
    let search: SavedSearch | undefined;
    try {
      const query = filtersToParams(filtersFromProfile(profile)).toString();
      search = await savedSearches.create('My profile', query, true);
      await api.createSubscription(search.id, 'email');
      flash();
    } catch (e) {
      // The search may have been created before the subscribe call failed — clean it
      // up so the toggle doesn't end up "on" (search exists) while showing an error.
      if (search) {
        try {
          await savedSearches.remove(search.id);
        } catch {
          // best-effort; the toggle's error state still surfaces the original failure.
        }
      }
      saveState = 'error';
      saveError = e instanceof ApiError ? e.message : 'Could not enable the alert.';
    }
  }

  async function disable() {
    if (!profileSearch) return;
    saveState = 'saving';
    saveError = null;
    try {
      await savedSearches.remove(profileSearch.id);
      flash();
    } catch (e) {
      saveState = 'error';
      saveError = e instanceof ApiError ? e.message : 'Could not disable the alert.';
    }
  }

  function toggle() {
    void (enabled ? disable() : enable());
  }
</script>

<section class="rounded-xl border border-border bg-card p-4">
  <div class="flex items-center gap-3">
    <div class="grid size-9 shrink-0 place-items-center rounded-lg bg-brand-muted text-brand-strong">
      <User class="size-4.5" aria-hidden="true" />
    </div>
    <div class="min-w-0 flex-1">
      <h2 class="text-sm font-semibold leading-tight">Jobs matching my profile</h2>
      <p class="text-xs text-muted-foreground">
        Get notified when a new job matches your role, skills and location preferences.
      </p>
    </div>

    {#if saveState === 'saving'}
      <span class="text-xs text-muted-foreground">Saving…</span>
    {:else if saveState === 'saved'}
      <span class="flex items-center gap-1 text-xs text-brand-strong"><Check class="size-3.5" aria-hidden="true" /> Saved</span>
    {:else if saveState === 'error'}
      <span class="text-xs text-destructive">{saveError}</span>
    {/if}

    {@render toggleSwitch(enabled, 'Notify me about jobs matching my profile', toggle, saveState === 'saving')}
  </div>
</section>

{#snippet toggleSwitch(on: boolean, label: string, onToggle: () => void, disabled: boolean)}
  <button
    type="button"
    role="switch"
    aria-checked={on}
    aria-label={label}
    onclick={onToggle}
    {disabled}
    class={cn(
      'relative inline-flex h-6 w-11 shrink-0 items-center rounded-full transition-colors disabled:opacity-50',
      'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-ring focus-visible:ring-offset-2 focus-visible:ring-offset-card',
      on ? 'bg-brand' : 'bg-muted',
    )}
  >
    <span
      class={cn(
        'inline-block size-5 rounded-full bg-white shadow-sm transition-transform',
        on ? 'translate-x-[22px]' : 'translate-x-0.5',
      )}
    ></span>
  </button>
{/snippet}
