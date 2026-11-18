<script lang="ts">
  import { resolve } from '$app/paths';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import type { Job } from '#lib/types.js';
  import { Button } from '#lib/ui/index.js';
  import { messages } from './EmployerView.messages';

  // One vacancy in the employer's list: its title, badges and location, and the edit,
  // close and reopen actions. EmployerDashboard owns the state the actions change.
  let {
    job,
    closing,
    reopening,
    reopenError,
    onEdit,
    onClose,
    onReopen,
  }: {
    job: Job;
    closing: boolean;
    reopening: boolean;
    /** The reopen failure for this vacancy, if its last reopen failed. */
    reopenError: string | null;
    onEdit: () => void;
    onClose: () => void;
    onReopen: () => void;
  } = $props();

  const s = $derived(t(messages, locale()));
</script>

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
  </div>
  {#if reopenError}
    <p class="text-sm text-destructive">{reopenError}</p>
  {/if}
</li>
