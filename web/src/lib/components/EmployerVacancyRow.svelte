<script lang="ts">
  import { resolve } from '$app/paths';
  import { api } from '#lib/api.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import type { Job } from '#lib/types.js';
  import { Button } from '#lib/ui/index.js';
  import { messages } from './EmployerView.messages';

  // One vacancy in the employer's list: its title, badges and location, and the edit,
  // close and reopen actions. EmployerDashboard owns the state the actions change.
  let {
    job,
    canManage = true,
    closing,
    reopening,
    reopenError,
    onEdit,
    onClose,
    onReopen,
  }: {
    job: Job;
    canManage?: boolean;
    closing: boolean;
    reopening: boolean;
    /** The reopen failure for this vacancy, if its last reopen failed. */
    reopenError: string | null;
    onEdit: () => void;
    onClose: () => void;
    onReopen: () => void;
  } = $props();

  const s = $derived(t(messages, locale()));
  let analytics = $state<ReturnType<typeof api.getEmployerVacancyAnalytics> | null>(null);
  const number = $derived(new Intl.NumberFormat(locale()));
  const percent = $derived(new Intl.NumberFormat(locale(), { style: 'percent', maximumFractionDigits: 1 }));
</script>

<li class="flex flex-col gap-3 px-4 py-3">
  <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
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
    <div class="flex shrink-0 flex-wrap gap-2">
      <Button
        type="button"
        variant="ghost"
        size="sm"
        aria-expanded={analytics !== null}
        onclick={() => (analytics = analytics ? null : api.getEmployerVacancyAnalytics(job.public_slug))}
        >{s.analytics}</Button
      >
      {#if canManage}
        <Button variant="ghost" size="sm" onclick={onEdit}>{s.editVacancy}</Button>
        {#if !job.closed_at}
          <Button variant="ghost" size="sm" disabled={closing} onclick={onClose}>
            {closing ? s.closing : s.closeVacancy}
          </Button>
        {:else}
          <Button variant="ghost" size="sm" disabled={reopening} onclick={onReopen}>
            {reopening ? s.reopening : s.reopenVacancy}
          </Button>
        {/if}
      {/if}
    </div>
  </div>
  {#if reopenError}
    <p class="text-sm text-destructive">{reopenError}</p>
  {/if}
  {#if analytics}
    <section aria-label={s.analytics} class="flex flex-col gap-3 rounded-md bg-muted/50 p-3">
      <p class="text-xs font-medium text-muted-foreground">{s.analyticsPeriod}</p>
      {#await analytics}
        <p role="status" class="text-sm text-muted-foreground">{s.analyticsLoading}</p>
      {:then report}
        <dl class="grid grid-cols-2 gap-3 sm:grid-cols-4">
          {#each [[s.analyticsViews, number.format(report.views)], [s.analyticsClicks, number.format(report.apply_clicks)], [s.analyticsApplications, number.format(report.applications)], [s.analyticsClickRate, report.views > 0 ? percent.format(report.apply_clicks / report.views) : '—']] as [label, value] (label)}
            <div class="flex flex-col gap-1">
              <dt class="text-xs text-muted-foreground">{label}</dt>
              <dd class="text-lg font-semibold tabular-nums">{value}</dd>
            </div>
          {/each}
        </dl>
        <p class="text-xs text-muted-foreground">{s.analyticsNote}</p>
      {:catch}
        <p role="alert" class="text-sm text-destructive">{s.analyticsError}</p>
      {/await}
    </section>
  {/if}
</li>
