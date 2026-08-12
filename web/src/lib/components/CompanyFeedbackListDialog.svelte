<script lang="ts">
  import { Flag, Star } from '@lucide/svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import { api } from '$lib/api';
  import { companyFeedbackReportReasons, companyFeedbackTypeLabel } from '$lib/companyFeedback';
  import { Paginator } from '$lib/paginated.svelte';
  import type { CompanyFeedback } from '$lib/types';
  import { Button, Dialog } from '$lib/ui';
  import { formatDate } from '$lib/utils';
  import LoadMore from './LoadMore.svelte';

  // Read-only list of a company's feedback, offset-paginated on the shared
  // Paginator (retries a transient network blip instead of dead-ending the
  // dialog — the same reliability every other paginated list in the app gets).
  // The "Write feedback" button is the list's own entry point into the write
  // dialog: without it, once a company has any feedback, the header badge only
  // ever opens this read-only list and there is no way back to
  // CompanyFeedbackDialog.
  let {
    slug,
    companyName,
    onClose,
    onWriteFeedback,
  }: { slug: string; companyName?: string; onClose: () => void; onWriteFeedback: () => void } =
    $props();

  let open = $state(true);
  $effect(() => {
    if (!open) onClose();
  });

  const page = new Paginator<CompanyFeedback>((limit, offset) => api.listCompanyFeedback(slug, limit, offset));
  $effect(() => {
    void page.start();
  });

  // Reporting is inline, per item: a reason picker replaces the "Report" link,
  // not a second dialog stacked on this one. `reported` marks items already
  // flagged this session so the affordance turns into a quiet thank-you
  // instead of inviting a duplicate (the server treats a repeat as a no-op
  // regardless, but there is no reason to invite it).
  let reportingId = $state<number | null>(null);
  let reportReason = $state('');
  let reportSubmitting = $state(false);
  let reportError = $state<string | null>(null);
  const reported = new SvelteSet<number>();

  function startReport(id: number) {
    reportingId = id;
    reportReason = '';
    reportError = null;
  }
  function cancelReport() {
    reportingId = null;
  }
  async function submitReport(id: number) {
    if (!reportReason) return;
    reportSubmitting = true;
    reportError = null;
    try {
      await api.reportCompanyFeedback(id, reportReason);
      reported.add(id);
      reportingId = null;
    } catch {
      reportError = 'Could not send the report. Please try again.';
    } finally {
      reportSubmitting = false;
    }
  }
</script>

<Dialog bind:open title={companyName ? `Feedback on ${companyName}` : 'Feedback'} class="max-w-lg">
  {#if page.status !== 'loading'}
    <div class="mb-4 flex justify-end">
      <Button variant="outline" size="sm" onclick={onWriteFeedback}>Write feedback</Button>
    </div>
  {/if}
  {#if page.status === 'loading'}
    <p class="text-sm text-muted-foreground">Loading…</p>
  {:else if page.status === 'error'}
    <p role="alert" class="text-sm text-destructive">Could not load feedback. Please try again.</p>
  {:else if page.items.length === 0}
    <p class="text-sm text-muted-foreground">No feedback yet.</p>
  {:else}
    <ul class="flex max-h-96 flex-col gap-4 overflow-y-auto">
      {#each page.items as item (item.id)}
        <li class="border-b border-border pb-4 last:border-b-0 last:pb-0">
          <div class="flex items-start justify-between gap-2">
            <div class="flex items-center gap-2">
              <div class="flex items-center text-warning-strong">
                {#each [1, 2, 3, 4, 5] as value (value)}
                  <Star class="size-4" fill={value <= item.rating ? 'currentColor' : 'none'} />
                {/each}
              </div>
              <span class="rounded-full bg-secondary px-2 py-0.5 text-xs text-secondary-foreground">
                {companyFeedbackTypeLabel(item.feedback_type)}
              </span>
            </div>
            {#if reported.has(item.id)}
              <span class="text-xs text-muted-foreground">Reported</span>
            {:else if reportingId !== item.id}
              <button
                type="button"
                class="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
                onclick={() => startReport(item.id)}
              >
                <Flag class="size-3" aria-hidden="true" /> Report
              </button>
            {/if}
          </div>
          <p class="mt-2 whitespace-pre-wrap text-sm">{item.body}</p>
          <p class="mt-1.5 text-xs text-muted-foreground">{item.author} · {formatDate(item.created_at)}</p>
          {#if reportingId === item.id}
            <div class="mt-2 flex flex-wrap items-center gap-2 rounded-md border border-border bg-secondary/40 p-2">
              <select
                bind:value={reportReason}
                aria-label="Reason for reporting this review"
                class="rounded-md border border-border bg-background px-2 py-1 text-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                <option value="" disabled>Why are you reporting this?</option>
                {#each companyFeedbackReportReasons as r (r.value)}
                  <option value={r.value}>{r.label}</option>
                {/each}
              </select>
              <Button
                variant="outline"
                size="sm"
                disabled={!reportReason || reportSubmitting}
                onclick={() => submitReport(item.id)}
              >
                {reportSubmitting ? 'Sending…' : 'Send'}
              </Button>
              <Button variant="ghost" size="sm" onclick={cancelReport}>Cancel</Button>
              {#if reportError}
                <p role="alert" class="w-full text-xs text-destructive">{reportError}</p>
              {/if}
            </div>
          {/if}
        </li>
      {/each}
    </ul>
    {#if page.hasMore}
      <LoadMore loading={page.loadingMore} error={page.loadMoreError} onclick={() => page.loadMore()} />
    {/if}
  {/if}
</Dialog>
