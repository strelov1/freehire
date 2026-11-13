<script lang="ts">
  import { resolve } from '$app/paths';
  import { api } from '#lib/api.js';
  import { AsyncData } from '#lib/asyncData.svelte.js';
  import { isAuthenticated } from '#lib/auth.svelte.js';
  import SignupsBars from '#lib/components/SignupsBars.svelte';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import type { ApiUsageDay } from '#lib/types.js';
  import { dailyUsage } from './usage';
  import { messages } from './messages';

  const s = $derived(t(messages, locale()));
  const usage = new AsyncData<ApiUsageDay[]>([]);
  // The server writes counts once a minute, so an open page re-reads on the same beat.
  let now = $state(new Date());
  // Until one read succeeds the figures are unknown, not zero; a later failed refresh keeps
  // the last good ones (AsyncData keeps its value on error).
  let loaded = $state(false);
  $effect(() => {
    if (!isAuthenticated()) return;
    const load = () => {
      now = new Date();
      void usage
        .run(() => api.getApiUsage())
        .then(() => {
          if (usage.status === 'ready') loaded = true;
        });
    };
    load();
    const timer = setInterval(load, 60_000);
    return () => clearInterval(timer);
  });

  const series = $derived(dailyUsage(usage.value, now));
  const total = $derived(series.reduce((n, d) => n + d.new, 0));
  const errors = $derived(usage.value.reduce((n, d) => n + d.errors, 0));
  const today = $derived(series.at(-1)?.new ?? 0);
</script>

<svelte:head><title>{s.headTitle}</title></svelte:head>

<div class="flex flex-col gap-6">
  <div class="grid grid-cols-3 gap-3 max-sm:grid-cols-1">
    {#each [[s.today, today], [s.month, total], [s.errorCount, errors]] as const as [label, value] (label)}
      <div class="flex flex-col gap-1 rounded-xl border border-border p-4">
        <span class="text-sm text-muted-foreground">{label}</span>
        <span class="text-2xl font-semibold tabular-nums">{loaded ? value.toLocaleString(locale()) : '—'}</span>
      </div>
    {/each}
  </div>

  <section class="flex flex-col gap-3 rounded-xl border border-border p-4">
    {#if usage.status === 'error' && !loaded}
      <p class="text-sm text-destructive" role="alert">{s.usageError}</p>
    {:else if loaded}
      <SignupsBars points={total > 0 ? series : []} label={s.chartLabel} empty={s.chartEmpty} />
    {/if}
    <p class="text-xs text-muted-foreground">{s.usageNote}</p>
  </section>

  <a href={resolve('/docs/api')} class="text-sm font-medium underline-offset-4 hover:underline">{s.docs}</a>
</div>
