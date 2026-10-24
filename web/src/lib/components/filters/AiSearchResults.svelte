<script lang="ts" module>
  /** What one run of a plan found. `catalogue` is the published snapshot's de-duplicated
   *  open-job count and null when the snapshot is not exact (or could not be read) — a
   *  planner estimate must not reach the page as a measurement. `companiesTotal` is null
   *  when the facets endpoint could not count companies for this plan. */
  export type Overview = {
    catalogue: number | null;
    found: number;
    companiesTotal: number | null;
    countries: Record<string, number>;
    topCompanies: { slug: string; name: string; count: number }[];
  };
</script>

<script lang="ts">
  import type { Snippet } from 'svelte';
  import { ArrowLeft, MessageSquare } from '@lucide/svelte';
  import { Button, Card, EntityLogo, SectionLabel } from '#lib/ui/index.js';
  import { limitToRequestedCountries } from '#lib/aiFilter.js';
  import { isAuthenticated } from '#lib/auth.svelte.js';
  import { companyLogoUrl } from '#lib/logo.js';
  import { signinUrl } from '#lib/signin.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, plural, t } from '#lib/i18n/t.js';
  import type { Interpretation } from '#lib/types.js';
  import { messages } from './AiSearchResults.messages';

  // The Results step: what the plan finds before anything is written to the job list —
  // the catalogue it searched, how many jobs and companies match, where they are and who
  // hires most — with the chat beside it for one more refinement, and Show N jobs to
  // apply. The host owns the requests and the conversation; this owns the layout.
  //
  // The chat stays reachable at every width (spec: "beside the overview at every
  // viewport width"): a side panel from `xl`, and below that a panel the Chat button
  // opens over the overview, because a 390px screen has no room for two columns.
  let {
    plan,
    overview,
    stale,
    chat,
    onApply,
  }: {
    plan: Interpretation;
    overview: Overview | null;
    /** A run is in flight; the figures on screen belong to the previous plan. */
    stale: boolean;
    chat: Snippet<[boolean]>;
    onApply: () => void;
  } = $props();

  const s = $derived(t(messages, locale()));
  const loc = $derived(locale() ?? 'en');
  const fmt = (n: number) => n.toLocaleString(loc);

  let chatOpen = $state(false);

  const countries = $derived(overview ? limitToRequestedCountries(overview.countries, plan) : []);
  // A figure we could not measure is shown as a dash with the reason beside its label,
  // never as a zero — and never at the size of a real number.
  const stats = $derived.by(() => {
    if (!overview) return [];
    const out: { label: string; value: string; note?: string }[] = [];
    if (overview.catalogue != null) out.push({ label: s.jobsSearched, value: fmt(overview.catalogue) });
    out.push({ label: s.jobsFound, value: fmt(overview.found) });
    out.push(
      overview.companiesTotal != null
        ? { label: s.companiesHiring, value: fmt(overview.companiesTotal) }
        : { label: s.companiesHiring, value: '—', note: s.unavailable },
    );
    return out;
  });
</script>

<div class="relative flex min-h-0 flex-1">
  <main class="min-w-0 flex-1 overflow-auto p-4 sm:p-5">
    {#if overview}
      <div class={stale ? 'pointer-events-none opacity-40 transition-opacity' : ''}>
        <header class="flex flex-wrap items-center justify-between gap-3 pb-4">
          <h2 class="font-semibold">{format(plural(loc, overview.found, s.found), { count: fmt(overview.found) })}</h2>
          <div class="flex flex-wrap gap-2">
            <Button variant="outline" class="xl:hidden" onclick={() => (chatOpen = true)}>
              <MessageSquare class="size-4" />
              {s.chat}
            </Button>
            {#if !isAuthenticated()}
              <Button variant="outline" href={signinUrl()}>{s.createAccount}</Button>
            {/if}
            <Button variant="primary" disabled={overview.found === 0} onclick={onApply}>
              {format(plural(loc, overview.found, s.show), { count: fmt(overview.found) })}
            </Button>
          </div>
        </header>
        <Card class="overflow-hidden">
          <div class="grid divide-x divide-border border-b border-border {stats.length === 3 ? 'grid-cols-3' : 'grid-cols-2'}">
            {#each stats as stat (stat.label)}
              <div class="p-3 sm:p-5">
                <p class="text-lg font-semibold tabular-nums sm:text-3xl">{stat.value}</p>
                <p class="text-xs text-muted-foreground sm:text-sm">{stat.label}</p>
                {#if stat.note}<p class="text-xs text-muted-foreground">{stat.note}</p>{/if}
              </div>
            {/each}
          </div>
          {#if countries.length}
            <section class="border-b border-border p-4 sm:p-5">
              <SectionLabel text={s.whereJobsAre} />
              <!-- Loaded on this step only: the land outline is the heaviest thing in the
                   modal and nobody who stops at the plan needs it. -->
              {#await import('./JobsWorldMap.svelte') then { default: JobsWorldMap }}
                <JobsWorldMap {countries} />
              {/await}
            </section>
          {/if}
          {#if overview.topCompanies.length}
            <section class="p-4 sm:p-5">
              <SectionLabel text={s.companiesSection} />
              <div class="mt-3 flex flex-wrap gap-2">
                {#each overview.topCompanies as company (company.slug)}
                  <span title={company.name}>
                    <EntityLogo name={company.name} src={companyLogoUrl(company.name) ?? undefined} shape="square" size="lg" />
                  </span>
                {/each}
              </div>
            </section>
          {/if}
        </Card>
      </div>
    {/if}
  </main>
  <aside
    class="w-full shrink-0 flex-col border-border bg-background max-xl:absolute max-xl:inset-0 max-xl:z-10 xl:flex xl:w-96 xl:border-l {chatOpen
      ? 'flex'
      : 'hidden'}"
  >
    <div class="flex items-center gap-2 border-b border-border px-4 py-3">
      <button
        type="button"
        class="flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground xl:hidden"
        onclick={() => (chatOpen = false)}
      >
        <ArrowLeft class="size-4" />
        {s.backToResults}
      </button>
      <SectionLabel text={s.chat} class="max-xl:hidden" />
    </div>
    {@render chat(true)}
  </aside>
</div>
