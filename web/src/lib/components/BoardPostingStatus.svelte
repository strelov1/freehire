<script lang="ts">
  import type { MyJob } from '#lib/types.js';
  import { daysUntilDeadline } from '#lib/board.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, plural, t } from '#lib/i18n/t.js';
  import { messages } from './BoardPostingStatus.messages';
  let { job, id }: { job: MyJob['job']; id?: string } = $props();
  const s = $derived(t(messages, locale()));

  function remaining(deadline: string): string {
    const days = daysUntilDeadline(deadline);
    if (days < 0) return s.deadlinePassed;
    if (days === 0) return s.lastDay;
    return format(plural(locale(), days, s.daysLeft), { days: String(days) });
  }
</script>

{#if job?.closed_at || job?.application_deadline}
  <span {id} class="flex flex-wrap gap-x-2 gap-y-1 text-xs">
    {#if job?.closed_at}
      <span class="font-medium text-destructive">{s.closed}</span>
    {/if}
    {#if job?.application_deadline}
      <span class="text-muted-foreground">{s.applyBy} <time datetime={job.application_deadline}>{job.application_deadline}</time></span>
      {#if !job.closed_at}
        <span class="text-muted-foreground">{remaining(job.application_deadline)}</span>
      {/if}
    {/if}
  </span>
{/if}
