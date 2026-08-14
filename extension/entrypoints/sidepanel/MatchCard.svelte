<script lang="ts">
  import { Bookmark, FileText, SquarePen } from '@lucide/svelte';
  import { Button, Card, CountryFlag } from 'freehire-design-system';
  import { getToken, HIRE_ORIGIN } from '../../lib/auth';
  import {
    companyLogoUrl,
    getMatchAnalysis,
    saveJob,
    unsaveJob,
    type FreehireJob,
    type JobMatch,
    type MatchAnalysisResponse,
  } from '../../lib/freehire';
  import { categoryLabel, countryLabel, regionLabel, workModeLabel } from '../../lib/labels';

  let { job, match }: { job: FreehireJob; match: JobMatch } = $props();

  function scoreTone(score: number): 'good' | 'warn' | 'bad' {
    if (score >= 70) return 'good';
    if (score >= 40) return 'warn';
    return 'bad';
  }

  let pct = $derived(Math.max(0, Math.min(100, match.coverage_percent)));
  let tone = $derived(scoreTone(pct));
  let logoUrl = $derived(companyLogoUrl(job.company));
  let monogram = $derived((job.company || job.title || '?').trim().charAt(0).toUpperCase());
  let logoFailed = $state(false);
  let jobUrl = $derived(`${HIRE_ORIGIN}/jobs/${encodeURIComponent(job.public_slug)}`);
  let tailorUrl = $derived(`${HIRE_ORIGIN}/tailor/${encodeURIComponent(job.public_slug)}`);
  let profileUrl = $derived(`${HIRE_ORIGIN}/my/profile`);
  // The ad-hoc text match (a page scraped off any site, not a catalog posting) carries
  // no slug — see App.svelte's `unknownPage`. Analyze/Save/facts all need a real job to
  // call the API against, so they stay hidden rather than firing requests at
  // `/api/v1/jobs//...`.
  let isCatalogJob = $derived(job.public_slug !== '');

  // Ordered job-facet rows, only those the job actually states. Labels mirror
  // web's enrichment.ts summaryFacets — see lib/labels.ts for why this is a port
  // rather than a shared import.
  interface Fact {
    label: string;
    text?: string;
    countries?: string[];
  }
  function buildFacts(j: FreehireJob): Fact[] {
    const rows: Fact[] = [];
    if (j.work_mode) rows.push({ label: 'Work format', text: workModeLabel(j.work_mode) });
    if (j.location) rows.push({ label: 'Location', text: j.location });
    if (j.regions?.length) rows.push({ label: 'Region', text: j.regions.map(regionLabel).join(', ') });
    if (j.enrichment?.category) rows.push({ label: 'Category', text: categoryLabel(j.enrichment.category) });
    if (j.countries?.length) rows.push({ label: 'Country', countries: j.countries });
    return rows;
  }
  let facts = $derived(buildFacts(job));

  // Optimistic, local-only: the card never fetches whether this job was saved
  // before the panel opened it (no proactive "record a view" call, unlike the
  // web job page) — it only reflects the panel's own save/unsave actions.
  let saved = $state(false);
  let savePending = $state(false);

  async function toggleSave() {
    if (savePending) return;
    const token = await getToken();
    if (!token) return;
    savePending = true;
    const wasSaved = saved;
    saved = !wasSaved;
    try {
      await (wasSaved ? unsaveJob(job.public_slug, token) : saveJob(job.public_slug, token));
    } catch {
      saved = wasSaved;
    } finally {
      savePending = false;
    }
  }

  // Cached AI fit analysis — read-only, same contract as web's MatchSummary.svelte:
  // never computes inline, just shows whatever the full-page analysis last cached.
  let analysisData = $state<MatchAnalysisResponse | null>(null);
  $effect(() => {
    const slug = job.public_slug;
    analysisData = null;
    if (slug === '') return;
    getToken()
      .then((token) => {
        if (!token) return null;
        return getMatchAnalysis(slug, token);
      })
      .then((d) => {
        if (d && job.public_slug === slug) analysisData = d;
      })
      .catch(() => {});
  });
  let analysis = $derived(analysisData?.analysis ?? null);
  let topGap = $derived(analysis?.gaps?.[0] ?? null);
  let credits = $derived(analysisData?.credits ?? null);
  let creditsSpent = $derived(!!credits && credits.remaining <= 0);
  let analysisTone = $derived(analysis ? scoreTone(analysis.overall_score) : 'good');
