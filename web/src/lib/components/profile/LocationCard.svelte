<script lang="ts">
  // "Where & how I want to work" as its own view — work format, base country/city, remote
  // reach, relocation — autosaving on every change straight to profileStore, the same way
  // the Roles card and the Skills view do.
  import { profileStore } from '#lib/profile.svelte.js';
  import type { UserProfile } from '#lib/types.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { messages } from './LocationCard.messages';
  import LocationPreferencesFields from './LocationPreferencesFields.svelte';

  let { profile, onProfileChanged }: { profile: UserProfile; onProfileChanged?: () => void } = $props();

  const s = $derived(t(messages, locale()));

  let busy = $state(false);
  // A flag, not the message itself — the message must stay derived from `s` so it
  // follows a later locale change (e.g. the account-language card next to this one)
  // instead of freezing in whatever locale was resolved when the save failed.
  let hasError = $state(false);

  async function save(next: Parameters<typeof profileStore.updateLocation>[0]) {
    busy = true;
    hasError = false;
    try {
      await profileStore.updateLocation(next);
      onProfileChanged?.();
    } catch {
      hasError = true;
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
  {#if hasError}
    <p class="text-sm text-destructive">{s.saveError}</p>
  {/if}
</div>
