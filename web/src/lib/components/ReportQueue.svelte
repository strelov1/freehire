<script lang="ts">
  import { dismissReport, listPendingReports, resolveReport } from '$lib/api';
  import { reportReasonLabel } from '$lib/reports';
  import type { Report } from '$lib/types';
  import { Badge, Button } from '$lib/ui';
  import { timeAgo } from '$lib/utils';
  import States from './States.svelte';

  // Rendered only inside the moderator-gated ModerationView, so it assumes a
  // moderator session; the server authorizes independently regardless.
  let status = $state<'loading' | 'error' | 'ready'>('loading');
  let queue = $state.raw<Report[]>([]);
  // The id currently being acted on, to disable its row's buttons.
  let acting = $state<number | null>(null);
  let actionError = $state<string | null>(null);

  async function load() {
    status = 'loading';
    try {
      queue = await listPendingReports();
      status = 'ready';
    } catch {
      status = 'error';
    }
  }

  // Load once on mount (the parent only mounts this for a moderator).
  $effect(() => {
    void load();
  });

  function drop(id: number) {
    queue = queue.filter((r) => r.id !== id);
  }

  // Resolve a report; closeJob decides whether the reported vacancy is soft-closed.
  async function resolve(r: Report, closeJob: boolean) {
    if (acting !== null) return;
    acting = r.id;
    actionError = null;
    try {
      await resolveReport(r.id, closeJob);
      drop(r.id);
    } catch {
      actionError = `Could not resolve the report on "${r.job_title}". It may have already been decided.`;
    } finally {
      acting = null;
    }
  }

  async function dismiss(r: Report) {
    if (acting !== null) return;
    const reason = window.prompt(`Dismiss the report on "${r.job_title}"? Optional reason:`, '');
    // null = cancelled; an empty string is a reasonless dismissal, which is allowed.
    if (reason === null) return;
    acting = r.id;
    actionError = null;
    try {
      await dismissReport(r.id, reason);
      drop(r.id);
    } catch {
      actionError = `Could not dismiss the report on "${r.job_title}". It may have already been decided.`;
    } finally {
      acting = null;
    }
  }
</script>

<div class="flex flex-col gap-6">
  <div class="flex flex-col gap-1">
    <h2 class="text-xl font-semibold tracking-tight">Reported jobs</h2>
    <p class="text-sm text-muted-foreground">
      Reports awaiting review. Closing removes the vacancy from listings; dismissing leaves it live.
    </p>
  </div>

  {#if actionError}
    <p class="text-sm text-destructive">{actionError}</p>
  {/if}

  {#if status === 'loading'}
    <States state="loading" />
  {:else if status === 'error'}
    <States state="error" message="Couldn't load the reports." />
  {:else if queue.length === 0}
    <States state="empty" message="No reported jobs — nothing to review." />
  {:else}
    <ul class="flex flex-col divide-y divide-border rounded-lg border border-border">
      {#each queue as r (r.id)}
        <li class="flex flex-col gap-3 px-4 py-3">
          <div class="flex flex-col gap-1">
            <div class="flex flex-wrap items-center gap-2">
              {#if r.job_slug}
                <a href={`/jobs/${r.job_slug}`} class="text-sm font-medium hover:underline">
                  {r.job_title || r.job_slug}
                </a>
              {:else}
                <span class="text-sm font-medium">{r.job_title || 'Unknown job'}</span>
              {/if}
              <Badge variant="secondary">{reportReasonLabel(r.reason)}</Badge>
            </div>
            <p class="whitespace-pre-wrap text-sm text-muted-foreground">{r.details}</p>
            <span class="text-xs text-muted-foreground">
              by {r.reporter_email ?? 'unknown'}{r.contact_telegram
                ? ` · TG: ${r.contact_telegram}`
                : ''} · {timeAgo(r.created_at)}
            </span>
          </div>
          <div class="flex shrink-0 flex-wrap gap-2">
            <Button variant="primary" size="sm" disabled={acting !== null} onclick={() => resolve(r, true)}>
              Close job
            </Button>
            <Button variant="outline" size="sm" disabled={acting !== null} onclick={() => resolve(r, false)}>
              Resolve, keep open
            </Button>
            <Button variant="ghost" size="sm" disabled={acting !== null} onclick={() => dismiss(r)}>
              Dismiss
            </Button>
          </div>
        </li>
      {/each}
    </ul>
  {/if}
</div>
