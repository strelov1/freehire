<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { SvelteURLSearchParams } from 'svelte/reactivity';
  import { Briefcase, Building2, Tags } from '@lucide/svelte';
  import Seo from '#lib/components/Seo.svelte';
  import { Input, TabStrip, tabStripId } from '#lib/ui/index.js';
  import CategoryPane from '#lib/components/filters/CategoryPane.svelte';
  import SearchSelect from '#lib/components/facets/SearchSelect.svelte';
  import { ApiError, api } from '#lib/api.js';
  import { dynamicOptions } from '#lib/facets.js';
  import { filtersToParams } from '#lib/filters.js';
  import { promptSignIn } from '#lib/signin.js';
  import { StagedFilters } from '#lib/stagedFilters.svelte.js';
  import { lookupIndustry, isIndustryMiss, searchIndustries, seedDictionaryReturnParams } from '#lib/dictionaryIndustry.js';

  type Kind = 'specialization' | 'skill' | 'industry';

  let { data } = $props();

  const store = new StagedFilters();
  // Seeded from the URL so a visitor sent to sign in comes back to the term they typed.
  let specQuery = $state(page.url.searchParams.get('specialization') ?? '');
  let skillQuery = $state(page.url.searchParams.get('skill') ?? '');
  let industryQuery = $state(page.url.searchParams.get('industry') ?? '');
  // A dictionary query (back from sign-in) or an explicit tab query reopens its tab.
  let tab = $state<Kind>(
    page.url.searchParams.has('industry') || page.url.searchParams.get('tab') === 'industry'
      ? 'industry'
      : page.url.searchParams.has('skill') || page.url.searchParams.get('tab') === 'skill'
        ? 'skill'
        : 'specialization',
  );
  const tabs = [
    { id: 'specialization', label: 'Specialization', icon: Briefcase },
    { id: 'skill', label: 'Skills', icon: Tags },
    { id: 'industry', label: 'Industries', icon: Building2 },
  ] as const;
  const PANEL_ID = 'dictionary-panel';

  // Per term: how many times it has been asked for, 'known' when the server already
  // resolves it, or a refusal to show.
  let asked = $state<Record<string, number | 'known' | 'limit' | 'failed'>>({});
  let pending = $state<string | null>(null);

  const key = (kind: Kind, term: string) => `${kind}:${term.toLowerCase()}`;

  const skills = $derived(store.facet('skills'));
  const industryResults = $derived(searchIndustries(industryQuery));
  const industryLookup = $derived(lookupIndustry(industryQuery));
  // Every skill the dictionary knows, busiest first; without counts, no numbers.
  const skillOptions = $derived.by(() => {
    const options = dynamicOptions('skills', data.counts?.facets.skills ?? {}, [...data.skillSlugs, ...skills.include]);
    return data.counts ? options : options.map((o) => ({ ...o, count: undefined }));
  });
  const jobsQuery = $derived(store.active > 0 ? filtersToParams(store.value).toString() : '');

  async function rememberTerm(kind: Kind, term: string) {
    const params = new SvelteURLSearchParams();
    seedDictionaryReturnParams(params, kind, term);
    const next = `${resolve('/dictionary')}?${params}`;
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- query is appended to resolve('/dictionary')
    await goto(next, { replaceState: true, reset: false });
  }

  async function add(kind: Kind, term: string) {
    const k = key(kind, term);
    pending = k;
    try {
      const res = await api.recordDictionaryGap(kind, term);
      asked[k] = res.request_count;
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        await rememberTerm(kind, term);
        promptSignIn();
        return;
      }
      if (err instanceof ApiError && err.status === 409) asked[k] = 'known';
      else if (err instanceof ApiError && err.status === 429) asked[k] = 'limit';
      else asked[k] = 'failed';
    } finally {
      pending = null;
    }
  }
</script>

<Seo
  title="Dictionary · freehire"
  description="Explore the specializations, skills, and company industries freehire recognizes. Request a term the dictionaries do not know yet."
  canonical={`${page.url.origin}/dictionary`}
/>

