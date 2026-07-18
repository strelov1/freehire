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
  } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { api, ApiError } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { REGION_OPTIONS, WORK_MODE_OPTIONS, CURRENCY_OPTIONS } from '$lib/facets';
  import type { Submission } from '$lib/types';
  import { Button, Input } from '$lib/ui';
  import { cn } from '$lib/utils';
  import NoteEditor from './NoteEditor.svelte';
  import TokenInput from './facets/TokenInput.svelte';

  // Form state. url/title/company are required (the server validates too); the rest are
  // optional. The structured facets (region/city/work-mode/skills) override the server's
  // dictionary derivation, and salary becomes an authoritative manual salary on the job.
  let url = $state('');
  let title = $state('');
  let company = $state('');
  let location = $state('');
  let source = $state('');

  // The description is authored as markdown in the shared tracker editor and converted to
  // HTML on submit (the catalogue renders descriptions as sanitized HTML). editorKey
  // remounts the editor to clear it after a successful submit.
  let descriptionMarkdown = $state('');
  let editorKey = $state(0);

  let region = $state('');
  let cities = $state<string[]>([]);
  let workMode = $state('');
  let skills = $state<string[]>([]);
  let salaryMin = $state<number | null>(null);
  let salaryMax = $state<number | null>(null);
  let currency = $state('');
  let period = $state('');

  // The salary period vocabulary (mirrors the backend enrich.SalaryPeriodValues).
  const PERIODS = [
    { value: 'year', label: 'per year' },
    { value: 'month', label: 'per month' },
    { value: 'day', label: 'per day' },
    { value: 'hour', label: 'per hour' },
  ];

  let submitting = $state(false);
  let formError = $state<string | null>(null);
  // The just-submitted vacancy, shown as a confirmation that it is awaiting review.
  let submitted = $state.raw<Submission | null>(null);

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

  function resetForm() {
    url = title = company = location = source = '';
    descriptionMarkdown = '';
    region = workMode = currency = period = '';
    cities = [];
    skills = [];
    salaryMin = salaryMax = null;
    editorKey += 1;
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
  <div class="flex flex-col gap-6">
    <div class="flex flex-col gap-1">
      <h1 class="text-2xl font-semibold tracking-tight">Submit a job</h1>
      <p class="text-sm text-muted-foreground">
        Hiring? Submit your opening — the skills, location, work format and salary you add help
        the right candidates find it. A moderator reviews it before it goes live in the catalogue.
      </p>
    </div>

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

    <form onsubmit={submit} class="flex flex-col gap-6">
      <!-- Basics: the required identity of the posting. -->
      <fieldset class="flex flex-col gap-4 rounded-lg border border-border p-4">
        <legend class="px-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
          Basics
        </legend>
        <label class="flex flex-col gap-1">
          <span class="flex items-center gap-1.5 text-sm font-medium">
            <Link2 class="size-3.5 text-muted-foreground" />
            Job URL <span class="text-destructive">*</span>
          </span>
          <Input bind:value={url} type="url" placeholder="https://…" class="w-full" />
        </label>
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
      <fieldset class="flex flex-col gap-4 rounded-lg border border-border p-4">
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
      <fieldset class="flex flex-col gap-2 rounded-lg border border-border p-4">
        <legend class="px-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
          <span class="flex items-center gap-1.5">
            <FileText class="size-3.5" />
            Description
          </span>
        </legend>
        {#key editorKey}
          <NoteEditor
            value={descriptionMarkdown}
            onsave={(v) => (descriptionMarkdown = v)}
            placeholder="Paste or write the job description…"
          />
        {/key}
      </fieldset>

      {#if formError}
        <p class="text-sm text-destructive">{formError}</p>
      {/if}

      <div>
        <Button variant="primary" type="submit" disabled={!canSubmit}>
          {submitting ? 'Submitting…' : 'Submit for review'}
        </Button>
      </div>
    </form>
  </div>
{/if}
