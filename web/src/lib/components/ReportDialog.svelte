<script lang="ts">
  import { ArrowLeft, Ban, BellOff, Bot, Check, ChevronRight, Clock, MoreHorizontal, ShieldAlert } from '@lucide/svelte';
  import { api, ApiError } from '$lib/api';
  import { appliedOnError, moderationReasonOf, processKindOf, reportReasons, reportRoute } from '$lib/reports';
  import type { ProcessReportKind, ReportPickerValue } from '$lib/types';
  import { Button, Dialog } from '$lib/ui';

  // Reports are filed against a single job, addressed by its public slug — except
  // the process facts, which are about the EMPLOYER and go to the company the job
  // already names. The parent owns open/close; this component owns the flow within.
  let {
    slug,
    companySlug,
    onClose,
  }: { slug: string; companySlug: string; onClose: () => void } = $props();

  // The parent mounts this component to open it and unmounts on onClose, so the
  // dialog is open for its whole life. Dialog owns the closing — Escape, the
  // backdrop and its own button all land on `open`, and this reports it upward
  // rather than each affordance calling onClose itself.
  let open = $state(true);
  $effect(() => {
    if (!open) onClose();
  });

  // step 'reason' picks one of the controlled reasons; 'details' collects an
  // optional elaboration; 'applied' asks when the caller applied; 'done' is the
  // post-submit thank-you.
  //
  // 'no_response' takes the 'applied' branch and files EVIDENCE, not a moderation
  // report. The user is describing what happened to them, not choosing a routing,
  // so the dialog looks the same either way. It matters because the moderation
  // queue's only lever is closing the job: somebody says nobody answered them and
  // the one available response is to delete the posting, which helps no one.
  let step = $state<'reason' | 'details' | 'applied' | 'done'>('reason');
  let reason = $state<ReportPickerValue | null>(null);
  let details = $state('');
  let appliedOn = $state('');
  let submitting = $state(false);
  let error = $state<string | null>(null);
  // Whether the last process action took a report BACK, so the closing line says what
  // happened rather than thanking somebody for withdrawing.
  let withdrew = $state(false);

  // Which process facts this caller already holds against the company. Loaded when the
  // dialog opens so the picker shows a held entry as held and offers to withdraw it —
  // without this the only way to learn is to file and be refused, which reads as the
  // dialog losing the report the person remembers making.
  //
  // Best effort: a failed load leaves the entry offered, and filing then answers 409
  // with a message that says so. A dialog that refuses to open because one read failed
  // would be worse than one that occasionally asks a question it could have answered.
  let myKinds = $state<ProcessReportKind[]>([]);
  $effect(() => {
    let cancelled = false;
    void api
      .myCompanyProcessReports(companySlug)
      .then((kinds) => {
        if (!cancelled) myKinds = kinds;
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  });

  // The element itself, because a malformed entry never reaches `appliedOn`: the
  // input clears its value and records the fact in `validity.badInput`.
  let appliedInput = $state<HTMLInputElement | null>(null);

  // A claim needs a date we can measure silence from, and no date can be in the
  // future. The server re-checks both — this is the affordance, not the guard.
  const today = new Date().toISOString().slice(0, 10);

  // A lucide icon per reason, keyed by the controlled value (the labels/order
  // themselves live in $lib/reports so they stay one source of truth).
  const reasonIcon: Record<ReportPickerValue, typeof BellOff> = {
    no_response: BellOff,
    ai_interview: Bot,
    not_relevant: Clock,
    spam: Ban,
    fraud: ShieldAlert,
    other: MoreHorizontal,
  };

  function pick(r: ReportPickerValue) {
    reason = r;
    error = null;
    const route = reportRoute(r);
    if (route === 'ghost') {
      step = 'applied';
      return;
    }
    if (route === 'process') {
      // Submitted from the picker with no second step. The entry IS the whole claim:
      // there is no date to bound it and nothing to elaborate, and asking anyway would
      // collect whatever gets typed to get past the field.
      //
      // A held entry withdraws instead. Withdrawal is how the signal self-heals when an
      // employer changes practice, so it has to be reachable from the same one tap that
      // filed it.
      const kind = processKindOf(r);
      if (!kind) return;
      if (myKinds.includes(kind)) {
        withdrew = true;
        void send(async () => {
          await api.withdrawCompanyProcessReport(companySlug, kind);
          myKinds = myKinds.filter((k) => k !== kind);
        });
        return;
      }
      withdrew = false;
      void send(async () => {
        await api.reportCompanyProcess(companySlug, kind);
        myKinds = [...myKinds, kind];
      });
      return;
    }
    step = 'details';
  }

  // Whether the caller already holds this entry, so the picker can say so.
  function held(value: ReportPickerValue): boolean {
    const kind = processKindOf(value);
    return kind !== null && myKinds.includes(kind);
  }

  function messageFor(e: unknown): string {
    if (e instanceof ApiError) {
      if (e.status === 409) return 'You already reported this.';
      if (e.status === 403) return 'Please confirm your email address first.';
      if (e.status === 429) return "That's a lot of reports today — try again tomorrow.";
      if (e.status === 401) return 'Please sign in to report a job.';
    }
    return 'Something went wrong. Please try again.';
  }

  // Wraps whichever call the chosen reason implies, so the two branches differ
  // only in what they send.
  async function send(call: () => Promise<unknown>) {
    error = null;
    submitting = true;
    try {
      await call();
      step = 'done';
    } catch (err) {
      error = messageFor(err);
    } finally {
      submitting = false;
    }
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    // Captured into a local before the closure: narrowing `reason` with an early
    // return does not survive into the callback passed to send().
    // Narrowed through moderationReasonOf rather than cast: it is the guard that
    // keeps a picker entry off the endpoint whose vocabulary does not contain it.
    const picked = reason && moderationReasonOf(reason);
    if (!picked) return;
    await send(() => api.reportJob(slug, { reason: picked, details }));
  }

  async function submitApplied(e: SubmitEvent) {
    e.preventDefault();
    const problem = appliedOnError({
      value: appliedOn,
      badInput: appliedInput?.validity.badInput ?? false,
      today,
    });
    if (problem) {
      error = problem;
      return;
    }
    await send(() => api.reportGhostJob(slug, { applied_on: appliedOn }));
  }
</script>

<Dialog bind:open title="Report this job" class="sm:max-w-md">

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
                <span class="text-xs text-muted-foreground">
                  {held(r.value) ? 'You reported this — tap to withdraw' : r.hint}
                </span>
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

        <!-- Optional by design: the reason already says what is wrong, so this is
             elaboration. A required field mostly collects whatever gets typed to
             pass it, which reaches a moderator looking like evidence. -->
        <label class="flex flex-col gap-1.5 text-sm">
          <span class="font-medium">Tell us more</span>
          <span class="text-xs text-muted-foreground">
            Optional — a line or two helps us review it faster.
          </span>
          <textarea
            bind:value={details}
            rows="4"
            placeholder="Anything else we should know…"
            class="mt-1 resize-y rounded-md border border-border bg-background px-3 py-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          ></textarea>
        </label>

        {#if error}
          <p role="alert" class="text-sm text-destructive">{error}</p>
        {/if}

        <Button type="submit" variant="primary" disabled={submitting}>
          {submitting ? 'Sending…' : 'Send report'}
        </Button>
      </form>
    {:else if step === 'applied'}
      <!-- novalidate: the browser's own refusal for an impossible date is a tooltip
           that never says which part is wrong, and it fires before submit, so the
           dialog would never get to explain itself. -->
      <form class="flex flex-col gap-4" novalidate onsubmit={submitApplied}>
        <button
          type="button"
          onclick={() => (step = 'reason')}
          class="flex items-center gap-1.5 self-start text-sm text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft class="size-4" /> Back
        </button>

        <label class="flex flex-col gap-1.5 text-sm">
          <span class="font-medium">When did you apply?</span>
          <span class="text-xs text-muted-foreground">
            We count how long a posting has gone unanswered, so the date is what makes this
            useful to other people.
          </span>
          <input
            type="date"
            bind:this={appliedInput}
            bind:value={appliedOn}
            oninput={() => (error = null)}
            required
            max={today}
            class="mt-1 rounded-md border border-border bg-background px-3 py-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
        </label>

        {#if error}
          <p role="alert" class="text-sm text-destructive">{error}</p>
        {/if}

        <!-- Never disabled on an empty date: an impossible entry empties the value
             while the field still reads as filled in, and a dead button is the one
             thing that cannot say why. -->
        <Button type="submit" variant="primary" disabled={submitting}>
          {submitting ? 'Sending…' : 'Send report'}
        </Button>
      </form>
    {:else}
      <div class="flex flex-col items-center gap-3 py-4 text-center">
        <span class="flex size-10 items-center justify-center rounded-full bg-secondary">
          <Check class="size-5" />
        </span>
        <p class="text-sm">
          {#if reason && reportRoute(reason) === 'ghost'}
            Thanks — noted. If enough people report the same thing, we'll flag this posting.
          {:else if reason && reportRoute(reason) === 'process'}
            {#if withdrew}
              Withdrawn — your report no longer counts toward this company's label.
            {:else}
              Thanks — noted. This is now shown on the company, with how many people reported it.
            {/if}
          {:else}
            Thanks — your report was sent. We'll take a look.
          {/if}
        </p>
        <Button variant="outline" onclick={onClose} class="mt-1">Close</Button>
      </div>
    {/if}
</Dialog>
