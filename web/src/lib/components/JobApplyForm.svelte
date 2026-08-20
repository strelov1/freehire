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
  let { form }: { form: Display | null } = $props();
</script>

{#if applyFormWorthShowing(form)}
  <section class="flex flex-col gap-3">
    <!-- The provider is the whole caption here. It was a chip beside a heading before
         this block moved into a tab; the tab now says what it is, so what is left worth
         saying is whose form it is, verbatim from the ATS. -->
    <p class="text-xs text-muted-foreground">
      As published by <span class="capitalize">{form.provider}</span>
    </p>

    {#if form.basics?.length}
      <!-- The controls every application demands, said once. Listed rather than dropped:
           a form that does NOT want a CV is worth knowing too. -->
      <p class="max-w-3xl text-sm text-muted-foreground">
        {form.basics.join(', ')}
      </p>
    {/if}

    {#if form.questions?.length}
      <!-- The hint follows the question rather than being pinned to the right edge. On a
           wide viewport that pinning left ~900px of empty space between a question and the
           word describing its answer, which is a long way to travel to read one line. -->
      <!-- Keyed on index, not on the question text. The text is the employer's, verbatim, and
           real ATS forms repeat it — Greenhouse and Workable both publish the same screening
           question twice on some postings. A duplicate key throws each_key_duplicate during
           Svelte 5 hydration rather than warning, which took the whole job page down. The list
           is inert (replaced wholesale when `form` changes, never reordered or filtered), so
           position is a sound identity here. -->
      <ul class="flex max-w-3xl flex-col gap-2">
        {#each form.questions as question, i (i)}
          <li class="text-sm">
            <span>{question.text}</span>
            {#if question.answer || !question.required}
              <span class="ml-2 whitespace-nowrap text-xs text-muted-foreground">
                {#if question.answer}{question.answer}{/if}
                {#if question.answer && !question.required}&middot;{/if}
                {#if !question.required}optional{/if}
              </span>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  </section>
{/if}
