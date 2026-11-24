<script lang="ts">
  import { api } from '#lib/api.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { messages } from './EmployerView.messages';

  // Human clicks freehire sent to the employer's site for one vacancy: totals, a day table
  // (the sponsored dashboard's style), then the country and source splits.
  let { slug }: { slug: string } = $props();

  const s = $derived(t(messages, locale()));
  const number = $derived(new Intl.NumberFormat(locale()));
  const report = $derived(api.getEmployerVacancyOutboundClicks(slug));

  // Cloudflare also sends XX (unknown) and T1 (Tor), which are not regions: show the code as sent.
  function country(code: string): string {
    if (code === '') return s.outboundUnknown;
    try {
      return new Intl.DisplayNames(locale(), { type: 'region' }).of(code) ?? code;
    } catch {
      return code;
    }
  }
</script>

<div class="flex flex-col gap-3 border-t border-border pt-3">
  <p class="text-xs font-medium text-muted-foreground">{s.outboundTitle}</p>
  {#await report}
    <p role="status" class="text-sm text-muted-foreground">{s.analyticsLoading}</p>
  {:then r}
    {#if r.clicks === 0}
      <p class="text-sm text-muted-foreground">{s.outboundEmpty}</p>
    {:else}
      <dl class="grid grid-cols-2 gap-3">
        {#each [[s.outboundClicks, r.clicks], [s.outboundVisitors, r.visitors]] as [label, value] (label)}
          <div class="flex flex-col gap-1">
            <dt class="text-xs text-muted-foreground">{label}</dt>
            <dd class="text-lg font-semibold tabular-nums">{number.format(Number(value))}</dd>
          </div>
        {/each}
      </dl>
      <div class="overflow-x-auto">
        <table class="w-full text-left text-xs">
          <thead class="text-muted-foreground">
            <tr>
              <th class="py-1 pr-3 font-medium">{s.outboundDay}</th>
              <th class="py-1 pr-3 font-medium">{s.outboundClicks}</th>
              <th class="py-1 font-medium">{s.outboundVisitors}</th>
            </tr>
          </thead>
          <tbody>
            {#each r.days as d (d.day)}
              <tr class="border-t border-border">
                <td class="py-1 pr-3">{d.day}</td>
                <td class="py-1 pr-3">{number.format(d.clicks)}</td>
                <td class="py-1">{number.format(d.visitors)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
      <div class="grid gap-3 text-xs sm:grid-cols-2">
        <div>
          <h3 class="font-semibold">{s.outboundCountries}</h3>
          <ul>
            {#each r.countries as c (c.key)}
              <li>{country(c.key)}: {number.format(c.clicks)}</li>
            {/each}
          </ul>
        </div>
        <div>
          <h3 class="font-semibold">{s.outboundSources}</h3>
          <ul>
            {#each r.sources as c (c.key)}
              <li>{c.key}: {number.format(c.clicks)}</li>
            {/each}
          </ul>
        </div>
      </div>
    {/if}
  {:catch}
    <p role="alert" class="text-sm text-destructive">{s.outboundError}</p>
  {/await}
</div>
