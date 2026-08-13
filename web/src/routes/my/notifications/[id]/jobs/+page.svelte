<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { ApiError, api } from '$lib/api';
  import States from '$lib/components/States.svelte';
  import type { NotificationItem } from '$lib/types';

  // The jobs a multi-job subscription digest actually matched, as a snapshot
  // taken at delivery time — not a live re-run of the saved search, which can
  // drift (a listing here may since have closed or dropped out of a fresh
  // search). Reached from a NotificationCard whose notificationTarget() is
  // `{kind: 'digest'}`; also a valid direct link/bookmark on its own, so it
  // fetches its own data rather than expecting router state.

  let status = $state<'loading' | 'ready' | 'error' | 'not-found'>('loading');
  let item = $state<NotificationItem | null>(null);

  onMount(async () => {
    const id = Number(page.params.id);
    if (!Number.isFinite(id)) {
      status = 'not-found';
      return;
    }
    try {
      item = await api.getNotification(id);
      status = 'ready';
    } catch (e) {
      status = e instanceof ApiError && e.status === 404 ? 'not-found' : 'error';
    }
  });
</script>

<svelte:head>
  <title>Matched jobs — freehire</title>
</svelte:head>

<div class="max-w-2xl">
  <a
    href={resolve('/my/notifications/history')}
    class="mb-4 inline-block text-sm text-muted-foreground underline underline-offset-2 hover:text-foreground"
  >
    ← Notification history
  </a>

  {#if status === 'loading'}
    <States state="loading" rows={4} />
  {:else if status === 'not-found'}
    <States state="empty" message="This notification doesn't exist, or isn't yours." />
  {:else if status === 'error'}
    <States state="error" />
  {:else if item}
    <h1 class="mb-1 text-lg font-semibold">{item.title}</h1>
    <p class="mb-4 text-sm text-muted-foreground">{item.body}</p>

    {#if item.jobs && item.jobs.length > 0}
      <ul class="overflow-hidden rounded-md border border-border">
        {#each item.jobs as job (job.slug)}
          <li class="border-b border-border last:border-b-0">
            <a
              href={resolve('/jobs/[slug]', { slug: job.slug })}
              class="block px-3 py-3 transition-colors hover:bg-accent/50"
            >
              <span class="block text-sm font-medium text-foreground">{job.title}</span>
              <span class="block text-xs text-muted-foreground">{job.company}</span>
            </a>
          </li>
        {/each}
      </ul>
    {:else}
      <States state="empty" message="No jobs recorded for this notification." />
    {/if}
  {/if}
</div>
