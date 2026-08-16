<script lang="ts">
  // "Where & how I want to work" as its own view — work format, base country/city, remote
  // reach, relocation — autosaving on every change straight to profileStore, the same way
  // the Roles card and the Skills view do.
  import { profileStore } from '$lib/profile.svelte';
  import type { UserProfile } from '$lib/types';
  import LocationPreferencesFields from './LocationPreferencesFields.svelte';

  let { profile, onProfileChanged }: { profile: UserProfile; onProfileChanged?: () => void } = $props();

  let busy = $state(false);
  let error = $state<string | null>(null);

  async function save(next: Parameters<typeof profileStore.updateLocation>[0]) {
    busy = true;
    error = null;
    try {
      await profileStore.updateLocation(next);
      onProfileChanged?.();
    } catch {
      error = 'Could not update your location. Try again.';
    } finally {
      busy = false;
    }
  }
</script>

<div class="flex flex-col gap-4 {busy ? 'pointer-events-none opacity-60' : ''}">
  <LocationPreferencesFields
    value={profile.location_preferences}
    derivedLocation={profile.derived_location}
    onChange={save}
  />
  {#if error}
    <p class="text-sm text-destructive">{error}</p>
  {/if}
</div>
