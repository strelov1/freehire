<script lang="ts">
  import { resolve } from '$app/paths';
  import { api } from '#lib/api.js';
  import { hasVisibleRows, pendingRows, type PendingAnswerRow } from '#lib/answerBank.js';
  import { isAutoApplyPaused, setAutoApplyPaused } from '#lib/autoApplyPauseStorage.js';
  import { autoApplyProgressSteps } from '#lib/autoApplyProgress.js';
  import { autoApplyReviewBanner } from '#lib/autoApplyReview.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, t } from '#lib/i18n/t.js';
  import type { AutoApplyReviewInfo, TimelineEvent } from '#lib/types.js';
  import { Button } from '#lib/ui/index.js';
  import { errorMessage } from '#lib/utils.js';
  import AutoApplyProgress from './AutoApplyProgress.svelte';
  import { messages } from './JobDrawer.messages';

  // JobDrawer's auto-apply tab: the attempt's progress, pause and cancel, the review of
  // what will be sent, the questions it parked on and the answers banked for them, and
  // retry. `autoApply` is the drawer's (loaded with the linked mail) and is re-read here
  // after an action, the application's `events` handed back through `onevents`; `ongone` is told when a cancel took the attempt away.
  let {
    slug,
    hasPosting,
    autoApply = $bindable(),
    autoApplied,
    onevents,
    applyChatError,
    onstartapplychat,
    onautoapplyreview,
    ongone,
  }: {
    slug: string | null;
    hasPosting: boolean;
    autoApply: AutoApplyReviewInfo | null;
    onevents: (events: TimelineEvent[]) => void;
    autoApplied: boolean;
    applyChatError: string | null;
    onstartapplychat: () => void;
    onautoapplyreview: (decision: 'approved' | 'declined') => void;
    ongone: () => void;
  } = $props();

  const s = $derived(t(messages, locale()));

  let autoApplyDeciding = $state(false);
  let autoApplyError = $state<string | null>(null);
  const autoApplyBanner = $derived(autoApplyReviewBanner(autoApply?.status));

  // The application chat (openspec/changes/auto-apply-chat): the same questions and the same
  // review as the form below, answered in free text. Offered exactly where the form is —
  // while the attempt waits on the candidate.
  const offersApplyChat = $derived(autoApplyBanner?.kind === 'blocked' || autoApplyBanner?.kind === 'pending_review');

  const progressSteps = $derived(autoApplyProgressSteps(autoApply?.status, autoApplied));

  // The Pause/Continue marker (openspec/changes/auto-apply-progress-tab/design.md) is
  // purely a candidate-side reminder — it never reaches the server and has no effect on
  // cmd/auto-apply. Only offered over a live, still-open attempt: once it is terminal or
  // already submitted there is nothing left to pause. `pausedOverride` lets a press inside
  // this mount take effect immediately, without waiting on a reload to re-read storage.
  const canPauseAutoApply = $derived(
    !!autoApply && ['tailoring', 'pending_review', 'approved', 'blocked'].includes(autoApply.status),
  );
  let pausedOverride = $state<boolean | null>(null);
  const autoApplyPaused = $derived(pausedOverride ?? (autoApply ? isAutoApplyPaused(autoApply.queue_id) : false));
  function toggleAutoApplyPaused() {
    if (!autoApply) return;
    const next = !autoApplyPaused;
    setAutoApplyPaused(autoApply.queue_id, next);
    pausedOverride = next;
  }

  async function decideAutoApply(decision: 'approved' | 'declined') {
    if (!autoApply || autoApplyDeciding || autoApplyActing) return;
    autoApplyDeciding = true;
    autoApplyError = null;
    try {
      await api.reviewAutoApply(String(autoApply.queue_id), decision);
      autoApply = { ...autoApply, status: decision };
      // The board's own item is the parent's to mutate — see onautoapplyreview's own
      // doc comment for why this isn't done directly here.
      onautoapplyreview(decision);
    } catch (e) {
      autoApplyError = errorMessage(e, s.decisionFailed);
    } finally {
      autoApplyDeciding = false;
    }
  }

  // pendingRows pairs every pending question with its position in the list — never its
  // label — because two distinct questions can share label text and an entry can carry no
  // text at all; either would collide an `{#each}` key, a DOM id, or a draft keyed on the
  // text itself.
  const pendingQuestions = $derived(pendingRows(autoApply?.resolved_preview?.pending));

  // Whether anything in the pending list will actually render. Only an `empty` row (see
  // answerBank's PendingRowKind) draws nothing, so gating the list on pendingQuestions.length
  // alone produces an empty bulleted block whenever every pending question is empty-label. A
  // work-authorization question is `blocked`, not `empty` — it still renders, naming what is
  // stopping the application — so hasVisibleRows is what decides this, not a re-derivation
  // here.
  const showsPendingList = $derived(hasVisibleRows(pendingQuestions));

  // One draft per pending question, keyed by its row's position (see pendingQuestions).
  let bankDrafts = $state<Record<number, string>>({});
  let bankSaving = $state<number | null>(null);
  let bankError = $state<string | null>(null);
  // The rows whose answer reached the bank in this session. Nothing else says a save
  // happened: the draft clears and the button re-disables, which on its own reads like
  // typing that got lost.
  let bankSaved = $state<Record<number, boolean>>({});

  async function saveBankedAnswer(key: number, question: string) {
    const answer = bankDrafts[key]?.trim();
    // One save at a time — but every Save button is disabled while one is in flight, not
    // just this row's. A button that looks pressable and silently does nothing is worse
    // than one that is plainly unavailable.
    if (!answer || bankSaving !== null) return;
    bankSaving = key;
    bankError = null;
    try {
      await api.saveBankedAnswer(question, answer);
      // Cleared rather than left filled: the answer now lives in the bank, and a filled
      // input beside a saved answer reads as unsaved work. The marker below is what tells
      // the candidate it went somewhere.
      bankDrafts = { ...bankDrafts, [key]: '' };
      bankSaved = { ...bankSaved, [key]: true };
    } catch (e) {
      bankError = errorMessage(e, s.errors.saveAnswer);
    } finally {
      bankSaving = null;
    }
  }
  // The questions a submission attempt parked on, answerable the same way as the review's
  // pending ones: save the answer to the bank, then try again (openspec/changes/
  // auto-apply-over-mcp). Only one banner renders at a time, so the two lists share the
  // drafts below without their position keys colliding.
  const blockedQuestions = $derived(
    pendingRows(autoApply?.unmapped?.map((u) => ({ label: u.label, will_draft_at_submission: false }))),
  );
  const showsBlockedList = $derived(hasVisibleRows(blockedQuestions));

  // Retry re-arms a stopped attempt (or refreshes a review's preview after answers were
  // saved); cancel withdraws one. Both re-read the attempt afterwards rather than guessing
  // its next status — a retry can land on any of three, depending on where it stopped.
  let autoApplyActing = $state(false);
  async function actOnAutoApply(act: (slug: string) => Promise<unknown>, failed: string) {
    // One action at a time across review, retry and cancel: a review answer landing after
    // a cancel's refresh would restore the status the cancel just removed.
    if (!slug || autoApplyActing || autoApplyDeciding) return;
    autoApplyActing = true;
    autoApplyError = null;
    try {
      await act(slug);
      bankSaved = {};
      // Read directly rather than through loadEmails, which returns early while its own
      // load is in flight and would leave the pre-action status on screen.
      const app = await api.getTrackedApplication(slug);
      autoApply = app.auto_apply ?? null;
      onevents(app.events ?? []);
      // A cancel takes the Progress tab away with the attempt; stay on a tab that exists.
      if (!autoApply && !autoApplied) ongone();
    } catch (e) {
      autoApplyError = errorMessage(e, failed);
    } finally {
      autoApplyActing = false;
    }
  }
  const retryAutoApply = () => actOnAutoApply(api.retryAutoApply, s.retryFailed);
  const cancelAutoApply = () => actOnAutoApply(api.cancelAutoApply, s.cancelFailed);
  const savedAnyAnswer = $derived(Object.values(bankSaved).some(Boolean));
