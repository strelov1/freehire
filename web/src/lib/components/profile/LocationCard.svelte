<script lang="ts">
  // "Where & how I want to work" as its own view — work format, base country/city, remote
  // reach, relocation — autosaving on every change straight to profileStore, the same way
  // the Roles card and the Skills view do. The save state belongs to the caller, which shows
  // it on the section heading and keeps it across a remount of this card.
  import { profileStore } from '#lib/profile.svelte.js';
  import type { SaveStatus } from '#lib/saveStatus.svelte.js';
  import type { UserProfile } from '#lib/types.js';
  import LocationPreferencesFields from './LocationPreferencesFields.svelte';

  let {
    profile,
    status,
    onProfileChanged,
  }: { profile: UserProfile; status: SaveStatus; onProfileChanged?: () => void } = $props();

  function save(next: Parameters<typeof profileStore.updateLocation>[0]) {
    void status.run(async () => {
      await profileStore.updateLocation(next);
      onProfileChanged?.();
    });
  }
</script>

<div class={status.saving ? 'pointer-events-none opacity-60' : ''}>
  <LocationPreferencesFields
    value={profile.location_preferences}
    derivedLocation={profile.derived_location}
    onChange={save}
  />
</div>
