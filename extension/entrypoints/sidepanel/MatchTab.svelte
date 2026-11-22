<script lang="ts">
  import { TOP_FRAME } from '../../lib/tools/attachCv';
  import type { HireUser } from '../../lib/auth';
  import type { FreehireJob } from '../../lib/freehire';
  import type { PlanItem } from '../../lib/applyPlan';
  import type { PlanRefused } from '../../lib/planRefused';
  import type { AnswerOffer } from '../../lib/answerOffer';
  import type { FormPlan } from './formPlan.svelte';
  import type { PageMatch } from './pageMatch.svelte';
  import MatchStatusView from './MatchStatusView.svelte';
  import AutofillFooter from './AutofillFooter.svelte';
  import FormPlanView from './FormPlanView.svelte';
  import AppliedPrompt from './AppliedPrompt.svelte';
  import PlanRefusalAlert from './PlanRefusalAlert.svelte';

  // The side panel's "Match" tab: the page's job and match, the application-form plan,
  // the action notices, and the pinned Autofill button. App.svelte owns the state.
  let {
    user,
    pageMatch,
    form,
    notices,
    asksApplied,
    appliedCandidate = $bindable(),
    autofilling,
    walkStop = $bindable(),
    answerOffers,
    savingAnswers,
    autofillRefusal,
    hasOverrideFill,
    onNotice,
    onReveal,
    onSave,
    onOverrideFill,
    onAutofill,
  }: {
    user: HireUser | null;
    pageMatch: PageMatch;
    form: FormPlan;
    notices: string[];
    asksApplied: boolean;
    appliedCandidate: FreehireJob | null;
    autofilling: boolean;
    walkStop: boolean;
    answerOffers: AnswerOffer[];
    savingAnswers: boolean;
    autofillRefusal: PlanRefused | null;
    hasOverrideFill: boolean;
    onNotice: (text: string) => void;
    onReveal: (item: PlanItem) => void;
    onSave: (chosen: AnswerOffer[]) => void;
    onOverrideFill: () => void;
    onAutofill: () => void;
  } = $props();
</script>

<div class="match-panel">
  <div class="match-scroll">
    {#if user}
      {#if asksApplied && appliedCandidate}
        <AppliedPrompt job={appliedCandidate} {onNotice} onDone={() => (appliedCandidate = null)} />
      {/if}
      <MatchStatusView
        status={pageMatch.status}
        job={pageMatch.job}
        match={pageMatch.match}
        hasUploadField={form.easyApply || form.uploads.some((u) => u.frame === TOP_FRAME)}
        error={pageMatch.error}
        onRetry={pageMatch.load}
        canAddJob={pageMatch.canAddJob}
        contributing={pageMatch.contributing}
        onAdd={pageMatch.contribute}
      />

      <FormPlanView
        plan={form.plan}
        planError={form.planError}
        questionsSeen={form.questionsSeen}
        fillingLabel={form.fillingLabel}
        {autofilling}
        {onReveal}
        onCancel={() => (walkStop = true)}
        {answerOffers}
        {savingAnswers}
        {onSave}
      />

      {#if autofillRefusal}
        <PlanRefusalAlert refusal={autofillRefusal} />
      {/if}
      {#each notices as notice, i (i)}
        <div class="message system">{notice}</div>
      {/each}
      {#if hasOverrideFill}
        <div class="message system">
          <button class="link" onclick={onOverrideFill} disabled={autofilling}>Fill it anyway</button>
        </div>
      {/if}
    {:else}
      <p class="empty">Sign in to see your match for this page.</p>
    {/if}
  </div>

  {#if user && pageMatch.status === 'ready' && pageMatch.job && pageMatch.job.public_slug !== ''}
    <AutofillFooter {autofilling} bind:walkStop {onAutofill} />
  {/if}
</div>

<style>
  .link {
    border: none;
    background: none;
    color: var(--brand-strong);
    cursor: pointer;
    font: inherit;
    font-size: 12px;
    padding: 0;
  }

  /* Plain flex wrapper, same shape as .chat-panel: .match-scroll is the part
   * that scrolls, .match-footer (the Autofill button) stays pinned below it,
   * same split as .messages/.composer on the Chat tab. */
  .match-panel {
    flex: 1;
    min-height: 0;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }

  .match-scroll {
    flex: 1;
    min-height: 0;
    min-width: 0;
    overflow-y: auto;
    padding: 12px;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  /* :global — a direct child can be a component's own root (MatchCard's card),
   * which carries that component's own scope hash, not this one; see the
   * .tab-strip note in App.svelte for the same reason. Without this, a flex column with
   * `overflow-y: auto` still shrinks its children to fit before it scrolls —
   * content taller than the visible area (a fully loaded MatchCard, well over a
   * short skeleton) proportionally squashed every child's height. `flex-shrink:
   * 0` makes .match-scroll actually scroll instead of squeezing its children. */
  .match-scroll > :global(*) {
    flex-shrink: 0;
  }

  .empty {
    color: var(--muted-foreground);
    text-align: center;
    margin-top: 40px;
  }

  .message {
    padding: 8px 10px;
    border-radius: 8px;
    max-width: 85%;
    white-space: pre-wrap;
    word-break: break-word;
  }

  .message.system {
    align-self: center;
    background: var(--warning-muted);
    font-size: 12px;
    color: var(--warning-strong);
  }
</style>
