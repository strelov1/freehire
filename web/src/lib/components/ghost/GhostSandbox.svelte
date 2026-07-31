<script lang="ts">
  import { SvelteSet } from 'svelte/reactivity';
  import GhostBadge from '$lib/components/GhostBadge.svelte';
  import GhostChecklist from '$lib/components/GhostChecklist.svelte';
  import { CRITERIA, WITNESS_GATE, ghostLevel } from '$lib/ghost';
  import type { GhostCriterionCode } from '$lib/ghost';
  import type { Ghost } from '$lib/generated/contracts';

  // The preview the reader drives. Below it stand the SAME components the job page
  // renders — not a copy of their markup, which would go stale the first time they are
  // redesigned, and not a screenshot, which additionally freezes one theme and one
  // employer's name.
  //
  // Its job is to let the reader FAIL to reach the strongest wording with posting shape
  // alone. Asserting that in a sentence is what the page did before, and a sentence is
  // exactly what a reader who has just been surprised by a badge does not believe.
  // SvelteSet rather than a plain Set reassigned on every change: it is the reactive
  // collection the framework ships, so `toggle` mutates in place and the deriveds below
  // still fire.
  const fired = new SvelteSet<GhostCriterionCode>(['evergreen_posting', 'ats_absent']);
  let contributors = $state(0);

  const outcomeSelected = $derived(
    CRITERIA.some((c) => c.tier === 'outcome' && fired.has(c.code)),
  );

  // Below the gate the contributor count is ABSENT from the payload rather than zeroed:
  // a count of one identifies that applicant to the employer. The assembled payload has
  // to reproduce that, or the preview would show a system that holds the number back
  // instead of never having one to serve.
  const ghost = $derived.by((): Ghost | null => {
    const criteria = CRITERIA.map((c) => c.code).filter((code) => fired.has(code));
    const level = ghostLevel(criteria, contributors);
    if (level === 'none') return null;

    return {
      level,
      criteria,
      criteria_total: CRITERIA.length,
      ...(contributors >= WITNESS_GATE ? { contributors } : {}),
      ats_checked_at: new Date(Date.now() - 2 * 86_400_000).toISOString(),
    };
  });

  // Contributors and outcome criteria imply each other in the real system: every row that
  // fires an outcome criterion records the person behind it, so neither can exist alone.
  // The guard runs BOTH ways — switching the last outcome criterion off drops the count,
  // and switching the first one on raises it to one — because a panel reading "applications
  // went unanswered" beside "nobody reported" describes a payload that cannot exist, on a
  // preview whose whole claim is that it cannot lie.
  function toggle(code: GhostCriterionCode) {
    if (fired.has(code)) fired.delete(code);
    else fired.add(code);

    const anyOutcome = CRITERIA.some((c) => c.tier === 'outcome' && fired.has(c.code));
    if (!anyOutcome) contributors = 0;
    else if (contributors === 0) contributors = 1;
  }

  const STEPS = [
    { value: 0, label: 'nobody' },
    { value: 1, label: '1 person' },
    { value: WITNESS_GATE, label: `${WITNESS_GATE} people` },
  ];
</script>

<div class="flex flex-col gap-5 rounded-xl border border-border p-5">
  <div class="grid gap-5 sm:grid-cols-2">
    {#each [{ tier: 'structural', heading: 'The shape of the posting' }, { tier: 'outcome', heading: 'What happened to applicants' }] as group (group.tier)}
      <fieldset class="flex flex-col gap-2">
        <legend class="mb-2 text-xs font-medium tracking-tight">{group.heading}</legend>
        {#each CRITERIA.filter((c) => c.tier === group.tier) as c (c.code)}
          <label class="flex cursor-pointer items-center gap-2 text-sm text-muted-foreground">
            <input
              type="checkbox"
              class="accent-warning"
              checked={fired.has(c.code)}
              onchange={() => toggle(c.code)}
            />
            {c.label}
          </label>
        {/each}
      </fieldset>
    {/each}
  </div>

  <fieldset class="flex flex-wrap items-center gap-x-4 gap-y-2 border-t border-border pt-4">
    <legend class="sr-only">How many distinct people contributed outcome evidence</legend>
    <span class="text-xs font-medium">Different people who reported</span>
    {#each STEPS as step (step.value)}
      <label
        class="flex items-center gap-1.5 text-sm {outcomeSelected
          ? 'cursor-pointer text-muted-foreground'
          : 'cursor-not-allowed text-muted-foreground/50'}"
      >
        <input
          type="radio"
          class="accent-warning"
          name="contributors"
          value={step.value}
          checked={contributors === step.value}
          disabled={!outcomeSelected}
          onchange={() => (contributors = step.value)}
        />
        {step.label}
      </label>
    {/each}
    {#if !outcomeSelected}
      <span class="basis-full text-xs text-muted-foreground">
        People arrive with outcome evidence — switch one on to move this.
      </span>
    {/if}
  </fieldset>

  <div class="rounded-lg border border-border bg-secondary/20 p-4">
    {#if ghost}
      <!-- Two surfaces, labelled as two surfaces. Stacked unlabelled they read as one
           screen saying the same thing twice — which is the arrangement the job page
           deliberately dropped, so showing it here would advertise a layout the product
           does not have. -->
      <div class="flex flex-col gap-4">
        <div class="flex flex-col gap-1.5">
          <span class="text-[10px] uppercase tracking-wider text-muted-foreground">
            In a list, on the card
          </span>
          <div><GhostBadge {ghost} /></div>
        </div>
        <div class="flex flex-col gap-1.5 border-t border-border pt-3">
          <span class="text-[10px] uppercase tracking-wider text-muted-foreground">
            On the job page
          </span>
          <GhostChecklist {ghost} />
        </div>
      </div>
    {:else}
      <p class="text-sm text-muted-foreground">
        Nothing is shown on the job. Neither gate has passed — a lone criterion with
        nobody behind it is too ordinary to mean anything.
      </p>
    {/if}
  </div>
</div>
