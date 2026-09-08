<script lang="ts">
  import { api } from '$lib/api';
  import { errorMessage } from '$lib/utils';
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
            <div>
              <p class="font-medium">{profile.name}</p>
              <p class="text-muted-foreground text-sm">
                {profile.headline} · {profile.company_name} ({profile.company_slug})
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
