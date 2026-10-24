<script lang="ts">
  import { Briefcase, Building2, Globe, MapPin, Plus } from '@lucide/svelte';
  import { tablist } from '#lib/actions/tablist.js';
  import { api, ApiError } from '#lib/api.js';
  import { AsyncData } from '#lib/asyncData.svelte.js';
  import { EMPLOYMENT_TYPE_OPTIONS, SENIORITY_OPTIONS, WORK_MODE_OPTIONS } from '#lib/facets.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, t } from '#lib/i18n/t.js';
  import { messages } from './EmployerView.messages';
  import type {
    EmployerApplyMode,
    EmployerCompany,
    EmployerCompanyProfileInput,
    EmployerQuestion,
    EmployerVacancyInput,
    Job,
  } from '#lib/types.js';
  import { resolve } from '$app/paths';
  import { Button, ConfirmDialog, Input, cn } from '#lib/ui/index.js';
  import States from './States.svelte';
  import EmployerQuestionsEditor from './EmployerQuestionsEditor.svelte';
  import TokenInput from './facets/TokenInput.svelte';

  let { company, onProfileSaved }: { company: EmployerCompany; onProfileSaved: (updated: EmployerCompany) => void } =
    $props();

  const s = $derived(t(messages, locale()));

  let activeTab = $state<'profile' | 'vacancies'>('profile');

  const selectClass =
    'h-9 rounded-lg border border-input bg-transparent px-3 text-sm transition-colors focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 dark:bg-input/30';

  // ── Profile tab ────────────────────────────────────────────────────────
  let tagline = $state(company.tagline ?? '');
  let description = $state(company.description ?? '');
  let website = $state(company.website ?? '');
  let industries = $state<string[]>(company.industries ?? []);
  let yearFounded = $state<number | null>(company.year_founded ?? null);
  let employeeCount = $state<number | null>(company.employee_count ?? null);
  let hqCountry = $state(company.hq_country ?? '');
  let subindustry = $state(company.subindustry ?? '');

  let savingProfile = $state(false);
  let profileError = $state<string | null>(null);
  let profileSavedAt = $state(0);

  async function saveProfile(e: SubmitEvent) {
    e.preventDefault();
    if (savingProfile) return;
    savingProfile = true;
    profileError = null;
    try {
      const input: EmployerCompanyProfileInput = {
        tagline: tagline.trim() || undefined,
        description: description.trim() || undefined,
        website: website.trim() || undefined,
        industries,
        year_founded: yearFounded ?? undefined,
        employee_count: employeeCount ?? undefined,
        hq_country: hqCountry.trim() || undefined,
        subindustry: subindustry.trim() || undefined,
      };
      const updated = await api.updateEmployerCompany(input);
      onProfileSaved(updated);
      profileSavedAt = Date.now();
    } catch (err) {
      profileError = err instanceof ApiError ? err.message : s.profileSaveError;
    } finally {
      savingProfile = false;
    }
  }

  // ── Vacancies tab ──────────────────────────────────────────────────────
  const vacanciesData = new AsyncData<Job[]>([]);
  let vacanciesLoaded = $state(false);
  $effect(() => {
    if (activeTab === 'vacancies' && !vacanciesLoaded) {
      vacanciesLoaded = true;
      void vacanciesData.run(() => api.listEmployerVacancies());
    }
  });

  // One form, reused for both create (editingSlug === null) and edit (editingSlug set) —
  // the same shape SubmitView uses for its own single form.
  let formOpen = $state(false);
  let editingSlug = $state<string | null>(null);
  // On freehire by default: it is what an employer cannot get from their own careers page.
  let applyMode = $state<EmployerApplyMode>('freehire');
  let questions = $state<EmployerQuestion[]>([]);
  let questionsLoaded = $state(true);
  let url = $state('');
  let title = $state('');
  let location = $state('');
  let workMode = $state('');
  let employmentType = $state('');
  let seniority = $state('');
  let descriptionMd = $state('');
  let skills = $state<string[]>([]);
  let saving = $state(false);
  let formError = $state<string | null>(null);

  // A url is the identity of a linked vacancy only, and only when creating one: an edit
  // never sends it, and a vacancy taking applications on freehire has none to send.
  const needsUrl = $derived(!editingSlug && applyMode === 'external');
  // A description is required: search keeps out a posting without one, and the server
  // refuses it rather than publish a vacancy nobody can find.
  const canSubmit = $derived(
    !!title.trim() && !!descriptionMd.trim() && (!needsUrl || !!url.trim()) && questionsLoaded,
  );

  function resetForm() {
    editingSlug = null;
    applyMode = 'freehire';
    questions = [];
    questionsLoaded = true;
    url = title = location = workMode = employmentType = seniority = descriptionMd = '';
    skills = [];
    formError = null;
  }

  function openCreate() {
    resetForm();
    formOpen = true;
  }

  function openEdit(job: Job) {
    editingSlug = job.public_slug;
    applyMode = job.direct_apply ? 'freehire' : 'external';
    questions = [];
    url = job.url;
    title = job.title;
    location = job.location;
    workMode = job.work_mode ?? '';
    employmentType = job.enrichment.employment_type ?? '';
    seniority = job.enrichment.seniority ?? '';
    descriptionMd = job.description;
    skills = job.skills;
    formError = null;
    formOpen = true;
    if (job.direct_apply) void loadQuestions(job.public_slug);
  }

  // The questions are read back through the same form read candidates use, so the editor
  // starts from exactly what is published. Saving is held until they arrive: a PATCH
  // carrying the empty list would erase them.
  async function loadQuestions(slug: string) {
    questionsLoaded = false;
    try {
      const form = await api.getDirectApplyForm(slug);
      if (editingSlug !== slug) return;
      questions = form.questions.map(({ label, type, required, options }) => ({ label, type, required, options }));
      questionsLoaded = true;
    } catch {
      if (editingSlug === slug) formError = s.questionsLoadError;
    }
  }

  function addSkill(value: string) {
    const v = value.trim();
    if (v && !skills.includes(v)) skills = [...skills, v];
  }
  function removeSkill(value: string) {
    skills = skills.filter((v) => v !== value);
  }

  function replaceInList(updated: Job) {
    vacanciesData.value = vacanciesData.value.map((j) => (j.public_slug === updated.public_slug ? updated : j));
  }

  async function submitForm(e: SubmitEvent) {
    e.preventDefault();
    if (saving || !canSubmit) return;
    saving = true;
    formError = null;
    try {
      if (editingSlug) {
        const updated = await api.updateEmployerVacancy(editingSlug, {
          questions: applyMode === 'freehire' ? questions : undefined,
          title: title.trim(),
          location: location.trim() || undefined,
          remote: workMode === 'remote',
          description: descriptionMd.trim() || undefined,
        });
        replaceInList(updated);
      } else {
        const input: EmployerVacancyInput = {
          apply_mode: applyMode,
          questions: applyMode === 'freehire' ? questions : undefined,
          url: applyMode === 'external' ? url.trim() : undefined,
          title: title.trim(),
          location: location.trim() || undefined,
          remote: workMode === 'remote',
          description: descriptionMd.trim() || undefined,
          work_mode: workMode || undefined,
          employment_type: employmentType || undefined,
          seniority: seniority || undefined,
          skills: skills.length ? skills : undefined,
        };
        const created = await api.createEmployerVacancy(input);
        // A re-Create under the same URL (this account's own reopen path) replaces the
        // existing row rather than duplicating it — mirror that here too.
        const exists = vacanciesData.value.some((j) => j.public_slug === created.public_slug);
        vacanciesData.value = exists ? vacanciesData.value.map((j) => (j.public_slug === created.public_slug ? created : j)) : [created, ...vacanciesData.value];
      }
      formOpen = false;
      resetForm();
    } catch (err) {
      formError =
        err instanceof ApiError && err.status === 409
          ? s.urlTakenError
          : err instanceof ApiError
            ? err.message
            : s.vacancyError;
    } finally {
      saving = false;
    }
  }

  // Closing takes it out of search and stops applications, so it asks first: "Close" beside
  // a list row reads too easily as "close this panel".
  let closeTarget = $state<Job | null>(null);
  let confirmOpen = $state(false);
  let closingSlug = $state<string | null>(null);
  async function closeVacancy(job: Job) {
    closingSlug = job.public_slug;
    try {
      await api.closeEmployerVacancy(job.public_slug);
      replaceInList({ ...job, closed_at: new Date().toISOString() });
    } finally {
      closingSlug = null;
    }
  }

  let reopeningSlug = $state<string | null>(null);
  let reopenError = $state<{ slug: string; message: string } | null>(null);
  async function reopenVacancy(job: Job) {
    if (reopeningSlug) return;
    reopeningSlug = job.public_slug;
    reopenError = null;
    try {
      await api.reopenEmployerVacancy(job.public_slug);
      replaceInList({ ...job, closed_at: undefined });
    } catch (err) {
      reopenError = { slug: job.public_slug, message: err instanceof ApiError ? err.message : s.vacancyError };
    } finally {
      reopeningSlug = null;
    }
  }
