<script lang="ts">
  import { marked } from 'marked';
  import {
    Link2,
    Briefcase,
    Building2,
    MapPin,
    Globe,
    Building,
    Tags,
    Banknote,
    FileText,
    CheckCircle2,
    Sparkles,
  } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { tablist } from '$lib/actions/tablist';
  import { api, ApiError } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { htmlToMarkdown } from '$lib/htmlToMarkdown';
  import { renderMarkdown } from '$lib/markdown';
  import {
    REGION_OPTIONS,
    WORK_MODE_OPTIONS,
    CURRENCY_OPTIONS,
    SENIORITY_OPTIONS,
    EMPLOYMENT_TYPE_OPTIONS,
  } from '$lib/facets';
  import type { Submission } from '$lib/types';
  import { Button, Input, cn } from '$lib/ui';
  import JobPreview from './JobPreview.svelte';
  import NoteEditor from './NoteEditor.svelte';
  import TokenInput from './facets/TokenInput.svelte';

  // Details/Preview tabs, hand-rolled with the shared tablist action rather than a
  // generic Tabs primitive — the pattern this codebase already uses (ReferralsView,
  // the tracking/activity layouts).
  let activeTab = $state<'details' | 'preview'>('details');

  // Form state. url/title/company are required (the server validates too); the rest are
  // optional. The structured facets (region/city/work-mode/skills) override the server's
  // dictionary derivation, and salary becomes an authoritative manual salary on the job.
  let url = $state('');
  let title = $state('');
  let company = $state('');
  let location = $state('');
  let source = $state('');

  // The description is authored as markdown in the shared tracker editor and converted
  // to HTML on submit (the catalogue renders descriptions as sanitized HTML).
  // NoteEditor live-syncs this value — including resetForm() clearing it after a
  // successful submit — so no remount is ever needed here.
  let descriptionMarkdown = $state('');

  let region = $state('');
  let cities = $state<string[]>([]);
  let workMode = $state('');
  let employmentType = $state('');
  let seniority = $state('');
  let skills = $state<string[]>([]);
  let salaryMin = $state<number | null>(null);
  let salaryMax = $state<number | null>(null);
  let currency = $state('');
  let period = $state('');

  // The salary period vocabulary (mirrors the backend vocab.SalaryPeriodValues).
  const PERIODS = [
    { value: 'year', label: 'per year' },
    { value: 'month', label: 'per month' },
    { value: 'day', label: 'per day' },
    { value: 'hour', label: 'per hour' },
  ];

  // Prefill state, independent of submit(): a best-effort aid that never blocks or
  // overwrites manual entry. prefilling disables the button while a request is in
  // flight; prefillMissURL names the URL a miss was reported for, so editing the URL
  // field afterwards clears the note instead of leaving it stuck next to a new link.
  let prefilling = $state(false);
  let prefillMissURL = $state<string | null>(null);
  const prefillMiss = $derived(prefillMissURL !== null && prefillMissURL === url.trim());

  let submitting = $state(false);
  let formError = $state<string | null>(null);
  // The just-submitted vacancy, shown as a confirmation that it is awaiting review.
  let submitted = $state.raw<Submission | null>(null);

  // The Preview tab's rendered description. Unlike submit()'s descriptionHtml (which the
  // server sanitizes on the way in, per moderation.Create), this renders straight to
  // {@html} client-side — renderMarkdown() (marked + DOMPurify) is required here, not
  // just marked.parse(), so a submitter pasting raw HTML into the editor can't run script
  // in their own preview before the submission is ever sanitized server-side.
  const previewDescriptionHtml = $derived(renderMarkdown(descriptionMarkdown.trim()));

  const canSubmit = $derived(
    url.trim() !== '' && title.trim() !== '' && company.trim() !== '' && !submitting,
  );

  // Shared surface for the native selects so they match the Input primitive.
  const selectClass =
    'h-9 rounded-lg border border-input bg-transparent px-3 text-sm transition-colors focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 dark:bg-input/30';

  function addToken(list: string[], value: string): string[] {
    const v = value.trim();
    if (v === '' || list.includes(v)) return list;
    return [...list, v];
  }

  // Fills in whatever the URL parsed to, but only into fields the submitter has not
  // already typed into — a late-arriving prefill must never clobber manual entry.
  // Silent on a miss or a network failure: the button is a best-effort aid, not a
  // required step, and the submitter can always keep typing.
  async function prefillFromURL() {
    const target = url.trim();
    if (target === '' || prefilling) return;
    prefilling = true;
    prefillMissURL = null;
    try {
      const result = await api.prefillSubmission(target);
      // Object.values(...).some((v) => v) would treat an empty array (skills: [])
      // as a hit — an array is truthy regardless of length.
      const found = Object.values(result).some((v) =>
        Array.isArray(v) ? v.length > 0 : typeof v === 'string' && v.trim() !== '',
      );
      if (!found) {
        prefillMissURL = target;
        return;
      }
      if (title.trim() === '' && result.title) title = result.title;
      if (company.trim() === '' && result.company) company = result.company;
      if (location.trim() === '' && result.location) location = result.location;
      if (workMode === '' && result.work_mode) workMode = result.work_mode;
      if (employmentType === '' && result.employment_type) employmentType = result.employment_type;
      if (seniority === '' && result.seniority) seniority = result.seniority;
      if (source.trim() === '' && result.source) source = result.source;
      // Skills is a list, not a scalar: merge in whatever the platform stated
      // structurally rather than an all-or-nothing overwrite, same dedup as typing
      // one in by hand.
      for (const skill of result.skills ?? []) skills = addToken(skills, skill);
      // The source page's description arrives as sanitized HTML, not the markdown this
      // editor wants, so it is converted before it lands.
      if (descriptionMarkdown.trim() === '' && result.description) {
        descriptionMarkdown = htmlToMarkdown(result.description);
      }
    } catch {
      // Best-effort: leave the form exactly as it was.
    } finally {
      prefilling = false;
    }
  }

  function resetForm() {
    url = title = company = location = source = '';
    descriptionMarkdown = '';
    region = workMode = employmentType = seniority = currency = period = '';
    cities = [];
    skills = [];
    salaryMin = salaryMax = null;
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    if (!canSubmit) return;
    submitting = true;
    formError = null;
    submitted = null;
    try {
      const md = descriptionMarkdown.trim();
      const descriptionHtml = md ? await marked.parse(md) : undefined;
      submitted = await api.submitJob({
        url: url.trim(),
        title: title.trim(),
        company: company.trim(),
        location: location.trim() || undefined,
        // The dedicated "Remote" work format implies the remote flag — no separate checkbox.
        remote: workMode === 'remote',
        description: descriptionHtml || undefined,
        source: source.trim() || undefined,
        skills: skills.length ? skills : undefined,
        regions: region ? [region] : undefined,
        cities: cities.length ? cities : undefined,
        work_mode: workMode || undefined,
        employment_type: employmentType || undefined,
        seniority: seniority || undefined,
        salary_min: salaryMin ?? undefined,
        salary_max: salaryMax ?? undefined,
        salary_currency: currency || undefined,
        salary_period: period || undefined,
      });
      resetForm();
    } catch (err) {
      // 409 means the URL is already awaiting review; surface the backend message.
      formError =
        err instanceof ApiError ? err.message : 'Could not submit the job. Please try again.';
    } finally {
      submitting = false;
    }
  }
