<script lang="ts">
  import LocationCard from '$lib/components/profile/LocationCard.svelte';
  import { profileStore } from '$lib/profile.svelte';
  import { handleSaved } from '../actions';

  const profile = $derived(profileStore.profile);
</script>

<!-- LocationPreferencesFields seeds its local edit state once from `profile`, on the
     explicit contract that the caller remounts it on a genuinely different value (see
     its own doc comment) — profileStore.refresh() (CV delete/upload) can replace
     `profile` while this view is open, so key it on `updated_at`. -->
{#if profile}
  {#key profile.updated_at}
    <LocationCard {profile} onProfileChanged={handleSaved} />
  {/key}
{/if}
