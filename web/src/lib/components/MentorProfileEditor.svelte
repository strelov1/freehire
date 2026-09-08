<script lang="ts">
  import { onMount } from 'svelte';
  import { resolve } from '$app/paths';
  import { api } from '$lib/api';
  import { browserTimezone, profileInputFromProfile, seedFormFromSuggestions } from '$lib/mentorship';
  import { errorMessage } from '$lib/utils';
  import { Badge, Button, Card, Input } from '$lib/ui';
  import TokenInput from '$lib/components/facets/TokenInput.svelte';
  import type { MentorProfileInput, OwnMentorProfile } from '$lib/types';

  let { profile = $bindable() }: { profile: OwnMentorProfile | null } = $props();

  // The browser's zone as the default for a NEW profile only. An existing one keeps what
  // the mentor chose: their availability is resolved through it, and silently re-reading
  // it from the machine they happen to be on would move every stated hour.
  const browserZone = browserTimezone();

  function blank(): MentorProfileInput {
    return {
      company_slug: '',
      slug: '',
      name: '',
      headline: '',
      bio: '',
      topics: [],
      languages: [],
      timezone: browserZone,
      session_minutes: 60,
      buffer_before_minutes: 0,
      buffer_after_minutes: 15,
      notice_minutes: 120,
      horizon_days: 30,
      meeting_url: '',
      show_photo: false,
    };
  }

  let form = $state<MentorProfileInput>(profile ? profileInputFromProfile(profile) : blank());
  let saving = $state(false);
  let error = $state('');

  // A one-time prefill for a brand-new profile only: fetched once on mount, seeded into
  // the still-blank form, and never touched again — every field stays an ordinary,
  // independently editable input from here on. A failed fetch simply leaves the form at
  // its ordinary blank defaults; it must never block rendering the form.
  if (!profile) {
    onMount(async () => {
      try {
        const suggestions = await api.mentorProfileSuggestions();
        form = seedFormFromSuggestions(form, suggestions);
      } catch {
        // Best-effort: the form already has its ordinary blank defaults.
      }
    });
  }

  // Chip helpers for the two open-vocabulary lists. A duplicate (exact string match) is
  // simply not added again rather than shown twice — the backend also dedupes on save,
  // but a chip that visibly repeats itself while typing reads as broken.
  function addTopic(value: string) {
    const v = value.trim();
    if (v && !form.topics.includes(v)) form.topics = [...form.topics, v];
  }
  function removeTopic(value: string) {
    form.topics = form.topics.filter((t) => t !== value);
  }
  function addLanguage(value: string) {
    const v = value.trim();
    if (v && !form.languages.includes(v)) form.languages = [...form.languages, v];
  }
  function removeLanguage(value: string) {
    form.languages = form.languages.filter((l) => l !== value);
  }

  async function save() {
    saving = true;
    error = '';
    try {
      profile = profile ? await api.updateMentorProfile(form) : await api.createMentorProfile(form);
    } catch (e) {
      error = errorMessage(e, 'The profile could not be saved.');
    } finally {
      saving = false;
    }
  }

  // Withdrawal is destructive in a way pausing is not: it cancels every confirmed future
  // booking and tells each seeker. So it asks first, and says what it will do.
  let withdrawing = $state(false);
  let confirmingWithdrawal = $state(false);

  async function withdraw() {
    withdrawing = true;
    error = '';
    try {
      await api.withdrawMentorProfile();
      // The profile is MARKED withdrawn, not deleted — past sessions and their reviews
      // hang off the row. Dropping it from local state simply returns this screen to the
      // "offer to mentor" state.
      profile = null;
      confirmingWithdrawal = false;
    } catch (e) {
      error = errorMessage(e, 'The profile could not be withdrawn.');
    } finally {
      withdrawing = false;
    }
  }

  async function togglePause() {
    if (!profile) return;
    saving = true;
    error = '';
    try {
      profile = await api.pauseMentorProfile(!profile.paused);
    } catch (e) {
      error = errorMessage(e, 'That could not be changed.');
    } finally {
      saving = false;
    }
  }
</script>

