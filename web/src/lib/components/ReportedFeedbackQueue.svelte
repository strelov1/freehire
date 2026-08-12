<script lang="ts">
  import { resolve as resolveRoute } from '$app/paths';
  import { api } from '$lib/api';
  import { AsyncData } from '$lib/asyncData.svelte';
  import { companyFeedbackTypeLabel } from '$lib/companyFeedback';
  import type { ReportedCompanyFeedback } from '$lib/types';
  import { Badge, Button } from '$lib/ui';
  import { timeAgo } from '$lib/utils';
  import States from './States.svelte';

  // Rendered only inside the moderator-gated ModerationView, so it assumes a
  // moderator session; the server authorizes independently regardless. Unlike
  // ReportQueue (job reports), a report here carries no resolve/dismiss state
  // of its own — Hide is the one action, and it acts on the review, not the
  // report rows, so there is nothing left to "decide" once it's done.
  let acting = $state<number | null>(null);
  let actionError = $state<string | null>(null);

  const queueData = new AsyncData<ReportedCompanyFeedback[]>([]);
  $effect(() => {
    void queueData.run(() => api.listReportedCompanyFeedback());
  });
  const status = $derived(queueData.status);
  const queue = $derived(queueData.value);

  function drop(id: number) {
    queueData.value = queueData.value.filter((r) => r.id !== id);
  }

  async function hide(r: ReportedCompanyFeedback) {
    if (acting !== null) return;
    acting = r.id;
    actionError = null;
    try {
      await api.hideCompanyFeedback(r.id);
      drop(r.id);
    } catch {
      actionError = `Could not hide the review on "${r.company_slug}". It may already be hidden.`;
    } finally {
      acting = null;
    }
  }
</script>

<div class="flex flex-col gap-6">
  <div class="flex flex-col gap-1">
    <h2 class="text-xl font-semibold tracking-tight">Reported feedback</h2>
    <p class="text-sm text-muted-foreground">
      Company reviews flagged by readers. Hiding drops a review from its company's public list and
      rating average immediately. There is no further queue state — a hidden review stays hidden
      until someone changes it directly.
    </p>
  </div>

  {#if actionError}
    <p class="text-sm text-destructive">{actionError}</p>
  {/if}

  {#if status === 'loading'}
    <States state="loading" />
  {:else if status === 'error'}
    <States state="error" message="Couldn't load reported feedback." />
  {:else if queue.length === 0}
    <States state="empty" message="No reported feedback — nothing to review." />
  {:else}
    <ul class="flex flex-col divide-y divide-border rounded-lg border border-border">
      {#each queue as r (r.id)}
        <li class="flex flex-col gap-3 px-4 py-3">
          <div class="flex flex-col gap-1">
            <div class="flex flex-wrap items-center gap-2">
              <a
                href={resolveRoute('/companies/[slug]', { slug: r.company_slug })}
                class="text-sm font-medium hover:underline"
              >
                {r.company_slug}
              </a>
              <Badge variant="secondary">{companyFeedbackTypeLabel(r.feedback_type)}</Badge>
              <Badge variant="missing">{r.report_count} report{r.report_count === 1 ? '' : 's'}</Badge>
            </div>
            <p class="whitespace-pre-wrap text-sm text-muted-foreground">{r.body}</p>
            <span class="text-xs text-muted-foreground">
              {r.rating}★ by {r.author}, posted {timeAgo(r.created_at)} · reported for {r.report_reasons.join(', ')}
            </span>
          </div>
          <div class="flex shrink-0 gap-2">
            <Button variant="primary" size="sm" disabled={acting !== null} onclick={() => hide(r)}>
              {acting === r.id ? 'Hiding…' : 'Hide review'}
            </Button>
          </div>
        </li>
      {/each}
    </ul>
  {/if}
</div>
