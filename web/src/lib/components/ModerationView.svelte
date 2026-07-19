<script lang="ts">
  import { page } from '$app/state';
  import { replaceState } from '$app/navigation';
  import { FileText, Flag, Handshake } from '@lucide/svelte';
  import type { LucideIcon } from '@lucide/svelte';
  import { api } from '$lib/api';
  import { AsyncData } from '$lib/asyncData.svelte';
  import { currentUser } from '$lib/auth.svelte';
  import type { Submission } from '$lib/types';
  import { Button } from '$lib/ui';
  import { cn, timeAgo } from '$lib/utils';
  import ReportQueue from './ReportQueue.svelte';
  import ReferralReviewView from './ReferralReviewView.svelte';
  import States from './States.svelte';

  const isModerator = $derived(currentUser()?.role === 'moderator');

  type View = 'queue' | 'reports' | 'referrals';
  const sections: { value: View; label: string; icon: LucideIcon }[] = [
    { value: 'queue', label: 'Moderation queue', icon: FileText },
    { value: 'reports', label: 'Reported jobs', icon: Flag },
    { value: 'referrals', label: 'Referral offers', icon: Handshake },
  ];

  // The active section is mirrored in `?tab=` so moderator deep-links (the
  // "Review offers →" link, a bookmark) land on the right pane and back/forward
  // works. `replaceState` swaps the URL without re-running load or scrolling.
  function readView(): View {
    const t = page.url.searchParams.get('tab');
    return sections.some((s) => s.value === t) ? (t as View) : 'queue';
  }
  let view = $state<View>(readView());
  function select(next: View) {
    if (next === view) return;
    view = next;
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- in-place query write to the current pathname; there is no route to resolve
    replaceState(`${page.url.pathname}?tab=${next}`, {});
  }

  // Shared nav-item treatment, mirroring the account sidebar in my/+layout.svelte.
  const itemClass = (active: boolean) =>
    cn(
      'flex items-center gap-2 rounded-md px-3 py-2 text-sm transition-colors',
      active
        ? 'bg-secondary font-medium text-secondary-foreground'
        : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground',
    );

  // The id currently being approved/rejected, to disable its row's buttons.
  let acting = $state<number | null>(null);
  let actionError = $state<string | null>(null);

  // Load once the moderator session is confirmed.
  const queueData = new AsyncData<Submission[]>([]);
  $effect(() => {
    if (isModerator) void queueData.run(() => api.listPendingSubmissions());
  });
  const status = $derived(queueData.status);
  const queue = $derived(queueData.value);

  function drop(id: number) {
    queueData.value = queueData.value.filter((s) => s.id !== id);
  }

  async function approve(s: Submission) {
    if (acting !== null) return;
    acting = s.id;
    actionError = null;
    try {
      await api.approveSubmission(s.id);
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
      await api.rejectSubmission(s.id, reason);
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
    <h1 class="text-2xl font-semibold tracking-tight">Moderation</h1>

    <!-- Same sections in two forms, mirroring the account shell: a horizontal
         scrollable strip below lg, a vertical sidebar beside the content at lg+. -->
    {#snippet navButtons(horizontal = false)}
      {#each sections as s (s.value)}
        {@const Icon = s.icon}
        <button
          type="button"
          aria-current={view === s.value ? 'page' : undefined}
          onclick={() => select(s.value)}
          class={cn(itemClass(view === s.value), horizontal && 'shrink-0 whitespace-nowrap')}
        >
          <Icon class="size-4 shrink-0" />
          {s.label}
        </button>
      {/each}
    {/snippet}

    <nav aria-label="Moderation sections" class="flex gap-1 overflow-x-auto lg:hidden">
      {@render navButtons(true)}
    </nav>

    <div class="lg:flex lg:gap-8">
      <aside aria-label="Moderation sections" class="hidden shrink-0 lg:block lg:w-56">
        <nav class="sticky top-6 flex flex-col gap-1">
          {@render navButtons()}
        </nav>
      </aside>

      <div class="min-w-0 flex-1">
    {#if view === 'queue'}
      <div class="flex flex-col gap-6">
        <p class="text-sm text-muted-foreground">
          Submissions awaiting review. Approving mints a live vacancy; rejecting records a reason.
        </p>

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
      </div>
    {:else if view === 'reports'}
      <ReportQueue />
    {:else}
      <div class="flex flex-col gap-6">
        <p class="text-sm text-muted-foreground">
          Applications to become a referrer. Approving lets seekers request referrals into that company.
        </p>
        <ReferralReviewView />
      </div>
    {/if}
      </div>
    </div>
  </div>
{/if}