</script>

{#if !isAuthenticated()}
  <p class="py-12 text-center text-sm text-muted-foreground">Sign in to submit a job.</p>
{:else}
  <div class="flex flex-col gap-8">
    <header class="flex flex-col gap-2 border-b border-border pb-6">
      <p class="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-brand-strong">
        <Briefcase class="size-3.5" /> For employers
      </p>
      <h1 class="text-3xl font-semibold tracking-tight">Post your opening</h1>
      <p class="max-w-2xl text-sm text-muted-foreground">
        Add the skills, location, work format and salary — the right candidates find it faster.
        A moderator reviews every submission before it goes live in the catalogue.
      </p>
    </header>

    {#if submitted}
      <div
        class="flex items-start gap-3 rounded-lg border border-border bg-secondary/40 p-4 text-sm"
        role="status"
      >
        <CheckCircle2 class="mt-0.5 size-5 shrink-0 text-brand-strong" />
        <div>
          Thanks — <span class="font-medium">{submitted.title}</span> at
          <span class="font-medium">{submitted.company}</span> was submitted and is awaiting review.
          You can track it under
          <a href={resolve('/my/submissions')} class="underline">My submissions</a>.
        </div>
      </div>
    {/if}

    <div class="flex flex-col gap-6">
      <!-- use:tablist is what makes role="tablist" true — see ReferralsView for the
           same pattern. -->
      <div class="flex gap-1 border-b border-border" role="tablist" use:tablist={activeTab}>
        {#each [['details', 'Details'], ['preview', 'Preview']] as [id, label] (id)}
          <button
            type="button"
            role="tab"
            aria-selected={activeTab === id}
            onclick={() => (activeTab = id as 'details' | 'preview')}
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

      {#if activeTab === 'preview'}
        <JobPreview
          {title}
          {company}
          {workMode}
          {region}
          {cities}
          {employmentType}
          {seniority}
          {skills}
          {salaryMin}
          {salaryMax}
          salaryCurrency={currency}
          salaryPeriod={period}
          descriptionHtml={previewDescriptionHtml}
        />
      {/if}

      <form
        onsubmit={submit}
        class={cn('flex flex-col divide-y divide-border', activeTab !== 'details' && 'hidden')}
      >
        <!-- Basics: the required identity of the posting. -->
        <fieldset class="flex flex-col gap-4 py-6">
          <legend class="px-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            Basics
          </legend>
          <div class="flex flex-col gap-1">
            <span class="flex items-center gap-1.5 text-sm font-medium">
              <Link2 class="size-3.5 text-muted-foreground" />
              Job URL <span class="text-destructive">*</span>
            </span>
            <div class="flex items-center gap-2">
              <!-- The label wraps only the Input, not the Button beside it — nesting
                   both inside one label would make the implicit label target ambiguous. -->
              <label class="w-full">
                <Input bind:value={url} type="url" placeholder="https://…" class="w-full" />
              </label>
              <Button
                type="button"
                variant="secondary"
                size="sm"
                class="shrink-0 gap-1.5"
                disabled={url.trim() === '' || prefilling}
                onclick={prefillFromURL}
              >
                <Sparkles class="size-3.5" />
                {prefilling ? 'Filling in…' : 'Fill in from this link'}
              </Button>
            </div>
            {#if prefillMiss}
              <p class="text-xs text-muted-foreground">
                Couldn't find anything to fill in from that link — no problem, just fill in the
                rest below.
              </p>
            {/if}
          </div>
          <div class="flex flex-col gap-4 sm:flex-row">
            <label class="flex flex-1 flex-col gap-1">
              <span class="flex items-center gap-1.5 text-sm font-medium">
                <Briefcase class="size-3.5 text-muted-foreground" />
                Title <span class="text-destructive">*</span>
              </span>
              <Input bind:value={title} placeholder="Senior Go Developer" class="w-full" />
            </label>
            <label class="flex flex-1 flex-col gap-1">
              <span class="flex items-center gap-1.5 text-sm font-medium">
                <Building2 class="size-3.5 text-muted-foreground" />
                Company <span class="text-destructive">*</span>
              </span>
              <Input bind:value={company} placeholder="Acme" class="w-full" />
            </label>
          </div>
        </fieldset>

        <!-- Details: the structured facets that make the vacancy searchable. -->
        <fieldset class="flex flex-col gap-4 py-6">
          <legend class="px-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            Details
          </legend>
          <div class="flex flex-col gap-4 sm:flex-row">
            <label class="flex flex-1 flex-col gap-1">
              <span class="flex items-center gap-1.5 text-sm font-medium">
                <MapPin class="size-3.5 text-muted-foreground" />
                Location
              </span>
              <Input bind:value={location} placeholder="Berlin, Germany" class="w-full" />
            </label>
            <label class="flex flex-1 flex-col gap-1">
              <span class="flex items-center gap-1.5 text-sm font-medium">
                <Globe class="size-3.5 text-muted-foreground" />
                Region
              </span>
              <select bind:value={region} class={cn(selectClass, 'w-full')}>
                <option value="">Any</option>
                {#each REGION_OPTIONS as opt (opt.value)}
                  <option value={opt.value}>{opt.label}</option>
                {/each}
              </select>
            </label>
          </div>

          <label class="flex flex-col gap-1">
            <span class="flex items-center gap-1.5 text-sm font-medium">
              <Building class="size-3.5 text-muted-foreground" />
              City
            </span>
            <TokenInput
              tokens={cities}
              onAdd={(v) => (cities = addToken(cities, v))}
              onRemove={(v) => (cities = cities.filter((c) => c !== v))}
              placeholder="Add a city and press Enter"
            />
          </label>

          <div class="flex flex-col gap-1">
            <span class="text-sm font-medium">Work format</span>
            <div class="flex flex-wrap gap-1.5">
              {#each WORK_MODE_OPTIONS as opt (opt.value)}
                <button
                  type="button"
                  onclick={() => (workMode = workMode === opt.value ? '' : opt.value)}
                  class={cn(
                    'rounded-full border px-3 py-1 text-sm transition-colors',
                    workMode === opt.value
                      ? 'border-transparent bg-secondary font-medium text-secondary-foreground'
                      : 'border-input text-muted-foreground hover:bg-accent hover:text-accent-foreground',
                  )}
                >
                  {opt.label}
                </button>
              {/each}
            </div>
          </div>

          <div class="flex flex-col gap-4 sm:flex-row">
            <label class="flex flex-1 flex-col gap-1">
              <span class="text-sm font-medium">Employment type</span>
              <select bind:value={employmentType} class={cn(selectClass, 'w-full')}>
                <option value="">Any</option>
                {#each EMPLOYMENT_TYPE_OPTIONS as opt (opt.value)}
                  <option value={opt.value}>{opt.label}</option>
                {/each}
              </select>
            </label>
            <label class="flex flex-1 flex-col gap-1">
              <span class="text-sm font-medium">Seniority</span>
              <select bind:value={seniority} class={cn(selectClass, 'w-full')}>
                <option value="">Any</option>
                {#each SENIORITY_OPTIONS as opt (opt.value)}
                  <option value={opt.value}>{opt.label}</option>
                {/each}
              </select>
            </label>
          </div>

          <label class="flex flex-col gap-1">
            <span class="flex items-center gap-1.5 text-sm font-medium">
              <Tags class="size-3.5 text-muted-foreground" />
              Skills
            </span>
            <TokenInput
              tokens={skills}
              onAdd={(v) => (skills = addToken(skills, v))}
              onRemove={(v) => (skills = skills.filter((s) => s !== v))}
              placeholder="e.g. Go, Kubernetes — Enter to add"
            />
          </label>

          <div class="flex flex-col gap-1">
            <span class="flex items-center gap-1.5 text-sm font-medium">
              <Banknote class="size-3.5 text-muted-foreground" />
              Salary
            </span>
            <div class="flex flex-wrap items-center gap-2">
              <Input
                type="number"
                min="0"
                placeholder="Min"
                value={salaryMin != null ? String(salaryMin) : ''}
                oninput={(e) =>
                  (salaryMin = e.currentTarget.value ? Number(e.currentTarget.value) : null)}
                class="w-24"
              />
              <span class="text-muted-foreground">–</span>
              <Input
                type="number"
                min="0"
                placeholder="Max"
                value={salaryMax != null ? String(salaryMax) : ''}
                oninput={(e) =>
                  (salaryMax = e.currentTarget.value ? Number(e.currentTarget.value) : null)}
                class="w-24"
              />
              <select bind:value={currency} class={selectClass} aria-label="Currency">
                <option value="">Currency</option>
                {#each CURRENCY_OPTIONS as opt (opt.value)}
                  <option value={opt.value}>{opt.label}</option>
                {/each}
              </select>
              <select bind:value={period} class={selectClass} aria-label="Salary period">
                <option value="">Period</option>
                {#each PERIODS as p (p.value)}
                  <option value={p.value}>{p.label}</option>
                {/each}
              </select>
            </div>
          </div>

          <label class="flex flex-col gap-1">
            <span class="text-sm font-medium">Source</span>
            <Input bind:value={source} placeholder="e.g. greenhouse (optional)" class="w-full" />
          </label>
        </fieldset>

        <!-- Description: the tracker's markdown editor; converted to HTML on submit. -->
        <fieldset class="flex flex-col gap-2 py-6">
          <legend class="px-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            <span class="flex items-center gap-1.5">
              <FileText class="size-3.5" />
              Description
            </span>
          </legend>
          <NoteEditor
            value={descriptionMarkdown}
            onsave={(v) => (descriptionMarkdown = v)}
            placeholder="Paste or write the job description…"
          />
        </fieldset>

        <div class="flex flex-col gap-3 py-6">
          {#if formError}
            <p class="text-sm text-destructive">{formError}</p>
          {/if}
          <div>
            <Button variant="primary" size="lg" type="submit" disabled={!canSubmit}>
              {submitting ? 'Submitting…' : 'Submit for review'}
            </Button>
          </div>
        </div>
      </form>
    </div>
  </div>
{/if}
