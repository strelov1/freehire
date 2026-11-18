<script lang="ts">
  // The onboarding flow's three questions, one at a time: a row of progress chips (each
  // answered one names its answer and reopens the question when tapped), then the open
  // question. Where and role answer on one tap; "what's in your way" takes several picks
  // and a Continue, nothing picked included. What the answers mean lives in
  // #lib/onboardingFlow and #lib/onboardingChallenge — this only asks.
  import { Check } from '@lucide/svelte';
  import { toggleChallenge } from '#lib/onboardingChallenge.js';
  import {
    answerLabel,
    QUESTIONS,
    roleChoices,
    whereChoices,
    type Answers,
    type Choice,
    type QuestionKey,
  } from '#lib/onboardingFlow.js';
  import { JOB_CHALLENGE_OPTIONS, JOB_CHALLENGE_OTHER } from '#lib/surveyOptions.js';
  import type { OnboardingParse } from '#lib/types.js';
  import { Button } from '#lib/ui/index.js';

  let {
    parse,
    answers,
    current,
    onAnswer,
    onOpen,
    roleCounts,
  }: {
    parse: OnboardingParse;
    answers: Answers;
    /** The question on screen. */
    current: QuestionKey;
    /** One answer (or, for the challenge, its picks and note) given. */
    onAnswer: (patch: Partial<Answers>) => void;
    /** Reopen an answered question. */
    onOpen: (key: QuestionKey) => void;
    /** Open jobs per category for the CV's skills; until it arrives the roles are the CV's own. */
    roleCounts?: Record<string, number>;
  } = $props();

  const name = $derived(parse.first_name ? `, ${parse.first_name}` : '');
  const titles = $derived<Record<QuestionKey, string>>({
    where: `Where do you want to work${name}?`,
    role: `What's next for you${name}?`,
    challenge: "What's in your way right now?",
  });
  const choices = $derived<Record<'where' | 'role', Choice[]>>({
    where: whereChoices(parse),
    role: roleChoices(parse, roleCounts),
  });

  // The challenge picks until Continue; reopened, they start from the answer given.
  // svelte-ignore state_referenced_locally
  let picked = $state<string[]>(answers.challenge ?? []);
  // svelte-ignore state_referenced_locally
  let note = $state(answers.challengeNote ?? '');
  const wantsNote = $derived(picked.includes(JOB_CHALLENGE_OTHER));
  const position = $derived(QUESTIONS.findIndex((q) => q.key === current) + 1);

  // The tapped choice disappears with its question, so focus would fall to the page; it
  // goes to the next question's heading instead. Only after an answer — the first question
  // appearing is not the visitor's doing.
  let justAnswered = false;
  function pick(key: 'where' | 'role', value: string) {
    justAnswered = true;
    onAnswer({ [key]: value });
  }
  function continueChallenge() {
    justAnswered = true;
    // A plain copy: `picked` is a $state proxy, which the draft's IndexedDB cannot store.
    onAnswer({ challenge: $state.snapshot(picked), challengeNote: wantsNote ? note.trim() : '' });
  }
  function focusAfterAnswer(el: HTMLElement) {
    if (!justAnswered) return;
    justAnswered = false;
    el.focus();
  }

  function chipText(key: QuestionKey, chip: string): string {
    if (answers[key] === undefined) return chip;
    return answerLabel(parse, answers, key) ?? 'No preference';
  }
</script>

<div class="flex flex-col gap-4">
  <ol class="flex flex-wrap gap-2" aria-label="Questions">
    {#each QUESTIONS as q (q.key)}
      {@const answered = answers[q.key] !== undefined}
      {@const isCurrent = q.key === current}
      <li>
        <button
          type="button"
          disabled={!answered || isCurrent}
          aria-current={isCurrent ? 'step' : undefined}
          onclick={() => onOpen(q.key)}
          class={[
            'inline-flex items-center gap-1 rounded-full border px-3 py-1 text-xs font-medium transition-colors',
            isCurrent && 'border-brand bg-brand text-brand-foreground',
            !isCurrent && answered && 'border-border bg-card hover:bg-accent',
            !isCurrent && !answered && 'border-transparent bg-muted text-muted-foreground',
          ]}
        >
          {#if answered && !isCurrent}<Check class="size-3 text-success-strong" aria-hidden="true" />{/if}
          {chipText(q.key, q.chip)}
        </button>
      </li>
    {/each}
  </ol>

  <div class="rounded-xl border border-border bg-background p-4 sm:p-5">
    <div class="flex items-baseline justify-between gap-3">
      {#key current}
        <h3
          tabindex="-1"
          {@attach focusAfterAnswer}
          class="text-lg font-semibold tracking-tight text-balance outline-none"
        >
          {titles[current]}
        </h3>
      {/key}
      <span class="shrink-0 text-xs font-medium tabular-nums text-muted-foreground">{position}/{QUESTIONS.length}</span>
    </div>

    {#if current === 'challenge'}
      <p class="mt-1 text-sm text-muted-foreground text-pretty">Pick all that apply, or none.</p>
      <div role="group" aria-label="What's in your way" class="mt-4 flex flex-wrap gap-2">
        {#each JOB_CHALLENGE_OPTIONS as o (o.value)}
          {@const on = picked.includes(o.value)}
          <button
            type="button"
            aria-pressed={on}
            onclick={() => (picked = toggleChallenge(picked, o.value))}
            class={[
              'rounded-full border px-4 py-2 text-sm font-medium transition-colors active:translate-y-px',
              on ? 'border-brand bg-brand text-brand-foreground' : 'border-border bg-card hover:bg-accent',
            ]}
          >
            {o.label}
          </button>
        {/each}
      </div>
      {#if wantsNote}
        <input
          bind:value={note}
          maxlength={300}
          aria-label="Tell us in your own words"
          placeholder="Something else? Tell us in a line (optional)"
          class="mt-3 w-full rounded-md border border-input bg-card px-3 py-2 text-sm outline-none placeholder:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring"
        />
      {/if}
      <div class="mt-4 flex justify-end">
        <Button variant="primary" onclick={continueChallenge}>Continue</Button>
      </div>
    {:else}
      <div class="mt-4 flex flex-wrap gap-2">
        {#each choices[current] as c (c.id)}
          {@const isPicked = answers[current] === c.id}
          <button
            type="button"
            aria-pressed={isPicked}
            onclick={() => pick(current, c.id)}
            class={[
              'rounded-full border px-4 py-2 text-sm font-medium transition-colors active:translate-y-px',
              isPicked ? 'border-brand bg-brand text-brand-foreground' : 'border-border bg-card hover:bg-accent',
            ]}
          >
            {c.label}
            {#if c.count !== undefined}
              <span class={['ml-1 text-xs tabular-nums', isPicked ? 'opacity-80' : 'text-muted-foreground']}
                >{c.count.toLocaleString('en')} jobs</span
              >
            {/if}
          </button>
        {/each}
      </div>
    {/if}
  </div>
</div>