{#snippet missing(kind: Kind, term: string)}
  {@const state = asked[key(kind, term)]}
  <div class="flex flex-wrap items-center gap-3 py-1 text-sm">
    {#if state === undefined}
      <span>“{term}” is not in the dictionary.</span>
      <button
        type="button"
        class="rounded-full bg-foreground px-3 py-1 font-medium text-background disabled:opacity-50"
        disabled={pending === key(kind, term)}
        onclick={() => void add(kind, term)}
      >
        Add to the list
      </button>
    {:else if state === 'known'}
      <span class="text-muted-foreground">“{term}” is already in the dictionary under another name.</span>
    {:else if state === 'limit'}
      <span class="text-muted-foreground">Daily limit reached. Try again tomorrow.</span>
    {:else if state === 'failed'}
      <span class="text-muted-foreground">Could not record this term.</span>
    {:else}
      <span class="text-muted-foreground">Thanks. Asked {state} {state === 1 ? 'time' : 'times'}.</span>
    {/if}
  </div>
{/snippet}

{#snippet missingSpecialization(term: string)}{@render missing('specialization', term)}{/snippet}
{#snippet missingSkill(term: string)}{@render missing('skill', term)}{/snippet}
{#snippet missingIndustry(term: string)}{@render missing('industry', term)}{/snippet}

<div class="mx-auto w-full max-w-4xl px-4 py-6">
  <header class="mb-6">
    <h1 class="text-2xl font-semibold tracking-tight">Explore the dictionaries</h1>
    <p class="mt-2 max-w-2xl text-sm leading-relaxed text-muted-foreground">
      Search the specializations and skills we recognize in job posts, or the industries we use to classify companies. If a term is missing, request it for review.
    </p>
  </header>

  <TabStrip {tabs} active={tab} onSelect={(id) => (tab = id)} label="Dictionary" panelId={PANEL_ID} class="mb-6" />

  <section id={PANEL_ID} role="tabpanel" aria-labelledby={tabStripId(PANEL_ID, tab)} tabindex="-1">
    {#if tab === 'specialization'}
      <CategoryPane {store} plain counts={data.counts} bind:query={specQuery} empty={missingSpecialization} />
    {:else if tab === 'skill'}
      <h2 class="mb-2 text-sm font-semibold tracking-tight">Skills</h2>
      <SearchSelect
        options={skillOptions}
        include={skills.include}
        placeholder="Search skills"
        techIcons
        expand
        onToggle={(v) => store.pick('skills', v)}
        bind:filter={skillQuery}
        empty={missingSkill}
      />
    {:else}
      <h2 class="mb-1 text-sm font-semibold tracking-tight">Industries</h2>
      <p class="mb-3 text-xs text-muted-foreground">
        These labels classify companies, not job posts; this dictionary has no counts.
      </p>
      <Input
        type="search"
        bind:value={industryQuery}
        placeholder="Search company industries"
        aria-label="Search company industries"
        class="mb-3 w-full"
      />
      {#if industryQuery.trim() && industryLookup.type === 'canonical'}
        <p class="mb-3 text-sm text-muted-foreground">
          Canonical entry: <code>{industryLookup.slug}</code> — {industryLookup.label}
        </p>
      {:else if industryQuery.trim() && industryLookup.type === 'alias'}
        <p class="mb-3 text-sm text-muted-foreground">
          Alias “{industryQuery.trim()}” resolves to <code>{industryLookup.slug}</code> — {industryLookup.label}
        </p>
      {/if}
      {#if industryResults.length}
        <ul class="max-h-96 space-y-1 overflow-y-auto rounded-md border p-2" aria-label="Recognized industries">
          {#each industryResults as industry (industry.slug)}
            <li class="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1 rounded px-2 py-1.5 text-sm">
              <span>{industry.label} <code class="ml-1 text-xs text-muted-foreground">{industry.slug}</code></span>
            </li>
          {/each}
        </ul>
      {/if}
      {#if isIndustryMiss(industryQuery)}
        {@render missingIndustry(industryQuery.trim())}
      {:else if !industryQuery.trim() && !industryResults.length}
        <p class="text-sm text-muted-foreground">No industries are available right now.</p>
      {/if}
    {/if}
  </section>

  {#if jobsQuery}
    <div class="sticky bottom-4 mt-8 flex justify-end">
      <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- internal /jobs filter link; resolve()d base plus a query string -->
      <a href={`${resolve('/jobs')}?${jobsQuery}`} class="rounded-full bg-foreground px-4 py-2 text-sm font-medium text-background shadow">
        Show jobs
      </a>
    </div>
  {/if}
</div>
