<script lang="ts">
  import { ArrowLeft, Ban, BellOff, Check, ChevronRight, Clock, MoreHorizontal, Send, ShieldAlert, X } from '@lucide/svelte';
  import { ApiError, reportJob } from '$lib/api';
  import { reportReasons } from '$lib/reports';
  import type { ReportReason } from '$lib/types';
  import { Button } from '$lib/ui';

  // Reports are filed against a single job, addressed by its public slug. The
  // parent owns open/close; this component owns the two-step flow within.
  let { slug, onClose }: { slug: string; onClose: () => void } = $props();

  // step 'reason' picks one of the controlled reasons; 'details' collects the
  // required explanation + optional contact; 'done' is the post-submit thank-you.
  let step = $state<'reason' | 'details' | 'done'>('reason');
  let reason = $state<ReportReason | null>(null);
  let details = $state('');
  let contactTelegram = $state('');
  let submitting = $state(false);
  let error = $state<string | null>(null);

  // A lucide icon per reason, keyed by the controlled value (the labels/order
  // themselves live in $lib/reports so they stay one source of truth).
  const reasonIcon: Record<ReportReason, typeof BellOff> = {
    no_response: BellOff,
    not_relevant: Clock,
    spam: Ban,
    fraud: ShieldAlert,
    other: MoreHorizontal,
  };

  function pick(r: ReportReason) {
    reason = r;
    error = null;
    step = 'details';
  }

  function messageFor(e: unknown): string {
    if (e instanceof ApiError) {
      if (e.status === 409) return 'You already have an open report for this job.';
      if (e.status === 400) return 'Please describe what is wrong.';
      if (e.status === 401) return 'Please sign in to report a job.';
    }
    return 'Something went wrong. Please try again.';
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    if (!reason) return;
    error = null;
    submitting = true;
    try {
      await reportJob(slug, {
        reason,
        details,
        contact_telegram: contactTelegram.trim() || undefined,
      });
      step = 'done';
    } catch (err) {
      error = messageFor(err);
    } finally {
      submitting = false;
    }
  }
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && onClose()} />

<div class="fixed inset-0 z-50 flex items-center justify-center p-4">
  <!-- Backdrop is a real button so closing on click is keyboard-accessible. -->
  <button type="button" aria-label="Close dialog" class="absolute inset-0 bg-black/50" onclick={onClose}
  ></button>

  <div
    role="dialog"
    aria-modal="true"
    aria-label="Report this job"
    class="relative w-full max-w-md rounded-lg border border-border bg-background p-6 shadow-lg"
  >
    <div class="mb-4 flex items-center justify-between gap-4">
      <h2 class="text-base font-semibold tracking-tight">Report this job</h2>
      <button
        type="button"
        aria-label="Close"
        onclick={onClose}
        class="text-muted-foreground hover:text-foreground"
      >
        <X class="size-5" />
      </button>
    </div>

    {#if step === 'reason'}
      <ul class="flex flex-col gap-2">
        {#each reportReasons as r (r.value)}
          {@const Icon = reasonIcon[r.value]}
          <li>
            <button
              type="button"
              onclick={() => pick(r.value)}
              class="flex w-full items-center gap-3 rounded-md border border-border bg-card px-3 py-3 text-left transition-colors hover:bg-accent"
            >
              <Icon class="size-5 shrink-0 text-muted-foreground" />
              <span class="flex min-w-0 flex-col">
                <span class="text-sm font-medium">{r.label}</span>
                <span class="text-xs text-muted-foreground">{r.hint}</span>
              </span>
              <ChevronRight class="ml-auto size-4 shrink-0 text-muted-foreground" />
            </button>
          </li>
        {/each}
      </ul>
    {:else if step === 'details'}
      <form class="flex flex-col gap-4" onsubmit={submit}>
        <button
          type="button"
          onclick={() => (step = 'reason')}
          class="flex items-center gap-1.5 self-start text-sm text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft class="size-4" /> Back
        </button>

        <label class="flex flex-col gap-1.5 text-sm">
          <span class="font-medium">What's wrong? <span class="text-destructive">*</span></span>
          <span class="text-xs text-muted-foreground">
            A line or two helps us review it faster and protect other users.
          </span>
          <textarea
            bind:value={details}
            required
            rows="4"
            placeholder="Describe what's wrong…"
            class="mt-1 resize-y rounded-md border border-border bg-background px-3 py-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          ></textarea>
        </label>

        <label class="flex flex-col gap-1.5 text-sm">
          <span class="font-medium">Telegram for follow-up</span>
          <span class="relative">
            <Send class="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <input
              type="text"
              bind:value={contactTelegram}
              placeholder="username or link"
              class="w-full rounded-md border border-border bg-background py-2 pl-9 pr-3 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </span>
        </label>

        {#if error}
          <p class="text-sm text-destructive">{error}</p>
        {/if}

        <Button type="submit" variant="primary" disabled={submitting}>
          {submitting ? 'Sending…' : 'Send report'}
        </Button>
      </form>
    {:else}
      <div class="flex flex-col items-center gap-3 py-4 text-center">
        <span class="flex size-10 items-center justify-center rounded-full bg-secondary">
          <Check class="size-5" />
        </span>
        <p class="text-sm">Thanks — your report was sent. We'll take a look.</p>
        <Button variant="outline" onclick={onClose} class="mt-1">Close</Button>
      </div>
    {/if}
  </div>
</div>
