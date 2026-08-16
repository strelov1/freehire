<script lang="ts">
  import type { Answers } from '$lib/generated/contracts';
  import { api } from '$lib/api';
  import { Button, Input } from '$lib/ui';

  // Editable form for the six screening questions that repeat across ATS application
  // forms (work authorization, visa sponsorship, desired salary, notice period,
  // willingness to relocate, 18+) — see internal/screeninganswers/AGENTS.md. Standalone
  // section on the profile page's Settings tab, its own heading and its own save action,
  // deliberately separate from ProfileForm (a different lifecycle: these are facts the
  // candidate states directly, not a CV/skills targeting profile).
  let { answers = null, onSaved }: { answers?: Answers | null; onSaved?: () => void } = $props();

  // Tri-state: a boolean field can be unset (the candidate has not said), yes, or no. A
  // native select keeps that third state explicit instead of defaulting a checkbox to
  // "no" for someone who has simply never answered.
  type TriState = '' | 'yes' | 'no';
  function toTriState(b: boolean | undefined): TriState {
    if (b === true) return 'yes';
    if (b === false) return 'no';
    return '';
  }
  function fromTriState(t: TriState): boolean | undefined {
    if (t === 'yes') return true;
    if (t === 'no') return false;
    return undefined;
  }

  // Number(...) alone silently turns anything unparseable (a stray letter, or a very
  // natural "120,000" with a thousands separator) into NaN, which JSON.stringify then
  // serializes as null — indistinguishable on the wire from the field being omitted, so
  // the server would leave the stored value untouched while this form still reports
  // success. Strip thousands separators first, then fail loudly rather than silently
  // dropping the field the candidate typed a value into.
  function parseWholeNumber(raw: string, fieldLabel: string): number {
    const n = Number(raw.replace(/,/g, ''));
    if (!Number.isFinite(n)) {
      throw new Error(`${fieldLabel} must be a number.`);
    }
    return n;
  }

  let authorizedCountriesText = $state((answers?.authorized_countries ?? []).join(', '));
  let visaSponsorshipNeeded = $state<TriState>(toTriState(answers?.visa_sponsorship_needed));
  let desiredSalaryAmount = $state(answers?.desired_salary_amount?.toString() ?? '');
  let desiredSalaryCurrency = $state(answers?.desired_salary_currency ?? '');
  let desiredSalaryPeriod = $state(answers?.desired_salary_period ?? '');
  let noticePeriodDays = $state(answers?.notice_period_days?.toString() ?? '');
  let willingToRelocate = $state<TriState>(toTriState(answers?.willing_to_relocate));
  let age18OrOlder = $state<TriState>(toTriState(answers?.age_18_or_older));

  let busy = $state(false);
  let error = $state<string | null>(null);
  let note = $state<string | null>(null);
  // Set by any keystroke, cleared right before a save request is sent — same guard
  // CandidateContactsEditor uses so a reload after this component's own save (the parent
  // re-fetches and passes a new `answers` prop) never overwrites an edit not yet saved.
  let dirty = $state(false);

  $effect(() => {
    if (dirty) return;
    authorizedCountriesText = (answers?.authorized_countries ?? []).join(', ');
    visaSponsorshipNeeded = toTriState(answers?.visa_sponsorship_needed);
    desiredSalaryAmount = answers?.desired_salary_amount?.toString() ?? '';
    desiredSalaryCurrency = answers?.desired_salary_currency ?? '';
    desiredSalaryPeriod = answers?.desired_salary_period ?? '';
    noticePeriodDays = answers?.notice_period_days?.toString() ?? '';
    willingToRelocate = toTriState(answers?.willing_to_relocate);
    age18OrOlder = toTriState(answers?.age_18_or_older);
  });

  function markDirty() {
    dirty = true;
  }

  // Only fields with a value in the form are sent — the server's partial-update contract
  // leaves an omitted field exactly as stored, so a blank input here means "unchanged",
  // not "clear it" (there is no clear operation; see the Store.Update doc comment).
  async function save() {
    busy = true;
    error = null;
    note = null;
    dirty = false;
    // Patch construction lives inside the try too: a throw here left busy stuck at true
    // forever in an earlier version (nothing downstream to reset it), which read as a
    // permanently-disabled button with no error shown — worse than the error path itself.
    try {
      const patch: Partial<Answers> = {};
      const countries = authorizedCountriesText
        .split(',')
        .map((c) => c.trim())
        .filter(Boolean);
      if (countries.length > 0) patch.authorized_countries = countries;
      const sponsorship = fromTriState(visaSponsorshipNeeded);
      if (sponsorship !== undefined) patch.visa_sponsorship_needed = sponsorship;
      if (desiredSalaryAmount.trim() !== '') patch.desired_salary_amount = parseWholeNumber(desiredSalaryAmount, 'Desired salary amount');
      if (desiredSalaryCurrency.trim() !== '') patch.desired_salary_currency = desiredSalaryCurrency.trim();
      if (desiredSalaryPeriod !== '') patch.desired_salary_period = desiredSalaryPeriod;
      if (noticePeriodDays.trim() !== '') patch.notice_period_days = parseWholeNumber(noticePeriodDays, 'Notice period');
      const relocate = fromTriState(willingToRelocate);
      if (relocate !== undefined) patch.willing_to_relocate = relocate;
      const age = fromTriState(age18OrOlder);
      if (age !== undefined) patch.age_18_or_older = age;

      await api.updateScreeningAnswers(patch);
      note = 'Screening answers saved.';
      onSaved?.();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Could not save screening answers.';
      // The save never landed — these values are still unsaved and must stay protected.
      dirty = true;
    } finally {
      busy = false;
    }
  }
