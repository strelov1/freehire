<script lang="ts">
  // Timezone and session length live here, beside the availability they resolve against,
  // rather than on the profile form — a mentor tuning when they're free tunes both at
  // once. Saved through the same whole-object PUT the profile form uses:
  // `profileInputFromProfile` reads every other field back from the stored profile so
  // this save can never silently reset something this screen does not show.
  import { api } from '$lib/api';
  import { profileInputFromProfile } from '$lib/mentorship';
  import { errorMessage } from '$lib/utils';
  import { Button, Card, Input } from '$lib/ui';
  import type { OwnMentorProfile } from '$lib/types';

  let { profile = $bindable() }: { profile: OwnMentorProfile } = $props();

  let timezone = $state(profile.timezone);
  let sessionMinutes = $state(profile.session_minutes);
  let saving = $state(false);
  let error = $state('');

  async function save() {
    saving = true;
    error = '';
    try {
      profile = await api.updateMentorProfile({
        ...profileInputFromProfile(profile),
        timezone,
        session_minutes: sessionMinutes,
      });
    } catch (e) {
      error = errorMessage(e, 'Could not save your session settings.');
    } finally {
      saving = false;
    }
  }
</script>

<Card class="flex flex-col gap-4 p-4">
  <h2 class="text-sm font-medium">Session settings</h2>
  <div class="grid gap-3 sm:grid-cols-2">
    <label class="text-sm">
      <span class="text-muted-foreground">Your timezone</span>
      <Input bind:value={timezone} class="mt-1 w-full" />
    </label>

    <label class="text-sm">
      <span class="text-muted-foreground">Session length, minutes</span>
      <input
        type="number"
        bind:value={sessionMinutes}
        class="border-input bg-background mt-1 w-full rounded-md border px-3 py-2 text-sm"
      />
    </label>
  </div>

  <div>
    <Button disabled={saving} onclick={save}>
      {saving ? 'Saving…' : 'Save'}
    </Button>
  </div>

  {#if error}
    <p class="text-destructive text-sm">{error}</p>
  {/if}
</Card>
