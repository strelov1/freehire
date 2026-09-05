<script lang="ts">
  // Role: specializations and level, editable inline — no separate "Role" tab to click
  // into (it's the one piece of the old identity header that doesn't carry enough content
  // to earn its own view, unlike Contacts/Location which do). Autosaves straight to
  // profileStore on every change, the same way the Skills view does.
  import { CATEGORY_OPTIONS } from '$lib/facets';
  import { profileStore } from '$lib/profile.svelte';
  import { MAX_SPECIALIZATIONS } from '$lib/profileLimits';
  import type { UserProfile } from '$lib/types';
  import SearchSelect from '../facets/SearchSelect.svelte';
  import SeniorityPills from './SeniorityPills.svelte';

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
  let levelError = $state<string | null>(null);
  let levelBusy = $state(false);

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

  // Level is a free multi-select with no floor, unlike the specializations above: someone
  // who is between levels ("senior or lead") is stating both, and someone who wants to stop
  // stating one at all must be able to — a profile that cannot go back to silent would make
  // the wizard's answer permanent. The account-setup card simply re-opens its step when the
  // list empties, which is the honest reading of "no level stated".
  async function toggleSeniority(value: string) {
    if (levelBusy) return;
    levelError = null;
    levelBusy = true;
    const next = profile.seniorities.includes(value)
      ? profile.seniorities.filter((s) => s !== value)
      : [...profile.seniorities, value];
    try {
      await profileStore.updateSeniorities(next);
      onProfileChanged?.();
    } catch {
      levelError = 'Could not update your level. Try again.';
    } finally {
      levelBusy = false;
    }
  }
</script>

<!-- `account-role` is the anchor the account-setup checklist's role step links to (see
     accountCompleteness.ts). It sits on the whole card, not on either half, because that
     step asks for both together — "what you do, and at what level". `scroll-mt-20` clears
     the sticky header (`h-14`), same as the other anchored sections. -->
<div id="account-role" class="flex scroll-mt-20 flex-col gap-6">
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

  <!-- Level lives beside Roles rather than in its own card: it was previously askable only
       inside the onboarding wizard, so a profile whose level needed changing had no surface
       at all. The control itself is the wizard's own, shared (SeniorityPills). -->
  <div class="flex flex-col gap-2">
    <div class="flex items-baseline justify-between">
      <span class="text-sm font-medium">Level</span>
      <span class="text-xs text-muted-foreground">Pick every level you'd take</span>
    </div>
    <SeniorityPills selected={profile.seniorities} onToggle={toggleSeniority} busy={levelBusy} />
    {#if levelError}
      <p class="text-xs text-destructive">{levelError}</p>
    {/if}
  </div>
</div>