</script>

<Card class="card">
  <a class="job" href={jobUrl} target="_blank" rel="noreferrer">
    {#key job.company}
      <div class="logo">
        {#if logoUrl && !logoFailed}
          <img src={logoUrl} alt="" onerror={() => (logoFailed = true)} />
        {:else}
          <span class="monogram">{monogram}</span>
        {/if}
      </div>
    {/key}
    <div class="jobmeta">
      {#if job.company}<div class="company">{job.company}</div>{/if}
      <div class="title">{job.title}</div>
    </div>
  </a>

  <div class="mrow">
    <span class="label">Profile match</span>
    <span class="count">{match.matched.length} of {match.total} skills</span>
  </div>
  <div class="pct {tone}">{pct}%</div>
  <div class="bar"><div class="fill {tone}" style="width:{pct}%"></div></div>

  <div class="group">
    <div class="glabel"><span class="dot good"></span> You have</div>
    <div class="chips">
      {#each match.matched as s (s)}<span class="chip good">{s}</span>{/each}
      {#if match.matched.length === 0}<span class="none">no matching skills yet</span>{/if}
    </div>
  </div>

  {#if match.missing.length > 0}
    <div class="group">
      <div class="glabel"><span class="dot miss"></span> Missing</div>
      <div class="chips">
        {#each match.missing as s (s)}<span class="chip miss">{s}</span>{/each}
      </div>
    </div>
  {/if}

  {#if isCatalogJob}
    <div class="analyze">
      <div class="label">Analyze match</div>

      {#if analysisData && !analysisData.has_cv}
        <div class="upload-row">
          <span class="upload-hint"><FileText class="icon-sm" />Upload a CV to analyse</span>
          <Button variant="primary" size="sm" href={profileUrl} target="_blank" rel="noreferrer">
            Upload CV
          </Button>
        </div>
      {:else if analysis}
        <a class="analysis-card" href={tailorUrl} target="_blank" rel="noreferrer">
          <div class="analysis-row">
            <span class="analysis-pct {analysisTone}">{analysis.overall_score}%</span>
            <span class="analysis-verdict {analysisTone}">{analysis.verdict}</span>
          </div>
          {#if topGap}
            <p class="analysis-gap"><span class="gap-label">Top gap:</span> {topGap}</p>
          {/if}
          <span class="analysis-link">View full analysis →</span>
        </a>
      {:else if creditsSpent}
        <p class="hint">
          You're out of AI credits for this month. They renew
          {credits ? new Date(credits.resets_at).toLocaleDateString(undefined, { month: 'short', day: 'numeric' }) : 'next month'}.
        </p>
      {:else}
        <p class="hint">How your CV reads against this role — fit, gaps, and ATS flags.</p>
        <Button class="tailor" variant="primary" size="sm" href={tailorUrl} target="_blank" rel="noreferrer">
          Tailor my CV
          <SquarePen class="icon-sm" />
        </Button>
        {#if credits}
          <p class="hint credits">{credits.remaining} AI credits left this month</p>
        {/if}
      {/if}
    </div>

    <Button
      class="save"
      variant="outline"
      size="sm"
      aria-pressed={saved}
      disabled={savePending}
      onclick={toggleSave}
    >
      <Bookmark class={saved ? 'icon-sm filled' : 'icon-sm'} />
      {saved ? 'Saved' : 'Save'}
    </Button>

    {#if facts.length}
      <dl class="facts">
        {#each facts as fact (fact.label)}
          <div class="frow">
            <dt class="flabel">{fact.label}</dt>
            {#if fact.countries}
              <dd class="fflags">
                {#each fact.countries as code (code)}
                  <CountryFlag {code} label={countryLabel(code)} class="flag" />
                {/each}
              </dd>
            {:else}
              <dd class="fvalue">{fact.text}</dd>
            {/if}
          </div>
        {/each}
      </dl>
    {/if}
  {/if}
</Card>

<style>
  :global(.card) {
    padding: 14px;
    margin: 12px;
  }
  .job {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-bottom: 12px;
    text-decoration: none;
    color: inherit;
  }
  .logo {
    flex: none;
    width: 36px;
    height: 36px;
    border-radius: 8px;
    overflow: hidden;
    border: 1px solid var(--border);
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--muted);
  }
  .logo img {
    width: 100%;
    height: 100%;
    object-fit: contain;
  }
  .monogram {
    font-size: 16px;
    font-weight: 700;
    color: var(--muted-foreground);
  }
  .jobmeta {
    min-width: 0;
  }
  .company {
    font-size: 12px;
    color: var(--muted-foreground);
  }
  .title {
    font-size: 15px;
    font-weight: 700;
    line-height: 1.25;
    margin-top: 2px;
  }
  .mrow {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
  }
  .label {
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--muted-foreground);
    font-weight: 600;
  }
  .count {
    font-size: 12px;
    color: var(--muted-foreground);
  }
  .pct {
    font-size: 30px;
    font-weight: 800;
    line-height: 1.1;
    margin: 2px 0 8px;
  }
  .pct.good {
    color: var(--brand);
  }
  .pct.warn {
    color: var(--warning-strong);
  }
  .pct.bad {
    color: var(--destructive);
  }
  .bar {
    height: 8px;
    border-radius: 999px;
    background: var(--muted);
    overflow: hidden;
  }
  .fill {
    height: 100%;
    border-radius: 999px;
    transition: width 0.3s ease;
  }
  .fill.good {
    background: var(--brand);
  }
  .fill.warn {
    background: var(--warning);
  }
  .fill.bad {
    background: var(--destructive);
  }
  .group {
    margin-top: 12px;
  }
  .glabel {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 13px;
    color: var(--foreground);
  }
  .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    display: inline-block;
  }
  .dot.good {
    background: var(--brand);
  }
  .dot.miss {
    background: var(--muted-foreground);
  }
  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 5px;
    margin-top: 6px;
  }
  .chip {
    font-size: 12px;
    padding: 3px 9px;
    border-radius: 999px;
    white-space: nowrap;
  }
  .chip.good {
    background: var(--brand-muted);
    color: var(--brand-strong);
    border: 1px solid color-mix(in srgb, var(--brand) 30%, transparent);
  }
  .chip.miss {
    background: var(--muted);
    color: var(--muted-foreground);
    border: 1px solid var(--border);
  }
  .none {
    font-size: 12px;
    color: var(--muted-foreground);
  }
  :global(.tailor) {
    width: 100%;
    margin-top: 14px;
    gap: 6px;
  }
  .analyze {
    margin-top: 16px;
    padding-top: 14px;
    border-top: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .hint {
    font-size: 13px;
    color: var(--muted-foreground);
  }
  .hint.credits {
    font-size: 12px;
    margin-top: -2px;
  }
  .upload-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
  }
  .upload-hint {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
    color: var(--muted-foreground);
  }
  .analysis-card {
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 10px;
    border-radius: 10px;
    border: 1px solid var(--border);
    text-decoration: none;
    color: inherit;
    transition: border-color 0.15s ease;
  }
  .analysis-card:hover {
    border-color: color-mix(in srgb, var(--brand) 40%, var(--border));
  }
  .analysis-row {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 8px;
  }
  .analysis-pct {
    font-size: 22px;
    font-weight: 800;
    line-height: 1;
  }
  .analysis-verdict {
    font-size: 13px;
    font-weight: 600;
  }
  .analysis-pct.good,
  .analysis-verdict.good {
    color: var(--brand-strong);
  }
  .analysis-pct.warn,
  .analysis-verdict.warn {
    color: var(--warning-strong);
  }
  .analysis-pct.bad,
  .analysis-verdict.bad {
    color: var(--destructive);
  }
  .analysis-gap {
    font-size: 12px;
    color: var(--muted-foreground);
  }
  .gap-label {
    font-weight: 500;
    color: var(--foreground);
  }
  .analysis-link {
    font-size: 12px;
    font-weight: 500;
    color: var(--brand-strong);
  }
  :global(.save) {
    width: 100%;
    margin-top: 10px;
    gap: 6px;
  }
  :global(.icon-sm) {
    width: 14px;
    height: 14px;
    flex-shrink: 0;
  }
  :global(.icon-sm.filled) {
    fill: currentcolor;
  }
  .facts {
    margin-top: 14px;
    padding-top: 12px;
    border-top: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .frow {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 10px;
    font-size: 13px;
  }
  .flabel {
    flex: none;
    color: var(--muted-foreground);
  }
  .fvalue {
    min-width: 0;
    text-align: right;
    font-weight: 500;
    word-break: break-word;
  }
  .fflags {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    flex-wrap: wrap;
    gap: 4px;
  }
  :global(.flag) {
    font-size: 16px;
  }
</style>
