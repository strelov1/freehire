<script lang="ts">
  import { approveSubmission, listPendingSubmissions, rejectSubmission } from '$lib/api';
  import { currentUser } from '$lib/auth.svelte';
  import type { Submission } from '$lib/types';
  import { Button } from '$lib/ui';
  import { timeAgo } from '$lib/utils';
  import ReportQueue from './ReportQueue.svelte';
  import States from './States.svelte';

  const isModerator = $derived(currentUser()?.role === 'moderator');

  let status = $state<'loading' | 'error' | 'ready'>('loading');
  let queue = $state.raw<Submission[]>([]);
  // The id currently being approved/rejected, to disable its row's buttons.
  let acting = $state<number | null>(null);
  let actionError = $state<string | null>(null);

  async function load() {
    status = 'loading';
    try {
      queue = await listPendingSubmissions();
      status = 'ready';
    } catch {
      status = 'error';
    }
  }

  // Load once the moderator session is confirmed.
  $effect(() => {
    if (isModerator) void load();
  });

  function drop(id: number) {
    queue = queue.filter((s) => s.id !== id);
  }

  async function approve(s: Submission) {
    if (acting !== null) return;
    acting = s.id;
    actionError = null;
    try {
      await approveSubmission(s.id);
      drop(s.id);
    } catch {
      actionError = `Could not approve "${s.title}". It may have already been decided.`;
    } finally {
      acting = null;
    }
  }

  async function reject(s: Submission) {
    if (acting !== null) return;
    const reason = window.prompt(`Reject "${s.title}"? Optional reason:`, '');
    // A null return means the moderator cancelled the prompt; an empty string is a
    // reasonless rejection, which is allowed.
    if (reason === null) return;
    acting = s.id;
    actionError = null;
    try {
      await rejectSubmission(s.id, reason);
      drop(s.id);
    } catch {
      actionError = `Could not reject "${s.title}". It may have already been decided.`;
    } finally {
      acting = null;
    }
  }
</script>

{#if !isModerator}
  <p class="py-12 text-center text-sm text-muted-foreground">
    This page is for moderators only.
  </p>
{:else}
  <div class="flex flex-col gap-6">
    <div class="flex flex-col gap-1">
      <h1 class="text-2xl font-semibold tracking-tight">Moderation queue</h1>
      <p class="text-sm text-muted-foreground">
        Submissions awaiting review. Approving mints a live vacancy; rejecting records a reason.
      </p>
    </div>

    {#if actionError}
      <p class="text-sm text-destructive">{actionError}</p>
    {/if}

    {#if status === 'loading'}
      <States state="loading" />
    {:else if status === 'error'}
      <States state="error" message="Couldn't load the queue." />
    {:else if queue.length === 0}
      <States state="empty" message="Nothing to review — the queue is empty." />
    {:else}
      <ul class="flex flex-col divide-y divide-border rounded-lg border border-border">
        {#each queue as s (s.id)}
          <li class="flex flex-col gap-3 px-4 py-3 sm:flex-row sm:items-start sm:justify-between">
            <div class="flex min-w-0 flex-col gap-0.5">
              <a
                href={s.url}
                target="_blank"
                rel="noopener noreferrer"
                class="truncate text-sm font-medium hover:underline"
              >
                {s.title}
              </a>
              <span class="truncate text-xs text-muted-foreground">
                {s.company}{s.location ? ` · ${s.location}` : ''}{s.remote ? ' · remote' : ''}
              </span>
              <span class="truncate text-xs text-muted-foreground">
                by {s.submitter_email ?? 'unknown'} · {timeAgo(s.created_at)}
              </span>
            </div>
            <div class="flex shrink-0 gap-2">
              <Button variant="primary" size="sm" disabled={acting !== null} onclick={() => approve(s)}>
                Approve
              </Button>
              <Button variant="ghost" size="sm" disabled={acting !== null} onclick={() => reject(s)}>
                Reject
              </Button>
            </div>
          </li>
        {/each}
      </ul>
    {/if}

    <div class="border-t border-border pt-6">
      <ReportQueue />
    </div>
  </div>
{/if}
