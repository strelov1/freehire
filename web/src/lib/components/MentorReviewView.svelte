<script lang="ts">
  import { api } from '$lib/api';
  import { errorMessage, formatDate } from '$lib/utils';
  import { AsyncData } from '$lib/asyncData.svelte';
  import { Badge, Button, Card } from '$lib/ui';
  import type { PendingMentorProfile } from '$lib/types';
  import States from './States.svelte';

  const queueData = new AsyncData<PendingMentorProfile[]>([]);
  $effect(() => {
    void queueData.run(() => api.listPendingMentorProfiles());
  });

  const status = $derived(queueData.status);
  const queue = $derived(queueData.value);

  // The row currently being decided, so its own buttons disable rather than the whole list.
  let acting = $state<number | null>(null);
  let error = $state<string | null>(null);

  // Which cards have their public-preview expanded. A set of ids rather than one flag,
  // so opening one profile's preview does not close another's.
  let previewing = $state<Set<number>>(new Set());
  // A photo load failure (unopted-in, or genuinely missing) hides the image rather than
  // showing a broken-image icon — the same rule the public card follows.
  let photoFailed = $state<Set<number>>(new Set());

  function togglePreview(id: number) {
    const next = new Set(previewing);
    if (next.has(id)) {
      next.delete(id);
    } else {
      next.add(id);
    }
    previewing = next;
  }

  async function decide(profile: PendingMentorProfile, next: 'approved' | 'rejected') {
    acting = profile.id;
    error = null;
    try {
      await api.decideMentorProfile(profile.id, next);
      queueData.value = queue.filter((p) => p.id !== profile.id);
    } catch (e) {
      error = errorMessage(e, 'That decision could not be recorded.');
    } finally {
      acting = null;
    }
  }
</script>

<div class="flex flex-col gap-4">
  <div>
    <h2 class="text-sm font-medium">Mentor profiles</h2>
    <p class="text-muted-foreground mt-1 text-sm">
      Oldest first. Approving one publishes it; nothing else does.
    </p>
  </div>

  {#if status === 'loading'}
    <States state="loading" />
  {:else if status === 'error'}
    <States state="error" message="Couldn't load the mentor queue." />
  {:else if queue.length === 0}
    <States state="empty" message="Nothing to review — no profiles are waiting." />
  {:else}
    <ul class="flex flex-col gap-3">
      {#each queue as profile (profile.id)}
        <li>
          <Card class="flex flex-col gap-3 p-4">
            <div class="flex flex-wrap items-baseline justify-between gap-2">
              <div>
                <p class="font-medium">{profile.name}</p>
                <p class="text-muted-foreground text-sm">
                  {profile.headline} · {profile.company_name} ({profile.company_slug})
                </p>
              </div>
              <p class="text-muted-foreground shrink-0 text-xs">
                Submitted {formatDate(profile.created_at)}
              </p>
            </div>

            {#if profile.bio}
              <p class="text-sm whitespace-pre-line">{profile.bio}</p>
            {/if}

            <div class="flex flex-wrap gap-1">
              {#each profile.topics as topic (topic)}
                <Badge variant="secondary">{topic}</Badge>
              {/each}
              {#each profile.languages as language (language)}
                <Badge variant="outline">{language}</Badge>
              {/each}
            </div>

            <!-- Corroboration, never a gate: this account is already an approved referrer
                 for the same company. The spec is explicit that an approved offer does not
                 approve a mentor profile, so this is shown and nothing acts on it. -->
            {#if profile.has_approved_referral_offer}
              <p class="text-muted-foreground text-sm">
                Already an approved referrer for this company.
              </p>
            {/if}

            <div>
              <Button variant="ghost" size="sm" onclick={() => togglePreview(profile.id)}>
                {previewing.has(profile.id) ? 'Hide preview' : 'Preview'}
              </Button>
            </div>

            <!-- What a visitor will see once this is approved — built from data already in
                 hand, plus the one thing that isn't: the opted-in photo, served through a
                 moderator-only route so a pending profile's picture never has to pass
                 through the public, slug-keyed one. -->
            {#if previewing.has(profile.id)}
              <div class="bg-muted/40 flex items-start gap-3 rounded-md border p-3">
                {#if profile.show_photo && !photoFailed.has(profile.id)}
                  <img
                    src={`/api/v1/mentorship/profiles/${profile.id}/photo`}
                    alt=""
                    class="h-16 w-16 shrink-0 rounded-full object-cover"
                    onerror={() => (photoFailed = new Set(photoFailed).add(profile.id))}
                  />
                {/if}
                <div class="flex flex-col gap-2">
                  <div>
                    <p class="font-medium">{profile.name}</p>
                    <p class="text-muted-foreground text-sm">
                      {profile.headline} · {profile.company_name}
                    </p>
                  </div>
                  <div class="flex flex-wrap gap-1">
                    {#each profile.topics as topic (topic)}
                      <Badge variant="secondary">{topic}</Badge>
                    {/each}
                    {#each profile.languages as language (language)}
                      <Badge variant="outline">{language}</Badge>
                    {/each}
                  </div>
                  {#if profile.bio}
                    <p class="text-sm whitespace-pre-line">{profile.bio}</p>
                  {/if}
                </div>
              </div>
            {/if}

            <div class="flex gap-2">
              <Button
                size="sm"
                disabled={acting === profile.id}
                onclick={() => decide(profile, 'approved')}
              >
                Approve
              </Button>
              <Button
                size="sm"
                variant="ghost"
                disabled={acting === profile.id}
                onclick={() => decide(profile, 'rejected')}
              >
                Reject
              </Button>
            </div>
          </Card>
        </li>
      {/each}
    </ul>
  {/if}

  {#if error}
    <p class="text-destructive text-sm">{error}</p>
  {/if}
</div>
