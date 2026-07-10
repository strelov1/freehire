<script lang="ts">
  import { resolve } from '$app/paths';
  import { api } from '$lib/api';
  import { AsyncData } from '$lib/asyncData.svelte';
  import { isAuthenticated } from '$lib/auth.svelte';
  import type { Submission } from '$lib/types';
  import { timeAgo } from '$lib/utils';
  import States from './States.svelte';

  // status → a coloured pill. The three review states map to amber/green/red.
  const statusClass: Record<Submission['status'], string> = {
    pending: 'bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-300',
    approved: 'bg-green-100 text-green-800 dark:bg-green-900/40 dark:text-green-300',
    rejected: 'bg-red-100 text-red-800 dark:bg-red-900/40 dark:text-red-300',
  };

  // Load once the session is confirmed (the boot-time /me resolution may still be in
  // flight when the page is opened directly).
  const submissionsData = new AsyncData<Submission[]>([]);
  $effect(() => {
    if (isAuthenticated()) void submissionsData.run(() => api.listMySubmissions());
  });
  const status = $derived(submissionsData.status);
  const submissions = $derived(submissionsData.value);
</script>

{#if !isAuthenticated()}
  <p class="py-12 text-center text-sm text-muted-foreground">Sign in to see your submissions.</p>
{:else}
  <div class="flex flex-col gap-6">
    <div class="flex flex-col gap-1">
      <h1 class="text-2xl font-semibold tracking-tight">My submissions</h1>
      <p class="text-sm text-muted-foreground">
        Jobs you submitted for review. <a href={resolve('/submit')} class="underline">Submit another</a>.
      </p>
    </div>

    {#if status === 'loading'}
      <States state="loading" />
    {:else if status === 'error'}
      <States state="error" message="Couldn't load your submissions." />
    {:else if submissions.length === 0}
      <States state="empty" message="No submissions yet. Submit a job to see it here." />
    {:else}
      <ul class="flex flex-col divide-y divide-border rounded-lg border border-border">
        {#each submissions as s (s.id)}
          <li class="flex items-start justify-between gap-3 px-4 py-3">
            <div class="flex min-w-0 flex-col gap-0.5">
              <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external posting URL, opened in a new tab; not an internal route -->
              <a href={s.url}
                target="_blank"
                rel="noopener noreferrer"
                class="truncate text-sm font-medium hover:underline"
              >
                {s.title}
              </a>
              <span class="truncate text-xs text-muted-foreground">
                {s.company}{s.location ? ` · ${s.location}` : ''} · submitted {timeAgo(s.created_at)}
              </span>
              {#if s.status === 'rejected' && s.review_reason}
                <span class="text-xs text-destructive">Reason: {s.review_reason}</span>
              {/if}
              {#if s.status === 'approved' && s.job_slug}
                <a
                  href={resolve('/jobs/[slug]', { slug: s.job_slug })}
                  class="text-xs font-medium text-foreground hover:underline"
                >
                  View vacancy →
                </a>
              {/if}
            </div>
            <span class="rounded-md px-2 py-0.5 text-xs font-medium {statusClass[s.status]}">
              {s.status}
            </span>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
{/if}
