<script module lang="ts">
  import type { Display } from '$lib/generated/contracts';

  /** Whether a form has anything to show: questions, or a statement of what it wants
   *  uploaded. Exported because the caller owning the tab has to ask the same question
   *  this component answers — a tab offered over an empty panel is worse than no tab,
   *  and two copies of the predicate would eventually disagree about which it is.
   *
   *  Narrows rather than returning a bare boolean, so a caller that passes the guard
   *  does not then have to re-prove to the compiler that the form is there. */
  export function applyFormWorthShowing(form: Display | null | undefined): form is Display {
    return !!form && ((form.questions?.length ?? 0) > 0 || (form.basics?.length ?? 0) > 0);
  }
</script>

<script lang="ts">
  // What the application will ask, as the ATS published it. `form` is null for most
  // postings — only a few platforms publish a form we can read — and the block simply
  // does not render, which is the ordinary case rather than a degraded one.
  //
  // Renders bare, with no card or heading of its own: it sits inside the job page's
  // tab panel, where the tab is the heading and a second one under it would say the
  // same thing twice.
  import { BrandMark } from '$lib/ui';
  import { applyFormModel, type GroupKey } from '$lib/applyFormGroups';
  import { ATS_MARKS } from '$lib/atsmarks';

  let { form }: { form: Display | null } = $props();

  // What each group of questions is called. Kept here rather than in the model: the
  // model decides which questions belong together and in what order, which is logic
  // worth testing; these are the words that decision is shown in.
  const GROUP_LABELS: Record<GroupKey, string> = {
    short: 'Short answers',
    choice: 'Pick from a list',
    written: 'Written answers',
    upload: 'Attachments',
  };

  const model = $derived(applyFormModel(form?.questions ?? []));
  const mark = $derived(ATS_MARKS[form?.provider ?? '']);
</script>

{#if applyFormWorthShowing(form)}
  <section class="flex flex-col gap-4">
    <!-- The caption says whose form this is and how much of it there is — the second
         being the fact that decides whether anyone applies, and the one a flat list of
         fifteen identical rows made the reader count for themselves.
         The mark sits BESIDE the provider's name, never instead of it: simple-icons
         carries one for Greenhouse and none for the other four platforms we capture
         (see atsmarks.ts), so a mark-only caption would leave most postings unattributed. -->
    <p class="flex items-center gap-1.5 text-xs text-muted-foreground">
      {#if mark}
        <!-- Hidden from the reader who is being read to: BrandMark labels itself with
             the brand's name, which the sentence right beside it already says. -->
        <span class="contents" aria-hidden="true">
          <BrandMark path={mark.path} hex={mark.hex} title={mark.title} class="size-3.5 shrink-0" />
        </span>
      {/if}
      <span>
        As published by <span class="capitalize">{form.provider}</span>
        {#if model.total > 0}
          &middot; {model.total}
          {model.total === 1 ? 'question' : 'questions'}
          <!-- A zero is never printed. "0 written answers" states a cost that does not
               exist; the model counts it honestly and the decision not to say it is here. -->
          {#if model.written > 0}
            &middot; {model.written}
            {model.written === 1 ? 'written answer' : 'written answers'}
          {/if}
        {/if}
      </span>
    </p>

    {#if form.basics?.length}
      <!-- The controls every application demands, said once. Listed rather than dropped:
           a form that does NOT want a CV is worth knowing too. Headed only when the
           questions below are headed, so it is never the one labelled block on the page. -->
      <div class="flex max-w-3xl flex-col gap-1">
        {#if model.headed}
          <h2 class="text-xs font-medium text-muted-foreground">Basics</h2>
        {/if}
        <p class="text-sm text-muted-foreground">{form.basics.join(', ')}</p>
      </div>
    {/if}

    <!-- Grouped by what answering one costs, cheapest first, so a reader meets the
         one-line questions before the essays and can stop as soon as the form is more
         than they will spend. The kind of answer is stated once by the heading instead
         of once per row, which is what used to wrap onto a second line and read as a
         list item of its own.
         Ordering within a group is the employer's, as served. Between groups it is
         ours — a deliberate trade, since the form is actually filled on the platform's
         own site where the platform's order governs. -->
    {#each model.groups as group (group.key)}
      <div class="flex max-w-3xl flex-col gap-1">
        {#if model.headed}
          <h2 class="text-xs font-medium text-muted-foreground">
            {GROUP_LABELS[group.key]} ({group.questions.length})
          </h2>
        {/if}
        <!-- Keyed on index, not on the question text. The text is the employer's, verbatim, and
             real ATS forms repeat it — Greenhouse and Workable both publish the same screening
             question twice on some postings. A duplicate key throws each_key_duplicate during
             Svelte 5 hydration rather than warning, which took the whole job page down. The list
             is inert (rebuilt wholesale when `form` changes, never reordered or filtered in
             place), so position is a sound identity here — in each group as it was in the one
             list this replaced. -->
        <ul class="flex flex-col gap-2">
          {#each group.questions as question, i (i)}
            <li class="text-sm">
              <span>{question.text}</span>
              <!-- The kind of answer is the heading's to say. What is left is whether the
                   platform will take the application without one; a required question is
                   the ordinary case and marking it would put a word on nearly every row
                   to report that nothing unusual is true of it. -->
              {#if !question.required}
                <span class="ml-2 whitespace-nowrap text-xs text-muted-foreground">optional</span>
              {/if}
            </li>
          {/each}
        </ul>
      </div>
    {/each}
  </section>
{/if}
