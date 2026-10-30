<script lang="ts">
  import { untrack } from 'svelte';
  import { COUNTRY_REGION_MAP, type Answers } from '#lib/generated/contracts.js';
  import { COUNTRY_OPTIONS } from '#lib/facets.js';
  import { Input } from '#lib/ui/index.js';
  import { SaveStatus } from '#lib/saveStatus.svelte.js';
  import { screeningStore } from '#lib/screening.svelte.js';
  import { TextAutosave } from '#lib/textAutosave.js';
  import SaveStatusLine from './SaveStatus.svelte';
  import SearchSelect from './facets/SearchSelect.svelte';
  import { messages } from './ScreeningAnswersForm.messages';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, t } from '#lib/i18n/t.js';

  // Editable form for the six screening questions that repeat across ATS application
  // forms (work authorization, visa sponsorship, desired salary, notice period,
  // willingness to relocate, 18+) plus the standing privacy consent — see internal/screeninganswers/AGENTS.md. Standalone
  // section on the profile's Screening answers tab, deliberately separate from ProfileForm
  // (a different lifecycle: these are facts the candidate states directly, not a
  // CV/skills targeting profile).
  //
  // It autosaves like every profile card (TextAutosave): a typed field a second after
  // typing stops or when focus leaves the form, a dropdown at once. Auto-apply sends these
  // answers to real employers, so a number field that does not read as a number is marked
  // and not saved. Countries are picked from a list rather than typed: the server accepts
  // only the countries its own dictionary places (screeninganswers.Validate), and a typed
  // name it did not know — "Германия", the Russian placeholder's own example — failed the
  // whole save, dropdowns included.
  let { answers = null }: { answers?: Answers | null } = $props();

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

  // Only plain decimal digits, or digits grouped by commas in threes ("120,000"), are read
  // as a number; anything else is NaN and refused. Number() on its own is too generous:
  // it reads "0x10" as 16 and "1e5" as 100000, and silently turns "120,000" into NaN, which
  // JSON.stringify sends as null — an omitted field the server leaves untouched while the
  // form looked saved. And "120,00" is a typo, not 12000.
  const wholeNumber = (raw: string) =>
    /^\d+$/.test(raw) || /^\d{1,3}(,\d{3})+$/.test(raw) ? Number(raw.replace(/,/g, '')) : Number.NaN;

  // The picker offers only the countries the server accepts. The site-wide list is every
  // ISO code; COUNTRY_REGION_MAP is generated from the server's own table, so the two
  // cannot drift.
  const acceptedCountries = COUNTRY_OPTIONS.filter((o) => o.value in COUNTRY_REGION_MAP);

  // The site-wide labels are English. The form shows each country in the reader's own
  // language — a fuzzy "Германия" also surfaces Армения, and beside English labels that
  // is easy to pick by mistake — and keeps the English name as a search alias.
  const countryOptions = $derived.by(() => {
    const names = new Intl.DisplayNames([locale()], { type: 'region' });
    return acceptedCountries.map((o) => ({ ...o, label: names.of(o.value.toUpperCase()) ?? o.label }));
  });
  const countryAliases = Object.fromEntries(acceptedCountries.map((o) => [o.value, [o.label]]));

  let authorizedCountries = $state<string[]>(answers?.authorized_countries ?? []);
  let visaSponsorshipNeeded = $state<TriState>(toTriState(answers?.visa_sponsorship_needed));
  let desiredSalaryAmount = $state(answers?.desired_salary_amount?.toString() ?? '');
  let desiredSalaryCurrency = $state(answers?.desired_salary_currency ?? '');
  let desiredSalaryPeriod = $state(answers?.desired_salary_period ?? '');
  let noticePeriodDays = $state(answers?.notice_period_days?.toString() ?? '');
  let willingToRelocate = $state<TriState>(toTriState(answers?.willing_to_relocate));
  let age18OrOlder = $state<TriState>(toTriState(answers?.age_18_or_older));
  let privacyConsent = $state<TriState>(toTriState(answers?.privacy_consent));
  let salaryInvalid = $state(false);
  let noticeInvalid = $state(false);
  let currencyInvalid = $state(false);

  // Only fields with a value go into the patch. The server leaves an omitted field as
  // stored, so an answer emptied here is sent separately, by name, to be withdrawn (see
  // withdrawn below). The stored answers go through the same shape, so the two compare
  // field for field.
  function patchOf(v: {
    countries: string[];
    sponsorship?: boolean;
    salary: string;
    currency: string;
    period: string;
    notice: string;
    relocate?: boolean;
    age?: boolean;
    consent?: boolean;
  }): Partial<Answers> {
    const patch: Partial<Answers> = {};
    if (v.countries.length > 0) patch.authorized_countries = v.countries;
    if (v.sponsorship !== undefined) patch.visa_sponsorship_needed = v.sponsorship;
    if (v.salary !== '') patch.desired_salary_amount = wholeNumber(v.salary);
    if (v.currency !== '') patch.desired_salary_currency = v.currency;
    if (v.period !== '') patch.desired_salary_period = v.period;
    if (v.notice !== '') patch.notice_period_days = wholeNumber(v.notice);
    if (v.relocate !== undefined) patch.willing_to_relocate = v.relocate;
    if (v.age !== undefined) patch.age_18_or_older = v.age;
    if (v.consent !== undefined) patch.privacy_consent = v.consent;
    return patch;
  }

  const read = () =>
    patchOf({
      countries: authorizedCountries,
      sponsorship: fromTriState(visaSponsorshipNeeded),
      salary: desiredSalaryAmount.trim(),
      currency: desiredSalaryCurrency.trim(),
      period: desiredSalaryPeriod,
      notice: noticePeriodDays.trim(),
      relocate: fromTriState(willingToRelocate),
      age: fromTriState(age18OrOlder),
      consent: fromTriState(privacyConsent),
    });

  const saved = () =>
    patchOf({
      countries: answers?.authorized_countries ?? [],
      sponsorship: answers?.visa_sponsorship_needed,
      salary: answers?.desired_salary_amount?.toString() ?? '',
      currency: answers?.desired_salary_currency ?? '',
      period: answers?.desired_salary_period ?? '',
      notice: answers?.notice_period_days?.toString() ?? '',
      relocate: answers?.willing_to_relocate,
      age: answers?.age_18_or_older,
      consent: answers?.privacy_consent,
    });

  // The endpoint's own rules (screeninganswers.Validate), applied here so a value it would
  // refuse is marked on its field instead of coming back as a failed save: a whole number,
  // and for the salary above zero. NaN fails Number.isInteger, so "12k" is caught too. A
  // currency is three letters; the server uppercases it.
  const wholeAtLeast = (value: number | undefined, minimum: number) =>
    value === undefined || (Number.isInteger(value) && value >= minimum);
  const salaryOk = (patch: Partial<Answers>) => wholeAtLeast(patch.desired_salary_amount, 1);
  const noticeOk = (patch: Partial<Answers>) => wholeAtLeast(patch.notice_period_days, 0);
  const currencyOk = (patch: Partial<Answers>) =>
    patch.desired_salary_currency === undefined || /^[A-Za-z]{3}$/.test(patch.desired_salary_currency);

  // The answers stored that this patch leaves out — the ones emptied here, to be withdrawn
  // rather than left as they were. The store asks it when the save is sent, so it sees
  // what an earlier save still in flight stored. An Undo re-sends the earlier patch
  // through the same path, so it withdraws an answer filled in for the first time too.
  const withdrawn = (patch: Partial<Answers>) =>
    (Object.keys(saved()) as (keyof Answers)[]).filter((k) => !(k in patch));

  const status = new SaveStatus();
  const autosave = new TextAutosave<Partial<Answers>>({
    status,
    read,
    saved,
    // Queued and session-guarded in the store; the PUT's own answer is the fresh copy, so
    // no re-read follows it and a failed one cannot hand the form stale answers.
    save: (patch) => screeningStore.save(patch, () => withdrawn(patch)),
    valid: (patch) => salaryOk(patch) && noticeOk(patch) && currencyOk(patch),
    // A number cannot be half-typed into something invalid — only a letter or a bad comma
    // makes it so — so it is marked after the pause too, not only on leaving the field. A
    // currency can be half-typed ("US"), so it is marked only on leaving (currencyLeft).
    refused: (patch) => {
      salaryInvalid = !salaryOk(patch);
      noticeInvalid = !noticeOk(patch);
    },
  });

  // Follow the stored answers when they change elsewhere (an Undo, another tab), only
  // while nothing typed here is unsent, and reading the boxes untracked so a keystroke
  // never re-runs it (see CandidateContactsEditor).
  $effect(() => {
    const stored = saved();
    if (autosave.holding) return;
    if (JSON.stringify(stored) === JSON.stringify(untrack(read))) return;
    authorizedCountries = answers?.authorized_countries ?? [];
    visaSponsorshipNeeded = toTriState(answers?.visa_sponsorship_needed);
    desiredSalaryAmount = answers?.desired_salary_amount?.toString() ?? '';
    desiredSalaryCurrency = answers?.desired_salary_currency ?? '';
    desiredSalaryPeriod = answers?.desired_salary_period ?? '';
    noticePeriodDays = answers?.notice_period_days?.toString() ?? '';
    willingToRelocate = toTriState(answers?.willing_to_relocate);
    age18OrOlder = toTriState(answers?.age_18_or_older);
    privacyConsent = toTriState(answers?.privacy_consent);
  });

  function edited() {
    salaryInvalid = false;
    noticeInvalid = false;
    autosave.edited();
  }

  function commit() {
    autosave.commit();
  }

  // A dropdown is a whole answer the moment it is picked; nothing to wait for.
  function picked() {
    edited();
    commit();
  }

  function toggleCountry(code: string) {
    authorizedCountries = authorizedCountries.includes(code)
      ? authorizedCountries.filter((c) => c !== code)
      : [...authorizedCountries, code];
    picked();
  }

  // The currency's mark is cleared only by editing the currency itself: while it stands
  // invalid every save is refused, so another answer changed meanwhile must not hide why.
  function currencyEdited() {
    currencyInvalid = false;
    edited();
  }

  function currencyLeft() {
    currencyInvalid = !currencyOk({ desired_salary_currency: desiredSalaryCurrency.trim() || undefined });
  }

  $effect(() => () => {
    autosave.commit(true);
  });
