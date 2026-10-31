<script lang="ts">
  import { api } from '#lib/api.js';
  import { errorMessage } from '#lib/utils.js';
  import { Button, Card, Input } from '#lib/ui/index.js';
  import { browserTimezone } from '#lib/mentorship.js';
  import { MOCK_TOPICS, isHttpsLink, mockTopicLabel } from '#lib/buddyMocks.js';
  import type { BuddyMockSettings, BuddyMockTopic } from '#lib/types.js';

  let { settings = $bindable() }: { settings: BuddyMockSettings | null } = $props();

  // The form edits a copy and writes back only what the server stored.
  let timezone = $state(settings?.timezone ?? browserTimezone());
  let meetingUrl = $state(settings?.meeting_url ?? '');
  let topics = $state<BuddyMockTopic[]>(settings ? [...settings.topics] : [...MOCK_TOPICS]);
  let paused = $state(settings?.paused ?? false);
  let saving = $state(false);
  let saved = $state(false);
  let error = $state('');

  const linkOk = $derived(isHttpsLink(meetingUrl));

  function toggle(topic: BuddyMockTopic, on: boolean) {
    topics = MOCK_TOPICS.filter((t) => (t === topic ? on : topics.includes(t)));
  }

  async function save() {
    saving = true;
    saved = false;
    error = '';
    try {
      settings = await api.saveBuddyMockSettings({ timezone, meeting_url: meetingUrl.trim(), topics, paused });
      saved = true;
    } catch (e) {
      error = errorMessage(e, 'Your mock interview settings could not be saved.');
    } finally {
      saving = false;
    }
  }
</script>

<Card class="flex flex-col gap-4 p-4">
  <h2 class="text-sm font-medium">Hosting mock interviews</h2>

  <label class="flex flex-col gap-1 text-sm">
    <span class="text-muted-foreground">Your time zone</span>
    <Input bind:value={timezone} class="w-full" placeholder="Europe/Berlin" />
  </label>

  <label class="flex flex-col gap-1 text-sm">
    <span class="text-muted-foreground">Your meeting link</span>
    <Input type="url" bind:value={meetingUrl} class="w-full" placeholder="https://meet.google.com/…" />
    <span class="text-muted-foreground text-xs">
      Your guest may see your name through this link — for example the name on your Zoom or
      Google account. Only the member who books you sees the link, and only after booking.
    </span>
    {#if meetingUrl && !linkOk}
      <span class="text-destructive text-xs">The link must start with https://</span>
    {/if}
  </label>

  <fieldset class="flex flex-col gap-1 text-sm">
    <legend class="text-muted-foreground mb-1">Topics you host</legend>
    {#each MOCK_TOPICS as topic (topic)}
      <label class="flex items-center gap-2">
        <input
          type="checkbox"
          checked={topics.includes(topic)}
          onchange={(e) => toggle(topic, e.currentTarget.checked)}
        />
        {mockTopicLabel(topic)}
      </label>
    {/each}
  </fieldset>

  <label class="flex items-center gap-2 text-sm">
    <input type="checkbox" bind:checked={paused} />
    Pause hosting — members cannot book you; booked mock interviews stay
  </label>

  <div class="flex items-center gap-3">
    <Button variant="primary" size="sm" disabled={saving || !linkOk || topics.length === 0} onclick={save}>
      {saving ? 'Saving…' : 'Save'}
    </Button>
    {#if saved}<span class="text-muted-foreground text-sm">Saved.</span>{/if}
  </div>
  {#if error}<p class="text-destructive text-sm">{error}</p>{/if}
</Card>
