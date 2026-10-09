<script lang="ts">
  import { api } from '$lib/api';
  import { AsyncData } from '$lib/asyncData.svelte';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { interviewRate, offerRate, replyRate } from '$lib/pipeline';
  import type { PipelineStats } from '$lib/types';
  import PipelineFunnel from './PipelineFunnel.svelte';
  import RateDonut from './RateDonut.svelte';
  import States from './States.svelte';
  import { messages } from './PipelineView.messages';
  import { locale } from '$lib/i18n/currentLocale.svelte';
  import { format, plural, t } from '$lib/i18n/t';

  // Single aggregate fetch (not paginated): the snapshot of where the caller's
  // applications stand.
  const pipeline = new AsyncData<PipelineStats | null>(null);
  $effect(() => {
    if (isAuthenticated()) void pipeline.run(() => api.getMyPipeline());
  });
  const status = $derived(pipeline.status);
  const stats = $derived(pipeline.value);

  const iv = $derived(stats ? interviewRate(stats) : 0);
  const offer = $derived(stats ? offerRate(stats) : 0);

  // Absent below the server's ten-application sample gate — never a zero or an
  // estimate, so there is nothing to derive when it is missing.
  const benchmark = $derived(stats?.reply_rate);
  const s = $derived(t(messages, locale()));
</script>

{#if status === 'loading'}
  <States state="loading" rows={3} />
{:else if status === 'error'}
  <States state="error" message={s.loadError} />
{:else if !stats || stats.applications === 0}
  <States state="empty" message={s.empty} />
{:else}
  <div class="flex flex-col gap-3">
    <!-- Rates and the funnel are two separate views, each in its own card. -->
    <div class="rounded-lg border bg-card p-5">
      <div class="flex flex-wrap items-center justify-center gap-10">
        <RateDonut percent={iv} label={s.interviewRate} sublabel={s.reachedInterview} />
        <RateDonut percent={offer} label={s.offerRate} sublabel={s.reachedOffer} />
      </div>
    </div>
    <div class="rounded-lg border bg-card p-5">
      <p class="mb-3 text-sm text-muted-foreground">
        {format(plural(locale(), stats.applications, s.applications), { count: String(stats.applications) })}
      </p>
      <PipelineFunnel {stats} />
    </div>
    {#if benchmark}
      <div class="rounded-lg border bg-card p-5">
        <div class="flex flex-wrap items-center justify-center gap-10">
          <RateDonut
            percent={replyRate(benchmark.you)}
            label={s.yourReplyRate}
            sublabel={format(s.withMailbox, { count: String(benchmark.you.applications) })}
          />
          <RateDonut
            percent={replyRate(benchmark.global)}
            label={s.averageReplyRate}
            sublabel={s.everyOtherCandidate}
          />
        </div>
      </div>
    {/if}
    <p class="text-xs text-muted-foreground">
      {s.footnote}
    </p>
  </div>
{/if}
