<script lang="ts">
  // Aliased: this component already has a local `resolve(report, …)` action.
  import { resolve as resolveRoute } from '$app/paths';
  import { api } from '$lib/api';
  import { AsyncData } from '$lib/asyncData.svelte';
  import {
    decisionLabel,
    decisionNotePlaceholder,
    decisionNotePrompt,
    decisionOutcome,
    reportReasonLabel,
    type DecisionKind,
  } from '$lib/reports';
  import type { Report } from '$lib/types';
  import { Badge, Button } from '$lib/ui';
  import { timeAgo } from '$lib/utils';
  import States from './States.svelte';

  // Rendered only inside the moderator-gated ModerationView, so it assumes a
  // moderator session; the server authorizes independently regardless.
  // The id currently being acted on, to disable its row's buttons.
  let acting = $state<number | null>(null);
  let actionError = $state<string | null>(null);
  let actionNotice = $state<string | null>(null);

  // The row whose decision form is open, and what that form will do. Only one at a
  // time: a note belongs to one decision, so two open drafts would be ambiguous.
  let deciding = $state<{ id: number; kind: DecisionKind } | null>(null);
  let note = $state('');
  let notifyReporter = $state(true);

  // Load once on mount (the parent only mounts this for a moderator).
  const reportsData = new AsyncData<Report[]>([]);
  $effect(() => {
    void reportsData.run(() => api.listPendingReports());
  });
  const status = $derived(reportsData.status);
  const queue = $derived(reportsData.value);

  function drop(id: number) {
    reportsData.value = reportsData.value.filter((r) => r.id !== id);
  }

  // Open the decision form for one report, starting from a clean draft.
  function open(r: Report, kind: DecisionKind) {
    if (acting !== null) return;
    deciding = { id: r.id, kind };
    note = '';
    notifyReporter = true;
    actionError = null;
    actionNotice = null;
  }

  function cancel() {
    deciding = null;
    note = '';
  }

  // Send the decision the open form describes. The note reaches the reporter, so it is
  // trimmed rather than mailed with the moderator's stray whitespace.
  async function decide(r: Report) {
    if (deciding === null || acting !== null) return;
    const kind = deciding.kind;
    acting = r.id;
    actionError = null;
    actionNotice = null;
    try {
      const text = note.trim();
      const decided =
        kind === 'dismiss'
          ? await api.dismissReport(r.id, text, notifyReporter)
          : await api.resolveReport(r.id, kind === 'close', text, notifyReporter);
      actionNotice = decisionOutcome({
        notifyRequested: notifyReporter,
        notified: decided.notified ?? false,
        reporterEmail: r.reporter_email,
        jobTitle: r.job_title,
      });
      deciding = null;
      drop(r.id);
    } catch {
      actionError = `Could not decide the report on "${r.job_title}". It may have already been decided.`;
    } finally {
      acting = null;
    }
  }
</script>

<div class="flex flex-col gap-6">
  <div class="flex flex-col gap-1">
    <h2 class="text-xl font-semibold tracking-tight">Reported jobs</h2>
    <p class="text-sm text-muted-foreground">
      Reports awaiting review. Closing removes the vacancy from listings; dismissing leaves it live. Either way the
      reporter can be emailed what you decided.
    </p>
  </div>

  {#if actionError}
    <p class="text-sm text-destructive">{actionError}</p>
  {/if}
  {#if actionNotice}
    <p class="text-sm text-warning-strong">{actionNotice}</p>
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
                <a href={resolveRoute('/jobs/[slug]', { slug: r.job_slug })} class="text-sm font-medium hover:underline">
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

          {#if deciding?.id === r.id}
            <div class="flex flex-col gap-2 rounded-md border border-border bg-muted/30 p-3">
              <label class="flex flex-col gap-1">
                <span class="text-xs font-medium text-muted-foreground">{decisionNotePrompt(deciding.kind)}</span>
                <textarea
                  bind:value={note}
                  rows="3"
                  placeholder={decisionNotePlaceholder(deciding.kind)}
                  class="w-full resize-y rounded border border-border bg-background px-2 py-1 text-sm"
                ></textarea>
              </label>
              <label class="flex items-center gap-2 text-sm">
                <input type="checkbox" bind:checked={notifyReporter} class="size-4 rounded border-input" />
                Email this to {r.reporter_email ?? 'the reporter'}
              </label>
              <div class="flex flex-wrap gap-2">
                <Button variant="primary" size="sm" disabled={acting !== null} onclick={() => decide(r)}>
                  {decisionLabel(deciding.kind)}
                </Button>
                <Button variant="ghost" size="sm" disabled={acting !== null} onclick={cancel}>Cancel</Button>
              </div>
            </div>
          {:else}
            <div class="flex shrink-0 flex-wrap gap-2">
              <Button variant="primary" size="sm" disabled={acting !== null} onclick={() => open(r, 'close')}>
                {decisionLabel('close')}
              </Button>
              <Button variant="outline" size="sm" disabled={acting !== null} onclick={() => open(r, 'resolve')}>
                {decisionLabel('resolve')}
              </Button>
              <Button variant="ghost" size="sm" disabled={acting !== null} onclick={() => open(r, 'dismiss')}>
                {decisionLabel('dismiss')}
              </Button>
            </div>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
</div>
