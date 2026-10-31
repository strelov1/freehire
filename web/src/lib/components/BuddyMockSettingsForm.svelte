<script lang="ts">
  import { Check, Info } from '@lucide/svelte';
  import { api } from '#lib/api.js';
  import { errorMessage } from '#lib/utils.js';
  import { Badge, Button, Card, Input, cn } from '#lib/ui/index.js';
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
  // Before the first save there is nothing to differ from, so the form is always savable.
  const dirty = $derived(
    !settings ||
      settings.timezone !== timezone ||
      settings.meeting_url !== meetingUrl.trim() ||
      settings.paused !== paused ||
      settings.topics.join() !== topics.join(),
  );
  const status = $derived(!settings ? 'Not set up' : settings.paused ? 'Paused' : 'Taking bookings');
  const uid = $props.id();

  function toggle(topic: BuddyMockTopic) {
    const on = !topics.includes(topic);
    topics = MOCK_TOPICS.filter((t) => (t === topic ? on : topics.includes(t)));
    saved = false;
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

<Card class="flex flex-col p-0">
  <div class="flex flex-col gap-4 p-5">
    <div class="flex items-start justify-between gap-3">
      <div class="flex flex-col gap-0.5">
        <h2 class="text-base font-semibold">Hosting settings</h2>
        <p class="text-sm text-muted-foreground">Where you meet guests and what you practise with them.</p>
      </div>
      <Badge variant={status === 'Taking bookings' ? 'brand' : 'outline'} class="shrink-0">{status}</Badge>
    </div>

    <div class="grid gap-4 sm:grid-cols-2">
      <div class="flex flex-col gap-1.5">
        <label for="{uid}-zone" class="text-sm font-medium">Time zone</label>
        <Input id="{uid}-zone" bind:value={timezone} class="w-full" placeholder="Europe/Berlin" />
        <p class="text-xs text-muted-foreground">Your weekly hours below are in this zone.</p>
      </div>
      <div class="flex flex-col gap-1.5">
        <label for="{uid}-link" class="text-sm font-medium">Meeting link</label>
        <Input
          id="{uid}-link"
          type="url"
          bind:value={meetingUrl}
          class="w-full"
          placeholder="https://meet.google.com/…"
          aria-invalid={meetingUrl !== '' && !linkOk}
          aria-describedby="{uid}-link-note"
        />
        {#if meetingUrl && !linkOk}
          <p class="text-xs text-destructive">The link must start with https://</p>
        {/if}
      </div>
    </div>

    <p id="{uid}-link-note" class="flex items-start gap-2 rounded-lg bg-muted/60 px-3 py-2 text-xs text-muted-foreground">
      <Info class="mt-px size-3.5 shrink-0" aria-hidden="true" />
      <span>
        Your guest may see your name through this link — for example the name on your Zoom or
        Google account. Only the member who books you sees the link, and only after booking.
      </span>
    </p>

    <fieldset class="flex flex-col gap-2">
      <legend class="mb-2 text-sm font-medium">Topics you host</legend>
      <div class="flex flex-wrap gap-2">
        {#each MOCK_TOPICS as topic (topic)}
          {@const on = topics.includes(topic)}
          <button
            type="button"
            aria-pressed={on}
            onclick={() => toggle(topic)}
            class={cn(
              'inline-flex h-8 items-center gap-1.5 rounded-full border px-3 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
              on
                ? 'border-transparent bg-brand-muted text-brand-strong'
                : 'border-border text-muted-foreground hover:bg-muted hover:text-foreground',
            )}
          >
            {#if on}<Check class="size-3.5" aria-hidden="true" />{/if}
            {mockTopicLabel(topic)}
          </button>
        {/each}
      </div>
      {#if topics.length === 0}<p class="text-xs text-destructive">Choose at least one topic.</p>{/if}
    </fieldset>

    <label class="flex items-start justify-between gap-4 border-t border-border pt-3">
      <span class="flex flex-col gap-0.5">
        <span class="text-sm font-medium">Pause hosting</span>
        <span class="text-xs text-muted-foreground">Members cannot book you; booked mock interviews stay.</span>
      </span>
      <input type="checkbox" class="mt-0.5 size-4 shrink-0 accent-brand" bind:checked={paused} onchange={() => (saved = false)} />
    </label>
  </div>

  <div class="flex flex-wrap items-center gap-3 border-t border-border bg-muted/40 px-5 py-3">
    <Button variant="primary" size="sm" disabled={saving || !dirty || !linkOk || topics.length === 0} onclick={save}>
      {saving ? 'Saving…' : 'Save settings'}
    </Button>
    {#if saved && !dirty}<span class="text-sm text-muted-foreground" role="status">Saved.</span>
    {:else if dirty && settings}<span class="text-sm text-muted-foreground">Unsaved changes</span>{/if}
  </div>
  {#if error}<p role="alert" class="border-t border-border px-5 py-3 text-sm text-destructive">{error}</p>{/if}
</Card>
