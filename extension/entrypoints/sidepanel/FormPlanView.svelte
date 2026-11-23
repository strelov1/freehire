<script lang="ts">
  import type { ApplyPlan, PlanItem } from '../../lib/applyPlan';
  import type { AnswerOffer } from '../../lib/answerOffer';
  import ApplyPlanCard from './ApplyPlan.svelte';
  import SaveAnswers from './SaveAnswers.svelte';

  let {
    plan,
    planError,
    questionsSeen,
    quiet = false,
    fillingLabel,
    autofilling,
    onReveal,
    onCancel,
    answerOffers,
    savingAnswers,
    onSave,
  }: {
    plan: ApplyPlan | null;
    planError: string;
    questionsSeen: number;
    /** On a page whose job card is showing, the stray-fields note is noise (an Easy Apply
     *  dialog mid-step reads that way for a moment). */
    quiet?: boolean;
    fillingLabel: string | null;
    autofilling: boolean;
    onReveal: (item: PlanItem) => void;
    onCancel: () => void;
    answerOffers: AnswerOffer[];
    savingAnswers: boolean;
    onSave: (chosen: AnswerOffer[]) => void;
  } = $props();
</script>

<!-- The account of the form the user is standing in front of. It is
     independent of the match: a page can show an application freehire
     does not carry a posting for, and the checklist is just as useful
     there. -->
{#if !plan && planError}
  <p class="no-plan">Could not read this page's form: {planError}</p>
{:else if !plan && questionsSeen > 0 && !quiet}
  <!-- The page asks questions but they did not add up to an
       application. Saying so beats an empty space the user has to
       guess about. -->
  <p class="no-plan">
    {questionsSeen} field{questionsSeen === 1 ? '' : 's'} on this page — not enough to read as an application form.
  </p>
{/if}
{#if plan}
  <ApplyPlanCard {plan} filling={fillingLabel} walking={autofilling} {onReveal} {onCancel} />
  {#if answerOffers.length > 0}
    <SaveAnswers offers={answerOffers} saving={savingAnswers} {onSave} />
  {/if}
{/if}

<style>
  .no-plan {
    font-size: 12px;
    color: var(--muted-foreground);
    padding: 0 2px;
  }
</style>
