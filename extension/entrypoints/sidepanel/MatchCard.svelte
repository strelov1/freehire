<script lang="ts">
  import { Button, Card } from 'freehire-design-system';
  import { HIRE_ORIGIN } from '../../lib/auth';
  import { companyLogoUrl, type FreehireJob, type JobMatch } from '../../lib/freehire';

  let { job, match }: { job: FreehireJob; match: JobMatch } = $props();

  let pct = $derived(Math.max(0, Math.min(100, match.coverage_percent)));
  let tone = $derived(pct >= 70 ? 'good' : pct >= 40 ? 'warn' : 'bad');
  let logoUrl = $derived(companyLogoUrl(job.company));
  let monogram = $derived((job.company || job.title || '?').trim().charAt(0).toUpperCase());
  let logoFailed = $state(false);
  let jobUrl = $derived(`${HIRE_ORIGIN}/jobs/${encodeURIComponent(job.public_slug)}`);
  let tailorUrl = $derived(`${HIRE_ORIGIN}/tailor/${encodeURIComponent(job.public_slug)}`);
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

  <Button
    class="tailor"
    variant="primary"
    size="sm"
    href={tailorUrl}
    target="_blank"
    rel="noreferrer"
  >
    Tailor CV
  </Button>
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
  }
</style>
