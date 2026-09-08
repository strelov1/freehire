<script lang="ts">
  import { api } from '$lib/api';
  import { browserTimezone } from '$lib/mentorship';
  import { errorMessage } from '$lib/utils';
  import { Badge, Button, Card } from '$lib/ui';
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
    };
  }

  function fromProfile(p: OwnMentorProfile): MentorProfileInput {
    return {
      company_slug: p.company_slug,
      slug: p.slug,
      name: p.name,
      headline: p.headline,
      bio: p.bio,
      topics: p.topics,
      languages: p.languages,
      timezone: p.timezone,
      session_minutes: p.session_minutes,
      // Read back rather than defaulted. The owner's read carries these precisely so an
      // edit re-submits what the mentor chose — a whole-object PUT that filled them from
      // defaults would silently reset the buffers of anyone who corrected their headline.
      buffer_before_minutes: p.buffer_before_minutes ?? 0,
      buffer_after_minutes: p.buffer_after_minutes ?? 0,
      notice_minutes: p.notice_minutes ?? 120,
      horizon_days: p.horizon_days ?? 30,
      meeting_url: p.meeting_url,
    };
  }

  let form = $state<MentorProfileInput>(profile ? fromProfile(profile) : blank());
  let topicsText = $state(profile ? profile.topics.join(', ') : '');
  let languagesText = $state(profile ? profile.languages.join(', ') : '');
  let saving = $state(false);
  let error = $state('');

  const list = (text: string) =>
    text
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);

  async function save() {
    saving = true;
    error = '';
    try {
      const body = { ...form, topics: list(topicsText), languages: list(languagesText) };
      profile = profile ? await api.updateMentorProfile(body) : await api.createMentorProfile(body);
    } catch (e) {
      error = errorMessage(e, 'The profile could not be saved.');
    } finally {
      saving = false;
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

  <div class="grid gap-3 sm:grid-cols-2">
    <label class="text-sm">
      <span class="text-muted-foreground">Your name, as seekers will see it</span>
      <input
        bind:value={form.name}
        class="border-input bg-background mt-1 w-full rounded-md border px-3 py-2 text-sm"
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
      <input
        bind:value={form.headline}
        class="border-input bg-background mt-1 w-full rounded-md border px-3 py-2 text-sm"
      />
    </label>

    <label class="text-sm">
      <span class="text-muted-foreground">Topics, comma separated</span>
      <input
        bind:value={topicsText}
        class="border-input bg-background mt-1 w-full rounded-md border px-3 py-2 text-sm"
      />
    </label>

    <label class="text-sm">
      <span class="text-muted-foreground">Languages, comma separated</span>
      <input
        bind:value={languagesText}
        class="border-input bg-background mt-1 w-full rounded-md border px-3 py-2 text-sm"
      />
    </label>

    <label class="text-sm">
      <span class="text-muted-foreground">Your timezone</span>
      <input
        bind:value={form.timezone}
        class="border-input bg-background mt-1 w-full rounded-md border px-3 py-2 text-sm"
      />
    </label>

    <label class="text-sm">
      <span class="text-muted-foreground">Session length, minutes</span>
      <input
        type="number"
        bind:value={form.session_minutes}
        class="border-input bg-background mt-1 w-full rounded-md border px-3 py-2 text-sm"
      />
    </label>

    <label class="text-sm sm:col-span-2">
      <span class="text-muted-foreground">
        Meeting link — a room you own. Only booked seekers ever see it.
      </span>
      <input
        bind:value={form.meeting_url}
        class="border-input bg-background mt-1 w-full rounded-md border px-3 py-2 text-sm"
      />
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

  {#if error}
    <p class="text-destructive text-sm">{error}</p>
  {/if}
</Card>
