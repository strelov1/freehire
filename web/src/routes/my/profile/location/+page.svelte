<script lang="ts">
  import AccountTimezone from '#lib/components/AccountTimezone.svelte';
  import LocationCard from '#lib/components/profile/LocationCard.svelte';
  import CompletenessBadge from '#lib/components/profile/CompletenessBadge.svelte';
  import { profileStore } from '#lib/profile.svelte.js';
  import { handleSaved } from '../actions';

  const profile = $derived(profileStore.profile);
</script>

<!-- LocationPreferencesFields seeds its local edit state once from `profile`, on the
     explicit contract that the caller remounts it on a genuinely different value (see
     its own doc comment) — profileStore.refresh() (CV delete/upload) can replace
     `profile` while this view is open, so key it on `updated_at`. -->
{#if profile}
  <div class="mb-4">
    <CompletenessBadge section="location" />
  </div>

  {#key profile.updated_at}
    <LocationCard {profile} onProfileChanged={handleSaved} />
  {/key}
{/if}

<!-- The timezone belongs with where the candidate is: the Talent Network card shows it, and
     digests and quiet hours are read in it. Outside the `{#key}` above, so a profile refresh
     never remounts it mid-pick. `account-timezone` is the target of the notification
     settings' "set your timezone" hint. -->
<div id="account-timezone" class="mt-8 scroll-mt-20 border-t border-border pt-6">
  <AccountTimezone />
</div>