<Card class="flex flex-col gap-4 p-4">
  <div class="flex flex-wrap items-center justify-between gap-2">
    <h2 class="text-sm font-medium">Your mentor profile</h2>
    {#if profile}
      <div class="flex items-center gap-2">
        <Badge variant={profile.status === 'approved' ? 'secondary' : 'outline'}>
          {profile.paused ? 'paused' : profile.status}
        </Badge>
        {#if profile.status === 'approved'}
          <!-- Pausing needs no moderator, and confirmed bookings stand through it. -->
          <Button variant="ghost" size="sm" disabled={saving} onclick={togglePause}>
            {profile.paused ? 'Resume' : 'Pause'}
          </Button>
        {/if}
      </div>
    {/if}
  </div>

  {#if profile && profile.status === 'pending'}
    <p class="text-muted-foreground text-sm">
      A moderator reviews new profiles by hand. Yours is not in the directory yet.
    </p>
  {/if}

  {#if profile}
    <p class="text-muted-foreground text-sm">
      Your timezone and session length live on the <a class="underline" href={resolve('/my/mentorship/schedule')}>schedule page</a> now, alongside when you're free.
    </p>
  {/if}

  <div class="grid gap-3 sm:grid-cols-2">
    <label class="text-sm">
      <span class="text-muted-foreground">Your name, as seekers will see it</span>
      <Input
        bind:value={form.name}
        class="mt-1 w-full"
      />
    </label>

    <label class="text-sm">
      <span class="text-muted-foreground">Company slug</span>
      <input
        bind:value={form.company_slug}
        disabled={Boolean(profile)}
        class="border-input bg-background mt-1 w-full rounded-md border px-3 py-2 text-sm disabled:opacity-60"
      />
    </label>

    <label class="text-sm">
      <span class="text-muted-foreground">Your URL</span>
      <input
        bind:value={form.slug}
        disabled={Boolean(profile)}
        class="border-input bg-background mt-1 w-full rounded-md border px-3 py-2 text-sm disabled:opacity-60"
      />
    </label>

    <label class="text-sm">
      <span class="text-muted-foreground">Headline</span>
      <Input
        bind:value={form.headline}
        class="mt-1 w-full"
      />
    </label>

    <label class="text-sm">
      <span class="text-muted-foreground">Topics</span>
      <div class="mt-1">
        <TokenInput
          tokens={form.topics}
          onAdd={addTopic}
          onRemove={removeTopic}
          placeholder="Type a topic, press Enter"
        />
      </div>
    </label>

    <label class="text-sm">
      <span class="text-muted-foreground">Languages</span>
      <div class="mt-1">
        <TokenInput
          tokens={form.languages}
          onAdd={addLanguage}
          onRemove={removeLanguage}
          placeholder="Type a language, press Enter"
        />
      </div>
    </label>

    <label class="text-sm sm:col-span-2">
      <span class="text-muted-foreground">
        Meeting link — a room you own. Only booked seekers ever see it.
      </span>
      <Input
        bind:value={form.meeting_url}
        class="mt-1 w-full"
      />
    </label>

    <label class="flex items-center gap-2 text-sm sm:col-span-2">
      <input type="checkbox" bind:checked={form.show_photo} class="h-4 w-4" />
      <span class="text-muted-foreground">
        Show my account's CV photo on my public mentor card and profile. Off by default —
        you decide whether that photo belongs here too.
      </span>
    </label>

    <label class="text-sm sm:col-span-2">
      <span class="text-muted-foreground">About you</span>
      <textarea
        bind:value={form.bio}
        rows="4"
        class="border-input bg-background mt-1 w-full rounded-md border px-3 py-2 text-sm"
      ></textarea>
    </label>
  </div>

  <div>
    <Button disabled={saving} onclick={save}>
      {saving ? 'Saving…' : profile ? 'Save changes' : 'Submit for review'}
    </Button>
  </div>

  {#if profile}
    <div class="border-t pt-4">
      {#if confirmingWithdrawal}
        <p class="text-sm font-medium">Withdraw your profile?</p>
        <p class="text-muted-foreground mt-1 text-sm">
          Every confirmed session still ahead is cancelled and each person told. Sessions
          that already happened stay in your history.
        </p>
        <div class="mt-3 flex gap-2">
          <Button variant="destructive" disabled={withdrawing} onclick={withdraw}>
            {withdrawing ? 'Withdrawing…' : 'Withdraw'}
          </Button>
          <Button
            variant="ghost"
            disabled={withdrawing}
            onclick={() => (confirmingWithdrawal = false)}
          >
            Keep it
          </Button>
        </div>
      {:else}
        <Button variant="ghost" size="sm" onclick={() => (confirmingWithdrawal = true)}>
          Withdraw profile
        </Button>
      {/if}
    </div>
  {/if}

  {#if error}
    <p class="text-destructive text-sm">{error}</p>
  {/if}
</Card>