</script>

<section class="flex flex-col gap-4" onfocusout={commit}>
  <div class="flex items-start justify-between gap-3">
    <div class="flex flex-col gap-1">
      <h3 class="text-sm font-semibold">{s.heading}</h3>
      <p class="text-xs text-muted-foreground">
        {s.description}
      </p>
    </div>
    <SaveStatusLine {status} />
  </div>

  <div class="grid gap-3 sm:grid-cols-2">
    <div class="flex flex-col gap-1 text-sm sm:col-span-2">
      <span class="text-muted-foreground">{s.fields.authorizedCountries}</span>
      <SearchSelect
        options={countryOptions}
        include={authorizedCountries}
        placeholder={s.fields.authorizedCountriesPlaceholder}
        onToggle={toggleCountry}
        label={s.fields.authorizedCountries}
        searchAliases={countryAliases}
        cap={8}
        clearOnSelect
      />
    </div>

    <label class="flex flex-col gap-1 text-sm">
      <span class="text-muted-foreground">{s.fields.visaSponsorship}</span>
      <select
        bind:value={visaSponsorshipNeeded}
        onchange={picked}
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
        onchange={picked}
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
        onchange={picked}
        class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
      >
        <option value="">{s.triState.notStated}</option>
        <option value="yes">{s.triState.yes}</option>
        <option value="no">{s.triState.no}</option>
      </select>
    </label>

    <label class="flex flex-col gap-1 text-sm sm:col-span-2">
      <span class="text-muted-foreground">{s.fields.privacyConsent}</span>
      <select
        bind:value={privacyConsent}
        onchange={picked}
        class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
      >
        <option value="">{s.triState.notStated}</option>
        <option value="yes">{s.triState.yes}</option>
        <option value="no">{s.triState.no}</option>
      </select>
      <span class="text-xs text-muted-foreground">{s.fields.privacyConsentHint}</span>
    </label>

    <label class="flex flex-col gap-1 text-sm">
      <span class="text-muted-foreground">{s.fields.noticePeriod}</span>
      <!-- inputmode, not type="number": the design-system Input's `value` is typed (and
           bound) as a string, but Svelte's bind:value on a native type="number" input
           coerces to a JS number regardless — desiredSalaryAmount.trim() below would
           throw. inputmode still gets the numeric keyboard on mobile without that trap. -->
      <Input
        inputmode="numeric"
        bind:value={noticePeriodDays}
        oninput={edited}
        placeholder="30"
        aria-invalid={noticeInvalid}
        class="w-full {noticeInvalid ? 'border-destructive focus-visible:ring-destructive' : ''}"
      />
      {#if noticeInvalid}
        <span class="text-xs text-destructive">{format(s.mustBeWholeNumber, { field: s.fields.noticePeriod })}</span>
      {/if}
    </label>

    <label class="flex flex-col gap-1 text-sm">
      <span class="text-muted-foreground">{s.fields.desiredSalaryAmount}</span>
      <Input
        inputmode="numeric"
        bind:value={desiredSalaryAmount}
        oninput={edited}
        placeholder="120000"
        aria-invalid={salaryInvalid}
        class="w-full {salaryInvalid ? 'border-destructive focus-visible:ring-destructive' : ''}"
      />
      {#if salaryInvalid}
        <span class="text-xs text-destructive">{format(s.mustBePositive, { field: s.fields.desiredSalaryAmount })}</span>
      {/if}
    </label>

    <label class="flex flex-col gap-1 text-sm">
      <span class="text-muted-foreground">{s.fields.currency}</span>
      <Input
        bind:value={desiredSalaryCurrency}
        oninput={currencyEdited}
        onblur={currencyLeft}
        placeholder="USD"
        maxlength={3}
        aria-invalid={currencyInvalid}
        class="w-full {currencyInvalid ? 'border-destructive focus-visible:ring-destructive' : ''}"
      />
      {#if currencyInvalid}
        <span class="text-xs text-destructive">{s.mustBeCurrency}</span>
      {/if}
    </label>

    <label class="flex flex-col gap-1 text-sm">
      <span class="text-muted-foreground">{s.fields.salaryPeriod}</span>
      <select
        bind:value={desiredSalaryPeriod}
        onchange={picked}
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

</section>
