<script lang="ts">
  // A presentational-only render of an in-progress /submit draft, styled like the real
  // vacancy page (JobView) so a submitter can see roughly how it will read once approved.
  // Unlike JobView it makes no network call and offers no interactive affordance
  // (apply/save/vote/report/discussion) — all of those presuppose a persisted job with a
  // public_slug, which a draft does not have. Facet labels are resolved through the same
  // shared vocabularies the filter bar and the submit form itself use, so a code never
  // renders raw.
  import {
    REGION_OPTIONS,
    WORK_MODE_OPTIONS,
    SENIORITY_OPTIONS,
    EMPLOYMENT_TYPE_OPTIONS,
  } from '$lib/facets';
  import { formatSalary } from '$lib/enrichment';
  import { companyLogoUrl } from '$lib/logo';
  import { Badge, EntityLogo } from '$lib/ui';
  import JobDescription from './JobDescription.svelte';
  import SkillIcon from './SkillIcon.svelte';

  let {
    title,
    company,
    workMode,
    region,
    cities,
    employmentType,
    seniority,
    skills,
    salaryMin,
    salaryMax,
    salaryCurrency,
    salaryPeriod,
    descriptionHtml,
  }: {
    title: string;
    company: string;
    workMode: string;
    region: string;
    cities: string[];
    employmentType: string;
    seniority: string;
    skills: string[];
    salaryMin: number | null;
    salaryMax: number | null;
    salaryCurrency: string;
    salaryPeriod: string;
    descriptionHtml: string;
  } = $props();

  // Sentence-cases a code the option list doesn't recognize (e.g. a value the backend
  // parsed from a source page but the frontend's vocabulary hasn't caught up with) —
  // the same fallback web/src/lib/enrichment.ts's label() applies for the live job page,
  // so an unrecognized facet never renders a raw snake_case code here either.
  const labelFor = (options: { value: string; label: string }[], value: string) => {
    const known = options.find((o) => o.value === value)?.label;
    if (known) return known;
    const spaced = value.replace(/_/g, ' ');
    return spaced.charAt(0).toUpperCase() + spaced.slice(1);
  };

  const salary = $derived(
    formatSalary({
      salary_min: salaryMin ?? undefined,
      salary_max: salaryMax ?? undefined,
      salary_currency: salaryCurrency || undefined,
      salary_period: salaryPeriod || undefined,
    }),
  );

  // Draft facets: label + value pairs for whatever the submitter has filled in so far.
  // Non-clickable — a draft is not a live catalog entry, so a filter link would be a lie.
  const facets = $derived(
    [
      workMode && { label: 'Work format', value: labelFor(WORK_MODE_OPTIONS, workMode) },
      region && { label: 'Region', value: labelFor(REGION_OPTIONS, region) },
      cities.length > 0 && { label: 'City', value: cities.join(', ') },
      seniority && { label: 'Seniority', value: labelFor(SENIORITY_OPTIONS, seniority) },
      employmentType && {
        label: 'Employment',
        value: labelFor(EMPLOYMENT_TYPE_OPTIONS, employmentType),
      },
    ].filter((f): f is { label: string; value: string } => Boolean(f)),
  );
</script>

<article class="flex flex-col gap-4 rounded-lg border border-border p-4">
  <div class="flex items-center gap-3">
    <EntityLogo
      name={company || 'Your company'}
      src={company ? (companyLogoUrl(company) ?? undefined) : undefined}
      shape="square"
      size="sm"
    />
    <p class="text-sm text-muted-foreground">{company || 'Your company'}</p>
  </div>

  <h1 class="text-2xl font-semibold tracking-tight">{title || 'Job title'}</h1>

  {#if salary}
    <p class="text-xl font-semibold tabular-nums tracking-tight">{salary}</p>
  {/if}

  {#if facets.length}
    <dl class="flex flex-col gap-2 border-t border-border pt-4 text-sm">
      {#each facets as facet (facet.label)}
        <div class="flex items-baseline justify-between gap-3">
          <dt class="shrink-0 text-muted-foreground">{facet.label}</dt>
          <dd class="min-w-0 break-words text-right font-medium">{facet.value}</dd>
        </div>
      {/each}
    </dl>
  {/if}

  {#if skills.length}
    <ul class="flex flex-wrap gap-1.5 border-t border-border pt-4">
      {#each skills as skill (skill)}
        <li>
          <Badge variant="brand" class="gap-1">
            <SkillIcon slug={skill} />
            {skill}
          </Badge>
        </li>
      {/each}
    </ul>
  {/if}

  {#if descriptionHtml}
    <div class="border-t border-border pt-4">
      <JobDescription html={descriptionHtml} />
    </div>
  {:else}
    <p class="border-t border-border pt-4 text-sm text-muted-foreground">
      Write the description in the Details tab to see it here.
    </p>
  {/if}
</article>
