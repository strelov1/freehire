<script lang="ts">
  // The onboarding flow's results, kept compact: who is hiring (a logo stack of the top
  // matches' employers), the five best matches one line each, a link to all of them in the
  // feed, the CV's ATS score, and the save action. The jobs are public postings — never
  // blurred or held back behind sign-up. Every figure here is the one the server answered:
  // the count is the search's own total and the window the one searched.
  import { resolve } from '$app/paths';
  import { ArrowRight, BriefcaseBusiness, Gauge, Pencil } from '@lucide/svelte';
  import { workArrangement } from '#lib/enrichment.js';
  import { countryLabel } from '#lib/facets.js';
  import { companyLogoUrl } from '#lib/logo.js';
  import { FIRST_WINDOW_DAYS, matchCompanies, matchCompaniesLine } from '#lib/onboardingFlow.js';
  import type { ATSReport, Job, OnboardingMatch } from '#lib/types.js';
  import { Button, Chip, EntityLogo } from '#lib/ui/index.js';
  import { timeAgo } from '#lib/utils.js';
  import CompanyLogoStack from '../CompanyLogoStack.svelte';
  import AtsScorecard from './AtsScorecard.svelte';
  import FlowStep from './FlowStep.svelte';

  let {
    matches,
    total,
    days,
    widened,
    ranked,
    chips,
    report,
    query,
    onEditAnswers,
  }: {
    matches: OnboardingMatch[];
    total: number;
    /** The window searched, in days. */
    days: number;
    /** The first window found nothing, so `days` is the wider one. */
    widened: boolean;
    /** The CV had skills to rank by; without them the jobs carry no match figure. */
    ranked: boolean;
    chips: string[];
    report: ATSReport;
    /** The jobs-search query the matches were found under; the feed link opens it. */
    query: string;
    /** Absent while the answers cannot change (the account is being saved). */
    onEditAnswers?: () => void;
  } = $props();

  const TOP = 5;

  const found = $derived(
    total > 0
      ? // Skills rank the list, they do not filter it: the count is everything under the answers.
        `${total.toLocaleString('en')} open ${total === 1 ? 'job' : 'jobs'} for your answers`
      : 'No open jobs for these answers yet',
  );
  const windowLine = $derived(
    widened
      ? `Nothing was posted in the last ${FIRST_WINDOW_DAYS} days, so these are from the last ${days}.`
      : `Posted in the last ${days} days.`,
  );
  const companies = $derived(matchCompanies(matches));
  const feedHref = $derived(`${resolve('/jobs')}?${query}`);

  /** Company · where (the work mode, else the first country) · how long ago. */
  function meta(job: Job): string {
    const where = workArrangement(job) ?? (job.countries?.[0] ? countryLabel(job.countries[0]) : null);
    return [job.company, where, timeAgo(job.posted_at ?? job.created_at, 'en')].filter(Boolean).join(' · ');
  }
</script>

<FlowStep icon={BriefcaseBusiness} title={found} subtitle={windowLine} state="done">
  <div class="flex flex-wrap items-center gap-2">
    {#each chips as chip (chip)}
      <Chip>{chip}</Chip>
    {/each}
    {#if onEditAnswers}
      <Button variant="ghost" size="sm" onclick={onEditAnswers}>
        <Pencil class="size-3.5" aria-hidden="true" /> Change answers
      </Button>
    {/if}
  </div>
  {#if matches.length > 0}
    <div class="flex flex-wrap items-center gap-x-2.5 gap-y-1 text-sm text-muted-foreground">
      <CompanyLogoStack names={companies} />
      <span>{matchCompaniesLine(matches, total)}</span>
    </div>
    <ul class="divide-y divide-border rounded-xl border border-border bg-background">
      {#each matches.slice(0, TOP) as m (m.job.public_slug)}
        <li class="flex items-center gap-3 px-3 py-2.5">
          <EntityLogo
            name={m.job.company}
            src={companyLogoUrl(m.job.company) ?? undefined}
            size="xs"
            shape="square"
            class="shrink-0"
          />
          <div class="min-w-0 flex-1">
            <a
              href={resolve('/jobs/[slug]', { slug: m.job.public_slug })}
              target="_blank"
              rel="noopener"
              class="line-clamp-1 text-sm font-medium hover:underline">{m.job.title}</a
            >
            <p class="line-clamp-1 text-xs text-muted-foreground">{meta(m.job)}</p>
          </div>
          {#if ranked}
            <span class="shrink-0 text-sm font-semibold tabular-nums text-brand">{m.match_percent}%</span>
          {/if}
        </li>
      {/each}
    </ul>
    <!-- eslint-disable svelte/no-navigation-without-resolve -- the feed route, resolved, with the searched query appended -->
    <a
      href={feedHref}
      target="_blank"
      rel="noopener"
      class="inline-flex items-center gap-1 self-end text-sm font-medium text-brand-strong underline-offset-4 hover:underline"
    >
      See all {total.toLocaleString('en')} in the feed
      <ArrowRight class="size-3.5" aria-hidden="true" />
    </a><!-- eslint-enable svelte/no-navigation-without-resolve -->
  {:else}
    <p class="text-sm text-muted-foreground">A broader answer finds more — try another place or another role.</p>
  {/if}
</FlowStep>

<FlowStep
  icon={Gauge}
  title="Your CV's ATS score"
  subtitle="How well an applicant tracking system reads it."
  state="done"
>
  <AtsScorecard {report} />
</FlowStep>
