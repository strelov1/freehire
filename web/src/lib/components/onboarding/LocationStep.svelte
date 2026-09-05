<script lang="ts">
  // Where the candidate is and where they will work. The fields themselves are the shared
  // profile control, so the wizard and the profile page cannot disagree about what the
  // question is; only the framing around it is the wizard's.
  //
  // `derivedLocation` is what we WORKED OUT (from a CV or a LinkedIn address), not what the
  // candidate said. The control seeds an unstated base from it and lets a stated one win,
  // which is exactly the precedence this needs.
  import LocationPreferencesFields from '$lib/components/profile/LocationPreferencesFields.svelte';
  import type { DerivedLocation, LocationPreferences } from '$lib/types';

  interface Props {
    value: LocationPreferences | null;
    derivedLocation: DerivedLocation | null | undefined;
    onChange: (next: LocationPreferences | null) => void;
  }

  let { value, derivedLocation, onChange }: Props = $props();
</script>

<h2 class="text-xl font-semibold tracking-tight">Where are you based?</h2>
<p class="mt-1 text-sm text-muted-foreground">Also optional — helps us match you to the right jobs.</p>

<div class="mt-5">
  <LocationPreferencesFields {value} {derivedLocation} {onChange} />
</div>