</script>

{#snippet answerList(rows: PendingAnswerRow[])}
  <ul class="flex flex-col gap-2 text-xs text-muted-foreground">
    {#each rows as row (row.key)}
      {#if row.kind === 'draft'}
        <li>{format(s.willBeFilledAutomatically, { label: row.pending.label })}</li>
      {:else if row.kind === 'blocked'}
        <!-- A work-authorization question: the bank refuses to recall an
           answer for it (the correct one depends on this posting's own
           country), and offering an input would only collect one it could
           never safely reuse. Plain text, distinct in wording and color from
           both the "filled automatically" row above and an answerable
           question's input below, so the candidate sees what is blocking
           the application instead of nothing at all. -->
        <li class="text-warning-strong">
          {format(s.cannotAnswerForYou, { label: row.pending.label })}
        </li>
      {:else if row.kind === 'answerable'}
        <li class="flex flex-col gap-1">
          <label class="text-foreground" for={`bank-${row.key}`}>{row.pending.label}</label>
          <div class="flex items-start gap-2">
            <!-- A textarea, not an input: the bound is 2000 characters, and a
               single-line control collects a paragraph the writer cannot
               re-read. -->
            <textarea
              id={`bank-${row.key}`}
              rows="2"
              maxlength="2000"
              class="min-w-0 flex-1 resize-y rounded-md border border-border bg-background px-2 py-1"
              bind:value={bankDrafts[row.key]}
              placeholder={s.answerPlaceholder}></textarea>
            <Button
              size="sm"
              variant="outline"
              disabled={bankSaving !== null || !bankDrafts[row.key]?.trim()}
              onclick={() => saveBankedAnswer(row.key, row.pending.label)}
            >
              {s.save}
            </Button>
          </div>
          {#if bankSaved[row.key]}
            <span class="text-brand-strong">{s.savedToBank}</span>
          {/if}
        </li>
      {/if}
    {/each}
  </ul>
{/snippet}

<div class="flex flex-col gap-4">
  {#if progressSteps}
    <AutoApplyProgress steps={progressSteps} />
  {/if}

  {#if canPauseAutoApply}
    <div class="flex flex-col gap-1">
      <div class="flex items-center gap-2">
        <Button variant="outline" size="sm" class="w-fit" onclick={toggleAutoApplyPaused}>
          {autoApplyPaused ? s.continue : s.pause}
        </Button>
        <Button
          variant="ghost"
          size="sm"
          class="w-fit"
          disabled={autoApplyActing || autoApplyDeciding}
          onclick={cancelAutoApply}
        >
          {s.cancelAutoApply}
        </Button>
      </div>
      {#if autoApplyPaused}
        <p class="text-xs text-muted-foreground">
          {s.pausedNote}
        </p>
      {/if}
    </div>
  {/if}

  <!-- Auto-apply's own "action needed"/status content (openspec/changes/
       auto-apply-progress-tab) — moved here from the Application tab so this is
       the one place it lives. -->
  {#if offersApplyChat}
    <div class="flex flex-col gap-1">
      <Button variant="primary" size="sm" class="w-fit" onclick={onstartapplychat}>{s.applyChat.open}</Button>
      <p class="text-xs text-muted-foreground">{s.applyChat.hint}</p>
      {#if applyChatError}
        <p class="text-xs text-destructive">{applyChatError}</p>
      {/if}
    </div>
  {/if}

  {#if autoApplyBanner?.kind === 'tailoring'}
    <div class="rounded-md border border-border bg-muted/30 px-3 py-2">
      <p class="text-sm text-muted-foreground">{s.banners.tailoring}</p>
    </div>
  {:else if autoApplyBanner?.kind === 'approved'}
    <div class="flex flex-col gap-2 rounded-md border border-border bg-muted/30 px-3 py-2">
      <p class="text-sm font-medium">{s.banners.approved}</p>
      {#if hasPosting && slug}
        <a href={resolve('/tailor/[slug]', { slug: slug })} class="w-fit text-xs underline-offset-2 hover:underline">
          {s.viewTailoredCv}
        </a>
      {/if}
    </div>
  {:else if autoApplyBanner?.kind === 'pending_review'}
    <div class="flex flex-col gap-2 rounded-md border border-warning/50 bg-warning-muted/40 px-3 py-2">
      <p class="text-sm font-medium">{s.banners.pendingReview}</p>
      {#if autoApply?.resolved_preview?.fields.length}
        <dl class="flex flex-col gap-1 text-sm">
          <!-- Keyed on the position, as JobApplyForm keys the questions it renders
               from the same forms. A PreviewField carries a label and a value and
               nothing else (internal/application/autoapply/preview.go), and an ATS
               form is free to ask "Location" twice — so a key on the label is one
               repeated question away from each_key_duplicate taking the drawer
               down, the same way the tag row did (FREEHIRE-WEB-20). This list is a
               read-only snapshot that nothing reorders, so position IS its
               identity. -->
          {#each autoApply.resolved_preview.fields as f, i (i)}
            <div class="flex gap-2">
              <dt class="shrink-0 text-muted-foreground">{f.label}:</dt>
              <dd class="min-w-0 truncate">{f.value}</dd>
            </div>
          {/each}
        </dl>
      {/if}
      {#if showsPendingList}
        {@render answerList(pendingQuestions)}
        {#if bankError}
          <p class="text-xs text-destructive">{bankError}</p>
        {/if}
      {/if}
      {#if hasPosting && slug}
        <a href={resolve('/tailor/[slug]', { slug: slug })} class="w-fit text-xs underline-offset-2 hover:underline">
          {s.viewTailoredCv}
        </a>
      {/if}
      {#if savedAnyAnswer}
        <div class="flex flex-col gap-1">
          <Button size="sm" variant="outline" class="w-fit" disabled={autoApplyActing} onclick={retryAutoApply}>
            {s.refreshPreview}
          </Button>
          <p class="text-xs text-muted-foreground">{s.refreshPreviewNote}</p>
        </div>
      {/if}
      <div class="flex items-center gap-2">
        <Button size="sm" disabled={autoApplyDeciding || autoApplyActing} onclick={() => decideAutoApply('approved')}>
          {s.approveAndSend}
        </Button>
        <Button
          size="sm"
          variant="outline"
          disabled={autoApplyDeciding || autoApplyActing}
          onclick={() => decideAutoApply('declined')}
        >
          {s.decline}
        </Button>
      </div>
      {#if autoApplyError}
        <p class="text-xs text-destructive">{autoApplyError}</p>
      {/if}
    </div>
  {:else if autoApplyBanner?.kind === 'blocked'}
    <div class="flex flex-col gap-2 rounded-md border border-warning/50 bg-warning-muted/40 px-3 py-2">
      <p class="text-sm font-medium">{s.banners.blocked}</p>
      {#if showsBlockedList}
        {@render answerList(blockedQuestions)}
        {#if bankError}
          <p class="text-xs text-destructive">{bankError}</p>
        {/if}
      {/if}
      <p class="text-xs text-muted-foreground">{s.banners.blockedRetryHint}</p>
      <Button size="sm" class="w-fit" disabled={autoApplyActing} onclick={retryAutoApply}>
        {s.tryAgain}
      </Button>
      {#if autoApplyError}
        <p class="text-xs text-destructive">{autoApplyError}</p>
      {/if}
    </div>
  {:else if autoApplyBanner?.kind === 'declined'}
    <div class="rounded-md border border-border bg-muted/30 px-3 py-2">
      <p class="text-sm text-muted-foreground">
        {s.banners.declined}
      </p>
    </div>
  {:else if autoApplyBanner?.kind === 'failed'}
    <div class="flex flex-col gap-2 rounded-md border border-border bg-muted/30 px-3 py-2">
      {#if autoApply?.may_have_reached_employer}
        <!-- The one failure retry refuses: the employer may already hold the
             application, and sending it again would be a duplicate in the
             candidate's name. -->
        <p class="text-sm text-muted-foreground">{s.banners.failedUncertain}</p>
      {:else}
        <p class="text-sm text-muted-foreground">{s.banners.failed}</p>
        <Button size="sm" variant="outline" class="w-fit" disabled={autoApplyActing} onclick={retryAutoApply}>
          {s.tryAgain}
        </Button>
      {/if}
      {#if autoApplyError}
        <p class="text-xs text-destructive">{autoApplyError}</p>
      {/if}
    </div>
  {:else if autoApplyBanner?.kind === 'tailor_failed'}
    <!-- Deliberately not the 'failed' copy: nothing was submitted and there is no
         tailored CV to look at, so "could not submit after retrying" would describe
         a step this attempt never reached. Says what did NOT happen, because the
         state it replaces (an entry that read as 'tailoring' forever) left people
         believing an application was on its way. -->
    <div class="rounded-md border border-border bg-muted/30 px-3 py-2">
      <p class="text-sm font-medium">{s.banners.tailorFailed}</p>
      <p class="text-xs text-muted-foreground">
        {s.banners.tailorFailedDetail}
      </p>
      <Button size="sm" variant="outline" class="w-fit" disabled={autoApplyActing} onclick={retryAutoApply}>
        {s.tryAgain}
      </Button>
      {#if autoApplyError}
        <p class="text-xs text-destructive">{autoApplyError}</p>
      {/if}
      {#if hasPosting && slug}
        <a href={resolve('/tailor/[slug]', { slug: slug })} class="w-fit text-xs underline-offset-2 hover:underline">
          {s.tailorACv}
        </a>
      {/if}
    </div>
  {:else if autoApplied}
    <div class="rounded-md border border-border bg-muted/30 px-3 py-2">
      <p class="text-sm text-muted-foreground">{s.banners.autoApplied}</p>
    </div>
  {/if}
</div>
