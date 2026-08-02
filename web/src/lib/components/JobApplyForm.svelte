<script lang="ts">
  import type { Display } from '$lib/generated/contracts';

  // What the application will ask, as the ATS published it. `form` is null for most
  // postings — only a few platforms publish a form we can read — and the block simply
  // does not render, which is the ordinary case rather than a degraded one.
  let { form }: { form: Display | null } = $props();

  // A form worth showing has either questions or a statement of what it wants uploaded.
  // Neither is a reason to render a heading over nothing.
  const worthShowing = $derived(
    !!form && (form.questions?.length > 0 || form.basics?.length > 0)
  );
</script>

{#if form && worthShowing}
  <section class="mt-6 rounded-lg border border-border bg-card p-4">
    <div class="mb-3 flex items-baseline justify-between gap-3">
      <h2 class="text-sm font-semibold tracking-tight">What this application asks</h2>
      <span class="text-xs text-muted-foreground capitalize">{form.provider}</span>
    </div>

    {#if form.basics?.length}
      <!-- The controls every application demands, said once. Listed rather than dropped:
           a form that does NOT want a CV is worth knowing too. -->
      <p class="mb-2 max-w-3xl text-sm text-muted-foreground">
        {form.basics.join(', ')}
      </p>
    {/if}

    {#if form.questions?.length}
      <!-- The hint follows the question rather than being pinned to the right edge. On a
           wide viewport that pinning left ~900px of empty space between a question and the
           word describing its answer, which is a long way to travel to read one line. -->
      <ul class="flex max-w-3xl flex-col gap-2">
        {#each form.questions as question (question.text)}
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
