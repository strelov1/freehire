<script lang="ts">
  import type { Answers } from '$lib/generated/contracts';
  import { api } from '$lib/api';
  import { Button, Input } from '$lib/ui';
  import { messages } from './ScreeningAnswersForm.messages';
  import { locale } from '$lib/i18n/currentLocale.svelte';
  import { format, t } from '$lib/i18n/t';

  // Editable form for the six screening questions that repeat across ATS application
  // forms (work authorization, visa sponsorship, desired salary, notice period,
  // willingness to relocate, 18+) — see internal/screeninganswers/AGENTS.md. Standalone
  // section on the profile page's Settings tab, its own heading and its own save action,
  // deliberately separate from ProfileForm (a different lifecycle: these are facts the
  // candidate states directly, not a CV/skills targeting profile).
  let { answers = null, onSaved }: { answers?: Answers | null; onSaved?: () => void } = $props();

  const s = $derived(t(messages, locale()));

  // Tri-state: a boolean field can be unset (the candidate has not said), yes, or no. A
  // native select keeps that third state explicit instead of defaulting a checkbox to
  // "no" for someone who has simply never answered.
  type TriState = '' | 'yes' | 'no';
  function toTriState(b: boolean | undefined): TriState {
    if (b === true) return 'yes';
    if (b === false) return 'no';
    return '';
  }
  function fromTriState(tri: TriState): boolean | undefined {
    if (tri === 'yes') return true;
    if (tri === 'no') return false;
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
      throw new Error(format(s.mustBeNumber, { field: fieldLabel }));
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
      if (desiredSalaryAmount.trim() !== '') patch.desired_salary_amount = parseWholeNumber(desiredSalaryAmount, s.fields.desiredSalaryAmount);
      if (desiredSalaryCurrency.trim() !== '') patch.desired_salary_currency = desiredSalaryCurrency.trim();
      if (desiredSalaryPeriod !== '') patch.desired_salary_period = desiredSalaryPeriod;
      if (noticePeriodDays.trim() !== '') patch.notice_period_days = parseWholeNumber(noticePeriodDays, s.fields.noticePeriod);
      const relocate = fromTriState(willingToRelocate);
      if (relocate !== undefined) patch.willing_to_relocate = relocate;
      const age = fromTriState(age18OrOlder);
      if (age !== undefined) patch.age_18_or_older = age;

      await api.updateScreeningAnswers(patch);
      note = s.saved;
      onSaved?.();
    } catch (e) {
      error = e instanceof Error ? e.message : s.saveFailed;
      // The save never landed — these values are still unsaved and must stay protected.
      dirty = true;
    } finally {
      busy = false;
    }
  }
</script>

<section class="flex flex-col gap-4">
  <div class="flex flex-col gap-1">
    <h3 class="text-sm font-semibold">{s.heading}</h3>
    <p class="text-xs text-muted-foreground">
      {s.description}
    </p>
  </div>

  <div class="grid gap-3 sm:grid-cols-2">
    <label class="flex flex-col gap-1 text-sm sm:col-span-2">
      <span class="text-muted-foreground">{s.fields.authorizedCountries}</span>
      <Input bind:value={authorizedCountriesText} oninput={markDirty} placeholder={s.fields.authorizedCountriesPlaceholder} class="w-full" />
    </label>

    <label class="flex flex-col gap-1 text-sm">
      <span class="text-muted-foreground">{s.fields.visaSponsorship}</span>
      <select
        bind:value={visaSponsorshipNeeded}
        onchange={markDirty}
        class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
      >
        <option value="">{s.triState.notStated}</option>
        <option value="yes">{s.triState.yes}</option>
        <option value="no">{s.triState.no}</option>
      </select>
    </label>

    <label class="flex flex-col gap-1 text-sm">
      <span class="text-muted-foreground">{s.fields.relocate}</span>
      <select
        bind:value={willingToRelocate}
        onchange={markDirty}
        class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
      >
        <option value="">{s.triState.notStated}</option>
        <option value="yes">{s.triState.yes}</option>
        <option value="no">{s.triState.no}</option>
      </select>
    </label>

    <label class="flex flex-col gap-1 text-sm">
      <span class="text-muted-foreground">{s.fields.age18}</span>
      <select
        bind:value={age18OrOlder}
        onchange={markDirty}
        class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
      >
        <option value="">{s.triState.notStated}</option>
        <option value="yes">{s.triState.yes}</option>
        <option value="no">{s.triState.no}</option>
      </select>
    </label>

    <label class="flex flex-col gap-1 text-sm">
      <span class="text-muted-foreground">{s.fields.noticePeriod}</span>
      <!-- inputmode, not type="number": the design-system Input's `value` is typed (and
           bound) as a string, but Svelte's bind:value on a native type="number" input
           coerces to a JS number regardless — desiredSalaryAmount.trim() below would
           throw. inputmode still gets the numeric keyboard on mobile without that trap. -->
      <Input inputmode="numeric" bind:value={noticePeriodDays} oninput={markDirty} placeholder="30" class="w-full" />
    </label>

    <label class="flex flex-col gap-1 text-sm">
      <span class="text-muted-foreground">{s.fields.desiredSalaryAmount}</span>
      <Input inputmode="numeric" bind:value={desiredSalaryAmount} oninput={markDirty} placeholder="120000" class="w-full" />
    </label>

    <label class="flex flex-col gap-1 text-sm">
      <span class="text-muted-foreground">{s.fields.currency}</span>
      <Input bind:value={desiredSalaryCurrency} oninput={markDirty} placeholder="USD" maxlength={3} class="w-full" />
    </label>

    <label class="flex flex-col gap-1 text-sm">
      <span class="text-muted-foreground">{s.fields.salaryPeriod}</span>
      <select
        bind:value={desiredSalaryPeriod}
        onchange={markDirty}
        class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
      >
        <option value="">{s.triState.notStated}</option>
        <option value="year">{s.periods.year}</option>
        <option value="month">{s.periods.month}</option>
        <option value="day">{s.periods.day}</option>
        <option value="hour">{s.periods.hour}</option>
      </select>
    </label>
  </div>

  <div class="flex flex-wrap items-center gap-2">
    <Button size="sm" variant="primary" disabled={busy} onclick={save}>{s.save}</Button>
  </div>
  {#if error}
    <p class="text-sm text-destructive">{error}</p>
  {:else if note}
    <p class="text-xs text-muted-foreground">{note}</p>
  {/if}
</section>