</script>

<div class="flex flex-col gap-6">
  <header class="flex items-center gap-2 border-b border-border pb-4">
    <Building2 class="size-5 text-muted-foreground" />
    <h1 class="text-2xl font-semibold tracking-tight">{format(s.dashboardTitle, { company: company.company_name })}</h1>
  </header>

  <div class="flex gap-1 border-b border-border" role="tablist" use:tablist={activeTab}>
    {#each [['profile', s.tabProfile], ['vacancies', s.tabVacancies]] as [id, label] (id)}
      <button
        type="button"
        role="tab"
        aria-selected={activeTab === id}
        onclick={() => (activeTab = id as 'profile' | 'vacancies')}
        class={cn(
          '-mb-px border-b-2 px-3 py-2.5 text-sm font-semibold',
          activeTab === id
            ? 'border-brand text-foreground'
            : 'border-transparent text-muted-foreground hover:text-foreground',
        )}
      >
        {label}
      </button>
    {/each}
  </div>

  {#if activeTab === 'profile'}
    <form onsubmit={saveProfile} class="flex max-w-xl flex-col gap-4">
      <label class="flex flex-col gap-1">
        <span class="text-sm font-medium">{s.profileTaglineLabel}</span>
        <Input bind:value={tagline} placeholder={s.profileTaglinePlaceholder} class="w-full" />
      </label>
      <label class="flex flex-col gap-1">
        <span class="text-sm font-medium">{s.profileDescriptionLabel}</span>
        <textarea bind:value={description} rows="4" class="w-full rounded-lg border border-input bg-transparent px-3 py-2 text-sm transition-colors focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 dark:bg-input/30"
        ></textarea>
      </label>
      <label class="flex flex-col gap-1">
        <span class="flex items-center gap-1.5 text-sm font-medium">
          <Globe class="size-3.5 text-muted-foreground" />
          {s.profileWebsiteLabel}
        </span>
        <Input bind:value={website} placeholder="acme.com" class="w-full" />
      </label>
      <label class="flex flex-col gap-1">
        <span class="text-sm font-medium">{s.profileIndustriesLabel}</span>
        <TokenInput label={s.profileIndustriesLabel} tokens={industries} onAdd={(v) => (industries = [...industries, v])} onRemove={(v) => (industries = industries.filter((x) => x !== v))} />
      </label>
      <div class="flex flex-col gap-4 sm:flex-row">
        <label class="flex flex-1 flex-col gap-1">
          <span class="text-sm font-medium">{s.profileYearFoundedLabel}</span>
          <Input
            type="number"
            value={yearFounded != null ? String(yearFounded) : ''}
            oninput={(e) => (yearFounded = e.currentTarget.value ? Number(e.currentTarget.value) : null)}
            class="w-full"
          />
        </label>
        <label class="flex flex-1 flex-col gap-1">
          <span class="text-sm font-medium">{s.profileEmployeeCountLabel}</span>
          <Input
            type="number"
            value={employeeCount != null ? String(employeeCount) : ''}
            oninput={(e) => (employeeCount = e.currentTarget.value ? Number(e.currentTarget.value) : null)}
            class="w-full"
          />
        </label>
      </div>
      <div class="flex flex-col gap-4 sm:flex-row">
        <label class="flex flex-1 flex-col gap-1">
          <span class="text-sm font-medium">{s.profileHqCountryLabel}</span>
          <Input bind:value={hqCountry} placeholder="US" maxlength={2} class="w-full" />
        </label>
        <label class="flex flex-1 flex-col gap-1">
          <span class="text-sm font-medium">{s.profileSubindustryLabel}</span>
          <Input bind:value={subindustry} class="w-full" />
        </label>
      </div>
      {#if profileError}
        <p class="text-sm text-destructive">{profileError}</p>
      {:else if profileSavedAt}
        {#key profileSavedAt}
          <p class="text-sm text-brand-strong">{s.profileSaved}</p>
        {/key}
      {/if}
      <Button type="submit" disabled={savingProfile} class="w-fit">
        {savingProfile ? s.profileSaving : s.profileSave}
      </Button>
    </form>
  {:else}
    <div class="flex flex-col gap-4">
      {#if !formOpen}
        <Button size="sm" class="w-fit gap-1.5" onclick={openCreate}>
          <Plus class="size-3.5" />
          {s.newVacancy}
        </Button>
      {/if}

      {#if formOpen}
        <form onsubmit={submitForm} class="flex flex-col gap-4 rounded-lg border border-border p-4">
          <h2 class="text-sm font-semibold">{editingSlug ? s.formTitleEdit : s.formTitleNew}</h2>
          {#if !editingSlug}
            <fieldset class="flex flex-col gap-2">
              <legend class="text-sm font-medium">{s.applyModeLabel}</legend>
              {#each [['freehire', s.applyModeFreehire, s.applyModeFreehireHint], ['external', s.applyModeExternal, s.applyModeExternalHint]] as [mode, label, hint] (mode)}
                <label class="flex items-start gap-2 text-sm">
                  <input type="radio" name="apply-mode" value={mode} bind:group={applyMode} class="mt-1 accent-current" />
                  <span class="flex flex-col">
                    <span class="font-medium">{label}</span>
                    <span class="text-xs text-muted-foreground">{hint}</span>
                  </span>
                </label>
              {/each}
            </fieldset>
            {#if applyMode === 'external'}
              <label class="flex flex-col gap-1">
                <span class="text-sm font-medium">{s.fieldUrl}</span>
                <Input bind:value={url} type="url" placeholder="https://…" class="w-full" />
                <span class="text-xs text-muted-foreground">{s.fieldUrlHint}</span>
              </label>
            {/if}
          {/if}
          <div class="flex flex-col gap-4 sm:flex-row">
            <label class="flex flex-1 flex-col gap-1">
              <span class="flex items-center gap-1.5 text-sm font-medium">
                <Briefcase class="size-3.5 text-muted-foreground" />
                {s.fieldTitle}
              </span>
              <Input bind:value={title} class="w-full" />
            </label>
            <label class="flex flex-1 flex-col gap-1">
              <span class="flex items-center gap-1.5 text-sm font-medium">
                <MapPin class="size-3.5 text-muted-foreground" />
                {s.fieldLocation}
              </span>
              <Input bind:value={location} class="w-full" />
            </label>
          </div>
          <div class="flex flex-col gap-4 sm:flex-row">
            {#if !editingSlug}
              <label class="flex flex-1 flex-col gap-1">
                <span class="text-sm font-medium">{s.fieldWorkMode}</span>
                <select bind:value={workMode} class={cn(selectClass, 'w-full')}>
                  <option value="">—</option>
                  {#each WORK_MODE_OPTIONS as o (o.value)}
                    <option value={o.value}>{o.label}</option>
                  {/each}
                </select>
              </label>
              <label class="flex flex-1 flex-col gap-1">
                <span class="text-sm font-medium">{s.fieldEmploymentType}</span>
                <select bind:value={employmentType} class={cn(selectClass, 'w-full')}>
                  <option value="">—</option>
                  {#each EMPLOYMENT_TYPE_OPTIONS as o (o.value)}
                    <option value={o.value}>{o.label}</option>
                  {/each}
                </select>
              </label>
              <label class="flex flex-1 flex-col gap-1">
                <span class="text-sm font-medium">{s.fieldSeniority}</span>
                <select bind:value={seniority} class={cn(selectClass, 'w-full')}>
                  <option value="">—</option>
                  {#each SENIORITY_OPTIONS as o (o.value)}
                    <option value={o.value}>{o.label}</option>
                  {/each}
                </select>
              </label>
            {/if}
          </div>
          <label class="flex flex-col gap-1">
            <span class="text-sm font-medium">{s.fieldDescription}</span>
            <span class="text-xs text-muted-foreground">{s.fieldDescriptionHint}</span>
            <textarea bind:value={descriptionMd} rows="6" class="w-full rounded-lg border border-input bg-transparent px-3 py-2 text-sm transition-colors focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 dark:bg-input/30"
            ></textarea>
          </label>
          {#if !editingSlug}
            <label class="flex flex-col gap-1">
              <span class="text-sm font-medium">{s.fieldSkills}</span>
              <TokenInput label={s.fieldSkills} tokens={skills} onAdd={addSkill} onRemove={removeSkill} />
            </label>
          {/if}
          {#if applyMode === 'freehire' && questionsLoaded}
            <EmployerQuestionsEditor bind:questions />
          {/if}
          {#if formError}
            <p class="text-sm text-destructive">{formError}</p>
          {/if}
          <div class="flex gap-2">
            <Button type="submit" disabled={saving || !canSubmit}>
              {#if editingSlug}
                {saving ? s.savingChanges : s.saveChanges}
              {:else}
                {saving ? s.publishing : s.publish}
              {/if}
            </Button>
            <Button type="button" variant="ghost" onclick={() => { formOpen = false; resetForm(); }}>
              {s.cancel}
            </Button>
          </div>
        </form>
      {/if}

      {#if vacanciesData.status === 'loading'}
        <States state="loading" />
      {:else if vacanciesData.status === 'error'}
        <States state="error" message={s.vacanciesLoadError} />
      {:else if vacanciesData.value.length === 0}
        <States state="empty" message={s.vacanciesEmpty} />
      {:else}
        <ul class="flex flex-col divide-y divide-border rounded-lg border border-border">
          {#each vacanciesData.value as job (job.public_slug)}
            <li class="flex flex-col gap-3 px-4 py-3 sm:flex-row sm:items-start sm:justify-between">
              <div class="flex min-w-0 flex-col gap-0.5">
                <span class="flex items-center gap-2 truncate text-sm font-medium">
                  <a
                    href={resolve('/jobs/[slug]', { slug: job.public_slug })}
                    title={s.viewVacancy}
                    class="truncate underline-offset-2 hover:underline">{job.title}</a
                  >
                  {#if job.direct_apply}
                    <span class="rounded-full bg-brand-muted px-2 py-0.5 text-xs font-medium text-brand-strong">
                      {s.onSiteLabel}
                    </span>
                  {/if}
                  {#if job.closed_at}
                    <span class="rounded-full bg-secondary px-2 py-0.5 text-xs font-medium text-secondary-foreground">
                      {s.closedLabel}
                    </span>
                  {/if}
                </span>
                <span class="truncate text-xs text-muted-foreground">
                  {job.location}{job.work_mode === 'remote' ? ` · ${s.fieldRemote}` : ''}
                </span>
              </div>
              <div class="flex shrink-0 gap-2">
                <Button variant="ghost" size="sm" onclick={() => openEdit(job)}>{s.editVacancy}</Button>
                {#if !job.closed_at}
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={closingSlug === job.public_slug}
                    onclick={() => {
                      closeTarget = job;
                      confirmOpen = true;
                    }}
                  >
                    {closingSlug === job.public_slug ? s.closing : s.closeVacancy}
                  </Button>
                {:else}
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={reopeningSlug === job.public_slug}
                    onclick={() => reopenVacancy(job)}
                  >
                    {reopeningSlug === job.public_slug ? s.reopening : s.reopenVacancy}
                  </Button>
                {/if}
              </div>
              {#if reopenError?.slug === job.public_slug}
                <p class="text-sm text-destructive">{reopenError.message}</p>
              {/if}
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  {/if}
</div>

<ConfirmDialog
  bind:open={confirmOpen}
  title={s.closeConfirmTitle}
  description={s.closeConfirmBody}
  confirmLabel={s.closeVacancy}
  cancelLabel={s.cancel}
  onConfirm={async () => {
    if (closeTarget) await closeVacancy(closeTarget);
  }}
/>
