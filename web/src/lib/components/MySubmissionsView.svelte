<script lang="ts">
  import { resolve } from '$app/paths';
  import { api } from '#lib/api.js';
  import { AsyncData } from '#lib/asyncData.svelte.js';
  import { isAuthenticated } from '#lib/auth.svelte.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t, tokenLabel } from '#lib/i18n/t.js';
  import type { Submission } from '#lib/types.js';
  import { timeAgo } from '#lib/utils.js';
  import { messages } from './MySubmissionsView.messages';
  import States from './States.svelte';

  const s = $derived(t(messages, locale()));

  // status → a coloured pill. The three review states map to warning/green/red.
  const statusClass: Record<Submission['status'], string> = {
    pending: 'bg-warning-muted text-warning-strong bg-warning-muted/40 text-warning-strong',
    approved: 'bg-success-muted text-success-strong',
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
  <p class="py-12 text-center text-sm text-muted-foreground">{s.signedOut}</p>
{:else}
  <div class="flex flex-col gap-6">
    <div class="flex flex-col gap-1">
      <h1 class="text-2xl font-semibold tracking-tight">{s.title}</h1>
      <p class="text-sm text-muted-foreground">
        {s.descriptionPrefix}
        <a href={resolve('/submit')} class="underline">{s.submitAnother}</a>.
      </p>
    </div>

    {#if status === 'loading'}
      <States state="loading" />
    {:else if status === 'error'}
      <States state="error" message={s.loadError} />
    {:else if submissions.length === 0}
      <States state="empty" message={s.empty} />
    {:else}
      <ul class="flex flex-col divide-y divide-border rounded-lg border border-border">
        {#each submissions as sub (sub.id)}
          <li class="flex items-start justify-between gap-3 px-4 py-3">
            <div class="flex min-w-0 flex-col gap-0.5">
              <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external posting URL, opened in a new tab; not an internal route -->
              <a href={sub.url}
                target="_blank"
                rel="noopener noreferrer"
                class="truncate text-sm font-medium hover:underline"
              >
                {sub.title}
              </a>
              <span class="truncate text-xs text-muted-foreground">
                {sub.company}{sub.location ? ` · ${sub.location}` : ''} · {s.submittedPrefix}
                {timeAgo(sub.created_at, locale())}
              </span>
              {#if sub.status === 'rejected' && sub.review_reason}
                <span class="text-xs text-destructive">{s.rejectionReasonPrefix} {sub.review_reason}</span>
              {/if}
              {#if sub.status === 'approved' && sub.job_slug}
                <a
                  href={resolve('/jobs/[slug]', { slug: sub.job_slug })}
                  class="text-xs font-medium text-foreground hover:underline"
                >
                  {s.viewVacancy}
                </a>
              {/if}
            </div>
            <span class="rounded-md px-2 py-0.5 text-xs font-medium {statusClass[sub.status]}">
              {tokenLabel(s.status, sub.status)}
            </span>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
{/if}