</script>

<section class="flex flex-col gap-4">
  <div class="flex flex-col gap-1">
    <h3 class="text-sm font-semibold">Screening answers</h3>
    <p class="text-xs text-muted-foreground">
      Answer these once and the browser extension can fill them into matching questions on real
      application forms.
    </p>
  </div>

  <div class="grid gap-3 sm:grid-cols-2">
    <label class="flex flex-col gap-1 text-sm sm:col-span-2">
      <span class="text-muted-foreground">Authorized to work in (comma-separated countries)</span>
      <Input bind:value={authorizedCountriesText} oninput={markDirty} placeholder="United States, Germany" class="w-full" />
    </label>

    <label class="flex flex-col gap-1 text-sm">
      <span class="text-muted-foreground">Need visa sponsorship?</span>
      <select
        bind:value={visaSponsorshipNeeded}
        onchange={markDirty}
        class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
      >
        <option value="">Not stated</option>
        <option value="yes">Yes</option>
        <option value="no">No</option>
      </select>
    </label>

    <label class="flex flex-col gap-1 text-sm">
      <span class="text-muted-foreground">Willing to relocate?</span>
      <select
        bind:value={willingToRelocate}
        onchange={markDirty}
        class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
      >
        <option value="">Not stated</option>
        <option value="yes">Yes</option>
        <option value="no">No</option>
      </select>
    </label>

    <label class="flex flex-col gap-1 text-sm">
      <span class="text-muted-foreground">18 or older?</span>
      <select
        bind:value={age18OrOlder}
        onchange={markDirty}
        class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
      >
        <option value="">Not stated</option>
        <option value="yes">Yes</option>
        <option value="no">No</option>
      </select>
    </label>

    <label class="flex flex-col gap-1 text-sm">
      <span class="text-muted-foreground">Notice period (days)</span>
      <!-- inputmode, not type="number": the design-system Input's `value` is typed (and
           bound) as a string, but Svelte's bind:value on a native type="number" input
           coerces to a JS number regardless — desiredSalaryAmount.trim() below would
           throw. inputmode still gets the numeric keyboard on mobile without that trap. -->
      <Input inputmode="numeric" bind:value={noticePeriodDays} oninput={markDirty} placeholder="30" class="w-full" />
    </label>

    <label class="flex flex-col gap-1 text-sm">
      <span class="text-muted-foreground">Desired salary amount</span>
      <Input inputmode="numeric" bind:value={desiredSalaryAmount} oninput={markDirty} placeholder="120000" class="w-full" />
    </label>

    <label class="flex flex-col gap-1 text-sm">
      <span class="text-muted-foreground">Currency (ISO 4217)</span>
      <Input bind:value={desiredSalaryCurrency} oninput={markDirty} placeholder="USD" maxlength={3} class="w-full" />
    </label>

    <label class="flex flex-col gap-1 text-sm">
      <span class="text-muted-foreground">Salary period</span>
      <select
        bind:value={desiredSalaryPeriod}
        onchange={markDirty}
        class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
      >
        <option value="">Not stated</option>
        <option value="year">Year</option>
        <option value="month">Month</option>
        <option value="day">Day</option>
        <option value="hour">Hour</option>
      </select>
    </label>
  </div>

  <div class="flex flex-wrap items-center gap-2">
    <Button size="sm" variant="primary" disabled={busy} onclick={save}>Save screening answers</Button>
  </div>
  {#if error}
    <p class="text-sm text-destructive">{error}</p>
  {:else if note}
    <p class="text-xs text-muted-foreground">{note}</p>
  {/if}
</section>
