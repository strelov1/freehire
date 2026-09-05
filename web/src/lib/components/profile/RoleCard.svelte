<script lang="ts">
  // Role: specializations, editable inline — no separate "Role" tab to click into (it's
  // the one piece of the old identity header that doesn't carry enough content to earn
  // its own view, unlike Contacts/Location which do). Autosaves straight to profileStore
  // on every change, the same way the Skills view does.
  import { CATEGORY_OPTIONS } from '$lib/facets';
  import { profileStore } from '$lib/profile.svelte';
  import { MAX_SPECIALIZATIONS } from '$lib/profileLimits';
  import type { UserProfile } from '$lib/types';
  import SearchSelect from '../facets/SearchSelect.svelte';

  let {
    profile,
    onProfileChanged,
  }: {
    profile: UserProfile;
    /** Fired after a Role change saves — the profile page's coverage reload and the
     *  profile-derived saved-search alert both need to stay in step with it, same as they
     *  did when Role lived behind ProfileForm's batched Save. */
    onProfileChanged?: () => void;
  } = $props();

  let specError = $state<string | null>(null);
  let specBusy = $state(false);

  async function toggleSpecialization(value: string) {
    if (specBusy) return;
    const has = profile.specializations.includes(value);
    if (has && profile.specializations.length === 1) {
      specError = 'You need at least one role — add another before removing this one.';
      return;
    }
    if (!has && profile.specializations.length >= MAX_SPECIALIZATIONS) {
      specError = `You can pick at most ${MAX_SPECIALIZATIONS} specializations.`;
      return;
    }
    specError = null;
    specBusy = true;
    const next = has
      ? profile.specializations.filter((s) => s !== value)
      : [...profile.specializations, value];
    try {
      await profileStore.updateSpecializations(next);
      onProfileChanged?.();
    } catch {
      specError = 'Could not update your role. Try again.';
    } finally {
      specBusy = false;
    }
  }
</script>

<div class="flex flex-col gap-2">
  <div class="flex items-baseline justify-between">
    <span class="text-sm font-medium">Roles</span>
    <span class="text-xs tabular-nums text-muted-foreground">
      {profile.specializations.length}/{MAX_SPECIALIZATIONS}
    </span>
  </div>
  <div class={specBusy ? 'pointer-events-none opacity-60' : ''}>
    <SearchSelect
      options={CATEGORY_OPTIONS}
      include={profile.specializations}
      placeholder="Search specializations"
      onToggle={toggleSpecialization}
      clearOnSelect
    />
  </div>
  {#if specError}
    <p class="text-xs text-destructive">{specError}</p>
  {/if}
</div>
